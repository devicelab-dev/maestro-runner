# Build & Release Instructions

## Prerequisites

### For All Platforms
- Go 1.22+
- Git
- GitHub CLI (`gh`) - https://cli.github.com/

### For macOS Signing (Optional but Recommended)
- Apple Developer Account ($99/year)
- Developer ID Application certificate installed in Keychain
- App-specific password generated at https://appleid.apple.com

## Setup for macOS Notarization

1. **Generate App-Specific Password:**
   - Go to https://appleid.apple.com
   - Sign in > App-Specific Passwords > Generate
   - Save the password (format: `xxxx-xxxx-xxxx-xxxx`)

2. **Set Environment Variables:**
   ```bash
   export APPLE_ID="your@email.com"
   export APPLE_APP_PASSWORD="xxxx-xxxx-xxxx-xxxx"
   ```

   Add to your shell profile for persistence:
   ```bash
   echo 'export APPLE_ID="your@email.com"' >> ~/.zshrc
   echo 'export APPLE_APP_PASSWORD="xxxx-xxxx-xxxx-xxxx"' >> ~/.zshrc
   source ~/.zshrc
   ```

3. **Verify Certificate:**
   ```bash
   security find-identity -v -p codesigning
   ```
   Should show: `Developer ID Application: ...`

## Build Release

### Step 1: Build All Binaries

```bash
./build-release.sh
```

This will:
- Build for: darwin-amd64, darwin-arm64, linux-amd64, linux-arm64
- Sign macOS binaries (if credentials are set)
- Submit to Apple for notarization (~5-10 min per binary)
- Staple notarization tickets
- Generate checksums
- Package drivers

Output: `dist/` directory with all binaries

### Step 2: Test Binaries

```bash
# Test each platform binary
./dist/maestro-runner-darwin-arm64 --version
./dist/maestro-runner-darwin-amd64 --version
./dist/maestro-runner-linux-amd64 --version
./dist/maestro-runner-linux-arm64 --version
```

### Step 3: Create GitHub Release

```bash
# Automatic (creates release and uploads all files)
./release.sh v1.0.0

# Or manual
VERSION=v1.0.0
gh release create $VERSION \
  --title "$VERSION" \
  --notes "Release notes here" \
  dist/*
```

## Build Without Notarization (Testing)

If you don't have Apple Developer credentials:

```bash
# Just build binaries (no signing)
unset APPLE_ID APPLE_APP_PASSWORD
./build-release.sh
```

Users will need to bypass Gatekeeper manually:
```bash
xattr -d com.apple.quarantine maestro-runner
```

## Troubleshooting

### "No identity found"
- Install Developer ID Application certificate from Apple Developer portal
- Import to Keychain Access

### "Notarization failed"
```bash
# Check notarization logs
xcrun notarytool log <submission-id> \
  --apple-id $APPLE_ID \
  --password $APPLE_APP_PASSWORD \
  --team-id A3RCAA2YAX
```

### "Invalid credentials"
- Verify APPLE_ID is correct
- Regenerate app-specific password if expired
- Check TEAM_ID matches your account

## Release Checklist

- [ ] Update version in `cli.go` if needed
- [ ] Run `./build-release.sh`
- [ ] Test all binaries
- [ ] Create git tag: `git tag v1.0.0 && git push origin v1.0.0`
- [ ] Run `./release.sh v1.0.0`
- [ ] Verify release on GitHub
- [ ] Test install script: `curl -fsSL https://raw.githubusercontent.com/.../install-download.sh | bash`
- [ ] Update documentation if needed

## Files Generated

```
dist/
├── maestro-runner-darwin-amd64     # Signed & Notarized
├── maestro-runner-darwin-arm64     # Signed & Notarized
├── maestro-runner-linux-amd64
├── maestro-runner-linux-arm64
├── drivers-android.tar.gz
└── checksums.txt
```

## Security Notes

- **NEVER** commit `APPLE_APP_PASSWORD` to git
- Scripts are in `.gitignore` to prevent accidental commits
- App-specific passwords can be revoked anytime at appleid.apple.com
- Team ID is safe to include in scripts (it's public info)
