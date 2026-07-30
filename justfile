# screenjack - display takeover toolkit

mod orchestra
mod payload

# List all recipes
[group('meta')]
default:
    @just --list --unsorted

# Build everything
[group('meta')]
build: orchestra::build payload::release
    @echo "Build complete"

# Clean all artifacts
[group('meta')]
clean: orchestra::clean payload::clean

# ─────────────────────────────────────────────────────────────────────────────
# Cross-compilation shortcuts
# ─────────────────────────────────────────────────────────────────────────────

# Build all payload targets (linux + windows)
[group('build')]
build-all: payload::all
    @echo "Payloads in dist/"

# Build linux static payload
[group('build')]
build-linux: payload::linux

# Build windows payload
[group('build')]
build-windows: payload::windows

# ─────────────────────────────────────────────────────────────────────────────
# Run shortcuts
# ─────────────────────────────────────────────────────────────────────────────

# Run the TUI orchestrator
[group('run')]
tui: orchestra::run

# Test payload with an image
[group('run')]
test-payload IMAGE="": payload::release
    ./payload/target/release/screenjack {{IMAGE}}
