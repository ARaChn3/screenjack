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

func TestRequiresWindows(t *testing.T) {
	tests := []struct {
		name string
		cfg  PackageConfig
		want bool
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

func TestOptionsForTarget(t *testing.T) {
	cfg := PackageConfig{
		ExecMethod:    1,                                                             // Ghosting (Windows)
		Evasion:       []bool{true, false, false, false, false, false, false, false}, // AMSI (Windows)
		PersistMethod: 2,                                                             // XDG (Linux)
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
