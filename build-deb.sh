#!/bin/bash
# Script to build .deb package from crocgui.tar.xz

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cd "$SCRIPT_DIR"

echo "=== Building .deb package for crocgui ==="

# Check if required files exist
if [ ! -f "crocgui.tar.xz" ]; then
    echo "ERROR: crocgui.tar.xz not found in current directory"
    exit 1
fi

if [ ! -f "FyneApp.toml" ]; then
    echo "ERROR: FyneApp.toml not found in current directory"
    exit 1
fi

# Read version from FyneApp.toml
VERSION=$(grep -E '^\s*Version\s*=' FyneApp.toml | sed -E 's/^\s*Version\s*=\s*"([^"]+)".*/\1/')
if [ -z "$VERSION" ]; then
    echo "ERROR: Could not extract Version from FyneApp.toml."
    cat FyneApp.toml
    exit 1
fi

echo "Version from FyneApp.toml: $VERSION"

# Clean up existing .deb file
rm -f "crocgui_*.deb"

# Create directory structure for .deb package
DEB_DIR="crocgui_${VERSION}_amd64"
echo "Creating .deb structure in: $DEB_DIR"

# Clean up existing directory
rm -rf "$DEB_DIR"

# Create directory structure
mkdir -p "$DEB_DIR/DEBIAN"
mkdir -p "$DEB_DIR/usr/share/crocgui"

# Copy the original tar.xz archive to the package
cp crocgui.tar.xz "$DEB_DIR/usr/share/crocgui/"

# Create control file with correct version
echo "Creating control file with version $VERSION..."
cp "DEBIAN/control" "$DEB_DIR/DEBIAN/control"
sed -i "s/^Version: .*/Version: $VERSION/" "$DEB_DIR/DEBIAN/control"

# Copy maintenance scripts
echo "Copying maintenance scripts..."
cp "DEBIAN/postinst" "$DEB_DIR/DEBIAN/"
cp "DEBIAN/prerm" "$DEB_DIR/DEBIAN/"
cp "DEBIAN/postrm" "$DEB_DIR/DEBIAN/"

# Set executable permissions
chmod +x "$DEB_DIR/DEBIAN/postinst"
chmod +x "$DEB_DIR/DEBIAN/prerm"
chmod +x "$DEB_DIR/DEBIAN/postrm"

# Build the .deb package with desired filename
echo "Building .deb package..."
dpkg-deb --build --root-owner-group "$DEB_DIR" "crocgui_${VERSION}_amd64.deb"

echo ""
echo "=== Package created successfully! ==="
echo "File: crocgui_${VERSION}_amd64.deb"
echo "Size: $(du -h crocgui_${VERSION}_amd64.deb | cut -f1)"

# Verify the package
echo ""
echo "Package contents:"
dpkg -c "crocgui_${VERSION}_amd64.deb" | head -20

echo ""
echo "Package info:"
dpkg -I "crocgui_${VERSION}_amd64.deb"

# Clean up temporary directory
rm -rf "$DEB_DIR"

echo ""
echo "To install:"
echo "sudo dpkg -i crocgui_${VERSION}_amd64.deb"

