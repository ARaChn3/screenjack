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

# Build all targets
all: linux windows

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
    @echo "Also run: sudo pacman -S mingw-w64-gcc"
