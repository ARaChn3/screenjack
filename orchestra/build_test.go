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
