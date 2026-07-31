# Troubleshooting

## Docker Builds

### "operation not supported" / veth bridge errors

```
failed to create endpoint ... on network bridge: failed to add the host (veth...) <=> sandbox (veth...) pair interfaces: operation not supported
```

**Fix:**
```bash
# Restart Docker daemon
sudo systemctl restart docker

# Prune stale networks
docker network prune -f

# Retry build
just -f payload.just docker-alpine
```

If it persists, verify Docker networking:
```bash
docker run --rm alpine echo "ok"
```

### Build hangs or times out

Docker pulls can be slow. First build downloads base images (~200MB for Alpine, ~500MB for Debian).

**Fix:** Run with verbose output:
```bash
docker build -f docker/Dockerfile.alpine -t screenjack-alpine . --progress=plain
```

### Permission denied on extracted binary

```bash
chmod +x dist/screenjack-alpine
```

## Native Builds

### musl target not found

```bash
rustup target add x86_64-unknown-linux-musl
sudo pacman -S musl  # Arch
```

### Windows cross-compile fails

```bash
rustup target add x86_64-pc-windows-gnu
sudo pacman -S mingw-w64-gcc  # Arch
```

### libxcb not found

```bash
# Arch
sudo pacman -S libxcb

# Debian/Ubuntu
sudo apt install libxcb1-dev libxcb-shm0-dev
```

## TUI Issues

### Payload not found after build

Check the target path matches your build:
- Native Linux: `payload/target/release/screenjack`
- Native Windows: `payload/target/x86_64-pc-windows-gnu/release/screenjack.exe`
- Docker builds: `dist/screenjack-alpine`, `dist/screenjack-debian`, etc.

### Config not saving

Config path: `~/.config/screenjack/config.json`

Ensure the directory exists:
```bash
mkdir -p ~/.config/screenjack
```
