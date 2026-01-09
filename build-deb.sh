#!/usr/bin/env bash
# .deb package builder with clean output

set -euo pipefail

# Get version from FyneApp.toml
VERSION=$(grep -E '^\s*Version\s*=' FyneApp.toml | sed -E 's/^\s*Version\s*=\s*"([^"]+)".*/\1/')
NAME="crocgui_${VERSION}_amd64"
DEB_FILE="${NAME}.deb"

echo "=== Building .deb package ==="
echo "Version: $VERSION"
echo "Package: $DEB_FILE"

# Cleanup and create structure
rm -f crocgui_*.deb 2>/dev/null || true
rm -rf "$NAME" 2>/dev/null || true
mkdir -p "$NAME/DEBIAN" "$NAME/usr/share/crocgui"

# Copy files
cp crocgui.tar.xz "$NAME/usr/share/crocgui/"
cp DEBIAN/control "$NAME/DEBIAN/"
sed -i "s/^Version:.*/Version: $VERSION/" "$NAME/DEBIAN/control"

# Optional scripts
for script in postinst prerm postrm; do
    [ -f "DEBIAN/$script" ] && cp "DEBIAN/$script" "$NAME/DEBIAN/" && chmod +x "$NAME/DEBIAN/$script"
done

# Build package
echo "Building package..."
dpkg-deb --build --root-owner-group "$NAME" "$DEB_FILE"
rm -rf "$NAME"

echo ""
echo "=== Package created ==="
echo "File: $DEB_FILE"
echo "Size: $(du -h "$DEB_FILE" | cut -f1)"

# Show package metadata (без дублей)
echo ""
echo "Package metadata:"
echo "-----------------"
dpkg -I "$DEB_FILE" 2>/dev/null | awk '!seen[$0]++' | head -20

# Show package contents (без дублей)
echo ""
echo "Package contents:"
echo "-----------------"
dpkg -c "$DEB_FILE" 2>/dev/null | awk '!seen[$0]++'

echo ""
echo "=== Verification ==="
# Verify key files exist in package
if dpkg -c "$DEB_FILE" 2>/dev/null | grep -q "crocgui.tar.xz"; then
    echo "✅ crocgui.tar.xz included"
else
    echo "❌ ERROR: crocgui.tar.xz missing"
    exit 1
fi

if dpkg -I "$DEB_FILE" 2>/dev/null | grep -q "Version: $VERSION"; then
    echo "✅ Version matches: $VERSION"
else
    echo "❌ ERROR: Version mismatch"
    exit 1
fi

echo ""
echo "✅ Done. Package built successfully."