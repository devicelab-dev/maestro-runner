# Installation Guide

## macOS/Linux Installation

### Option 1: Install Script (Recommended)

The install script builds from source and handles macOS Gatekeeper issues automatically:

```bash
curl -fsSL https://raw.githubusercontent.com/devicelab-dev/maestro-runner/main/install.sh | bash
```

Or clone and run locally:

```bash
git clone https://github.com/devicelab-dev/maestro-runner.git
cd maestro-runner
./install.sh
```

### Option 2: Homebrew (macOS)

```bash
brew tap devicelab-dev/maestro-runner
brew install maestro-runner
```

### Option 3: Download Pre-built Binary

1. Download the latest release from [Releases](https://github.com/devicelab-dev/maestro-runner/releases)
2. Extract the binary
3. **macOS only**: Remove the quarantine attribute:
   ```bash
   xattr -d com.apple.quarantine maestro-runner
   ```
4. Move to a directory in your PATH:
   ```bash
   chmod +x maestro-runner
   mv maestro-runner /usr/local/bin/
   ```

### Option 4: Build from Source

Requires Go 1.21 or later:

```bash
git clone https://github.com/devicelab-dev/maestro-runner.git
cd maestro-runner
go build -o maestro-runner .
sudo mv maestro-runner /usr/local/bin/
```

## macOS Gatekeeper Issues

If you see **"maestro-runner cannot be opened because it is from an unidentified developer"**:

### Method 1: Using Terminal (Easiest)
```bash
xattr -d com.apple.quarantine /path/to/maestro-runner
```

### Method 2: System Settings
1. Try to run `maestro-runner`
2. Go to **System Settings > Privacy & Security**
3. Click **"Open Anyway"** next to the blocked message
4. Confirm when prompted

### Method 3: Right-Click
1. Right-click (or Control-click) on `maestro-runner`
2. Select **"Open"**
3. Click **"Open"** in the dialog

## Verify Installation

```bash
maestro-runner --version
```

## Troubleshooting

### "command not found: maestro-runner"

The installation directory is not in your PATH. Add this to your shell profile (`~/.zshrc` or `~/.bashrc`):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Then restart your terminal or run:
```bash
source ~/.zshrc  # or ~/.bashrc
```

### "permission denied"

Make the binary executable:
```bash
chmod +x /path/to/maestro-runner
```

### macOS "damaged and can't be opened"

This happens when the quarantine attribute is set. Remove it:
```bash
xattr -d com.apple.quarantine /path/to/maestro-runner
```

If the issue persists, build from source using the install script.

## Uninstall

```bash
rm $(which maestro-runner)
# or if installed to ~/.local/bin
rm ~/.local/bin/maestro-runner
```
