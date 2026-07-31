# Screenjack UX Improvement Plan

## Current State (What We Know)

### Payload Capabilities
- Fullscreen takeover (X11 + Windows, multi-monitor)
- GIF/image display with animation
- Keyboard/mouse blocking
- Exit combo: Ctrl+Shift+Escape held 2s
- Embedded asset support (`--features embedded` + `SCREENJACK_ASSET` env)
- Persistence (`--persist` / `--unpersist`)
- Linux: VT switch blocking, magic sysrq disable (requires root)
- Windows: blocks Alt+Tab, Win key, Alt+F4, Ctrl+Esc

### TUI Structure (4 tabs)
1. **Build** - Select targets (linux-x86_64, windows-x86_64), build button
2. **Ducky** - Generate ducky scripts (OS toggle, URL, payload name)
3. **Server** - HTTP server for payload delivery
4. **Package** - RustRedOps options (execution, evasion, persistence, encryption)

### Package Tab Options (Current)
```
Execution Methods:
- Raw Binary (cross-platform)
- Process Ghosting [Win]
- Process Hollowing [Win]
- Process Herpaderping [Win]
- APC Injection [Win]
- Thread Hijacking [Win]
- Threadless Injection [Win]
- Module Stomping [Win]

Evasion:
- AMSI Bypass [Win]
- ETW Patch [Win]
- NTDLL Unhook [Win]
- PPID Spoof [Win]
- Block DLL Policy [Win]
- Anti-Debug [Win]
- Anti-Analysis [Win]
- Direct Syscalls [Win]

Persistence:
- None
- Registry Run [Win]
- XDG Autostart [Lin]
- Self-Delete [Win]

Encryption:
- AES Encrypt (cross-platform)
```

## Problem Statement

Current UX separates "Build" and "Package" into different tabs. User must:
1. Go to Build tab, select targets
2. Go to Package tab, configure options
3. Press build in Package tab
4. Somehow remember which options apply to which platform

This is confusing because Package options are platform-specific but the UI doesn't enforce this.

## Proposed Solution: Unified Build with "b" Keybind

Single build action that:
1. Reads selected targets from Build tab
2. Reads Package options from Package tab
3. Applies options conditionally based on target platform:
   - Windows-only options → only applied to Windows build
   - Linux-only options → only applied to Linux build
   - Cross-platform options → applied to both

## Decision Tree (DESIGNED - see specs/2026-07-31-unified-build-design.md)

```
"b" pressed
├─ VALIDATE FIRST (fail fast)
├─ QUEUE CHECK (auto-queue if busy, max 1)
├─ BUILD (parallel per target with platform filtering)
└─ FINAL STATUS (per-target reporting)

Cancel: "c" key
Log expand: "l" key
```

Key decisions:
- Validate before queue check (fail fast)
- Auto-queue replaces existing queue (newest wins)
- Smart suggestion for platform mismatch
- Per-target progress bars + collapsible log
- Snapshot options at queue time

## Tasks

### Quick Wins (TUI-only)
- [ ] #1 Add persist toggle to Ducky tab
- [ ] #2 Add ducky script preview panel

### Medium Effort
- [ ] #3 Add build progress spinner
- [ ] #4 Add asset preview in modal
- [ ] #7 Add local payload test button

### Larger Features
- [ ] #5 Add audio playback support to payload
- [ ] #6 Add audio asset selection to TUI (blocked by #5)

### Unified Build System (This Brainstorm)
- [ ] #8-12 Design and implement unified build with conditional packaging

## Resolved Questions

1. ~~Should Package tab be merged into Build tab?~~ → Stay separate, "b" reads from both
2. ~~UX for Windows-only options + no Windows target?~~ → Smart suggestion + warn if declined
3. ~~Validation/warnings for incompatible combos?~~ → Yes, validate first (fail fast)
4. ~~Progress UI?~~ → Per-target progress bars + collapsible log stream
