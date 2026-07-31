# screenjack

Fullscreen display takeover + input lock for authorized security auditing.

## Requirements

**Build machine (Arch/Linux):**
```bash
# Rust + Go
sudo pacman -S rust go just

# Cross-compile for Windows
sudo pacman -S mingw-w64-gcc
rustup target add x86_64-pc-windows-gnu

# Static musl builds (optional - or use docker-alpine)
sudo pacman -S musl
rustup target add x86_64-unknown-linux-musl
```

**Target machine:**
- Linux: X11 + libxcb (`sudo pacman -S libxcb` / `apt install libxcb1`)
- Windows: No deps (static binary)

## Structure

```
screenjack/
├── orchestra/     # Go TUI (build payloads, gen ducky scripts)
├── payload/       # Rust binary (screenjack payload)
├── dist/          # Built payloads for deployment
├── ducky/         # Generated Rubber Ducky scripts
└── assets/        # GIFs/images to display
```

## Quick Start

```bash
# Setup (first time)
just payload::setup

# Build everything
just build-all

# Run TUI
just tui
```

## Build Commands

```bash
just build-linux     # Linux payload -> dist/screenjack-linux
just build-windows   # Windows payload -> dist/screenjack.exe
just build-all       # Both targets
just tui             # Run orchestra TUI

# Docker builds (no local toolchain needed)
just -f payload.just docker-alpine   # Static musl binary
just -f payload.just docker-debian   # glibc binary
```

## Payload Usage

```bash
# With asset
./screenjack-linux /path/to/image.gif

# Exit: hold Ctrl+Shift+Escape for 2 seconds
```

## TUI Controls

| Key | Action |
|-----|--------|
| `tab` | Switch section |
| `j/k` | Navigate |
| `space` | Select/toggle |
| `b` | Build payloads |
| `g` | Generate ducky script |
| `p` | Preview asset (ASCII) |
| `o` | Open assets folder |
| `l` | View build logs |
| `q` | Quit |

## Ducky Deployment

Generated scripts in `ducky/` download and execute payload:
- `payload_windows.txt` - PowerShell download + exec
- `payload_linux.txt` - curl + exec

Configure URL/payload name in TUI before generating.

## License

MIT
