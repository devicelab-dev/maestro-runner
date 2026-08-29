#!/usr/bin/env bash
# Assemble the npm packages from an existing release build.
#
# Usage: VERSION=1.1.26 ./npm/build-npm.sh
#
# Consumes dist/<VERSION>/ — the signed, notarized tarballs build-release.sh
# already produced. Nothing is compiled here: a channel that needs its own
# build would drift from the artifact everyone else downloads.
#
# Produces npm/platforms/<pkg>/ for each target plus a versioned main package,
# and prints the publish commands. It does not publish; that needs npm
# credentials and a deliberate decision.
set -euo pipefail

VERSION="${VERSION:-}"
if [ -z "$VERSION" ]; then
    echo "Error: VERSION is required — VERSION=1.1.26 ./npm/build-npm.sh" >&2
    exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="$REPO_ROOT/dist/$VERSION"
NPM_DIR="$REPO_ROOT/npm"
PLATFORMS_DIR="$NPM_DIR/platforms"

if [ ! -d "$DIST_DIR" ]; then
    echo "Error: $DIST_DIR not found — run VERSION=$VERSION ./build-release.sh first" >&2
    exit 1
fi

# Release tarballs are named with Go's GOARCH; npm selects packages with
# Node's process.arch. They disagree on one name, so the mapping is explicit:
# "<go-os> <go-arch> <node-os> <node-arch>".
TARGETS=(
    "darwin arm64 darwin arm64"
    "darwin amd64 darwin x64"
    "linux arm64 linux arm64"
    "linux amd64 linux x64"
)

rm -rf "$PLATFORMS_DIR"
mkdir -p "$PLATFORMS_DIR"

for target in "${TARGETS[@]}"; do
    read -r go_os go_arch node_os node_arch <<< "$target"
    pkg="@devicelab/maestro-runner-${node_os}-${node_arch}"
    tarball="$DIST_DIR/maestro-runner-${VERSION}-${go_os}-${go_arch}.tar.gz"

    if [ ! -f "$tarball" ]; then
        echo "Error: missing $tarball" >&2
        exit 1
    fi

    echo "Packaging $pkg"
    dest="$PLATFORMS_DIR/maestro-runner-${node_os}-${node_arch}"
    mkdir -p "$dest"

    # The tarball's own layout is the one the binary expects: it resolves its
    # home from being at <root>/bin/maestro-runner and finds drivers at
    # <root>/drivers. Strip the wrapping directory and keep everything else
    # exactly where it was.
    tar -xzf "$tarball" -C "$dest" --strip-components=1

    # setup.sh belongs to the shell installer, not to an npm install.
    rm -f "$dest/setup.sh"

    if [ ! -x "$dest/bin/maestro-runner" ]; then
        echo "Error: $pkg has no executable at bin/maestro-runner" >&2
        exit 1
    fi

    cat > "$dest/package.json" <<EOF
{
  "name": "$pkg",
  "version": "$VERSION",
  "description": "maestro-runner binary for ${node_os} ${node_arch}. Installed automatically as an optional dependency of maestro-runner.",
  "homepage": "https://open.devicelab.dev/maestro-runner",
  "repository": {
    "type": "git",
    "url": "git+https://github.com/devicelab-dev/maestro-runner.git"
  },
  "license": "Apache-2.0",
  "author": "DeviceLab (https://devicelab.dev)",
  "os": ["$node_os"],
  "cpu": ["$node_arch"],
  "files": ["bin/", "drivers/", "LICENSE", "THIRD_PARTY_LICENSES"],
  "preferUnplugged": true
}
EOF
done

# Stamp the version into the main package and the optional dependencies that
# must resolve to the exact same build.
node - "$NPM_DIR/maestro-runner/package.json" "$VERSION" <<'NODE'
const fs = require('node:fs');
const [file, version] = process.argv.slice(2);
const pkg = JSON.parse(fs.readFileSync(file, 'utf8'));
pkg.version = version;
for (const dep of Object.keys(pkg.optionalDependencies)) {
  pkg.optionalDependencies[dep] = version;
}
fs.writeFileSync(file, JSON.stringify(pkg, null, 2) + '\n');
NODE

echo
echo "Built npm packages for $VERSION"
echo
echo "Verify locally before publishing:"
echo "  npm pack --dry-run --workspaces=false --prefix npm/maestro-runner"
echo "  node npm/maestro-runner/bin/maestro-runner.js --version   # after linking a platform package"
echo
echo "Publish — platform packages first, so the main package never resolves to a version that does not exist:"
for target in "${TARGETS[@]}"; do
    read -r _ _ node_os node_arch <<< "$target"
    echo "  npm publish npm/platforms/maestro-runner-${node_os}-${node_arch} --access public"
done
echo "  npm publish npm/maestro-runner --access public"
