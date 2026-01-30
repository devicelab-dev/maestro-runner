#!/bin/bash
# Installation script for maestro-runner
# Handles macOS Gatekeeper issues and installs to user's PATH

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "Installing maestro-runner..."
echo ""

# Detect OS
OS="$(uname -s)"
ARCH="$(uname -m)"

if [ "$OS" != "Darwin" ] && [ "$OS" != "Linux" ]; then
    echo -e "${RED}Error: Unsupported operating system: $OS${NC}"
    echo "maestro-runner currently supports macOS and Linux only."
    exit 1
fi

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed${NC}"
    echo "Please install Go from https://golang.org/dl/"
    exit 1
fi

echo -e "${GREEN}✓${NC} Go found: $(go version)"
echo ""

# Build from source
echo "Building maestro-runner from source..."
go build -o maestro-runner .

if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Build failed${NC}"
    exit 1
fi

echo -e "${GREEN}✓${NC} Build successful"
echo ""

# Determine install location
INSTALL_DIR="$HOME/.local/bin"
if [ ! -d "$INSTALL_DIR" ]; then
    echo "Creating $INSTALL_DIR..."
    mkdir -p "$INSTALL_DIR"
fi

# Move binary
echo "Installing to $INSTALL_DIR..."
mv maestro-runner "$INSTALL_DIR/maestro-runner"
chmod +x "$INSTALL_DIR/maestro-runner"

# macOS specific: Remove quarantine attribute
if [ "$OS" = "Darwin" ]; then
    echo "Removing macOS quarantine attribute..."
    xattr -d com.apple.quarantine "$INSTALL_DIR/maestro-runner" 2>/dev/null || true
    echo -e "${GREEN}✓${NC} macOS Gatekeeper bypass applied"
fi

echo -e "${GREEN}✓${NC} Installed to $INSTALL_DIR/maestro-runner"
echo ""

# Check if install dir is in PATH
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo -e "${YELLOW}Warning: $INSTALL_DIR is not in your PATH${NC}"
    echo ""
    echo "Add this line to your shell profile (~/.bashrc, ~/.zshrc, or ~/.profile):"
    echo ""
    echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
    echo "Then restart your shell or run: source ~/.zshrc"
    echo ""
else
    echo -e "${GREEN}✓${NC} Installation complete!"
    echo ""
    echo "Run: maestro-runner --help"
fi
