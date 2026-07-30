# Payload (Rust screenjack binary)

set working-directory := "payload"

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
    SCREENJACK_ASSET="{{ASSET}}" cargo build --release --features embedded
    mkdir -p ../dist
    cp target/release/screenjack ../dist/screenjack-linux

# Cross-compile for windows + copy to dist
windows:
    cargo build --release --target x86_64-pc-windows-gnu
    mkdir -p ../dist
    cp target/x86_64-pc-windows-gnu/release/screenjack.exe ../dist/screenjack.exe

# Windows with embedded asset
windows-embedded ASSET:
    SCREENJACK_ASSET="{{ASSET}}" cargo build --release --target x86_64-pc-windows-gnu --features embedded
    mkdir -p ../dist
    cp target/x86_64-pc-windows-gnu/release/screenjack.exe ../dist/screenjack.exe

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
    rustup target add x86_64-unknown-linux-musl
    rustup target add x86_64-pc-windows-gnu
    @echo "Also run: sudo pacman -S mingw-w64-gcc"
