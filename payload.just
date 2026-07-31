# Payload (Rust screenjack binary)

set working-directory := "payload"
set shell := ["bash", "-cu"]

# Windows PowerShell for windows-native recipe
set windows-shell := ["powershell.exe", "-NoLogo", "-Command"]

# Build debug
build:
    cargo build

# Build release
release:
    cargo build --release

# Build linux release + copy to dist
linux:
    cargo build --release
    mkdir -p ../dist
    cp target/release/screenjack ../dist/screenjack-linux

# Build linux with embedded asset
linux-embedded ASSET:
    SCREENJACK_ASSET="{{ absolute_path(ASSET) }}" cargo build --release --features embedded
    mkdir -p ../dist
    cp target/release/screenjack ../dist/screenjack-linux

# Cross-compile for windows + copy to dist
windows:
    cargo build --release --target x86_64-pc-windows-gnu
    mkdir -p ../dist
    cp target/x86_64-pc-windows-gnu/release/screenjack.exe ../dist/screenjack.exe

# Windows with embedded asset (cross-compile from Linux)
windows-embedded ASSET:
    SCREENJACK_ASSET="{{ absolute_path(ASSET) }}" cargo build --release --target x86_64-pc-windows-gnu --features embedded
    mkdir -p ../dist
    cp target/x86_64-pc-windows-gnu/release/screenjack.exe ../dist/screenjack.exe

# Native Windows build (run on Windows)
[windows]
windows-native:
    cargo build --release
    if (!(Test-Path ../dist)) { New-Item -ItemType Directory -Path ../dist }
    Copy-Item target/release/screenjack.exe ../dist/screenjack.exe

# Build static linux (musl) - works on Alpine/any distro
linux-static:
    cargo build --release --target x86_64-unknown-linux-musl
    mkdir -p ../dist
    cp target/x86_64-unknown-linux-musl/release/screenjack ../dist/screenjack-static

# Static linux with embedded asset
linux-static-embedded ASSET:
    SCREENJACK_ASSET="{{ absolute_path(ASSET) }}" cargo build --release --target x86_64-unknown-linux-musl --features embedded
    mkdir -p ../dist
    cp target/x86_64-unknown-linux-musl/release/screenjack ../dist/screenjack-static

# Build all targets
all: linux linux-static windows

# Run with optional image
run IMAGE="":
    cargo run --release -- {{IMAGE}}

# Check without building
check:
    cargo check

# Clean artifacts
clean:
    cargo clean
    rm -rf ../dist

# Setup cross-compile toolchain
setup:
    rustup target add x86_64-pc-windows-gnu
    rustup target add x86_64-unknown-linux-musl
    @echo "Also run: sudo pacman -S mingw-w64-gcc musl"

# ─────────────────────────────────────────────────────────────────────────────
# Docker cross-compile (no local toolchain needed)
# ─────────────────────────────────────────────────────────────────────────────

# Build via Alpine (musl static - portable, works everywhere)
docker-alpine:
    #!/usr/bin/env bash
    set -euo pipefail
    docker build -f ../docker/Dockerfile.alpine -t screenjack-alpine ..
    mkdir -p ../dist
    docker run --rm screenjack-alpine cat /src/target/release/screenjack > ../dist/screenjack-alpine
    chmod +x ../dist/screenjack-alpine
    echo "[+] dist/screenjack-alpine (static musl)"

# Build via Debian (glibc)
docker-debian:
    #!/usr/bin/env bash
    set -euo pipefail
    docker build -f ../docker/Dockerfile.debian -t screenjack-debian ..
    mkdir -p ../dist
    docker run --rm screenjack-debian cat /src/target/release/screenjack > ../dist/screenjack-debian
    chmod +x ../dist/screenjack-debian
    echo "[+] dist/screenjack-debian (glibc)"

# Build via Arch (glibc, matches Arch hosts)
docker-arch:
    #!/usr/bin/env bash
    set -euo pipefail
    docker build -f ../docker/Dockerfile.arch -t screenjack-arch ..
    mkdir -p ../dist
    docker run --rm screenjack-arch cat /src/target/release/screenjack > ../dist/screenjack-arch
    chmod +x ../dist/screenjack-arch
    echo "[+] dist/screenjack-arch (glibc)"

# Cross-compile for Windows via Docker
docker-windows:
    #!/usr/bin/env bash
    set -euo pipefail
    docker build -f ../docker/Dockerfile.windows -t screenjack-windows ..
    mkdir -p ../dist
    docker run --rm screenjack-windows cat /src/target/x86_64-pc-windows-gnu/release/screenjack.exe > ../dist/screenjack.exe
    echo "[+] dist/screenjack.exe"

# Build all via Docker
docker-all: docker-alpine docker-debian docker-windows
    @echo "[+] All Docker builds complete"
    @ls -lh ../dist/
