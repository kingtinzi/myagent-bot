package plugins

import "testing"

func TestNodeVersionAtLeast(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{raw: "v22.12.0", want: true},
		{raw: "18.0.0", want: true},
		{raw: "v17.9.1", want: false},
		{raw: "invalid", want: false},
		{raw: "", want: false},
	}
	for _, tt := range tests {
		if got := nodeVersionAtLeast(tt.raw, 18); got != tt.want {
			t.Fatalf("nodeVersionAtLeast(%q, 18) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestProbeRuntimeForPlugin_NonLobster(t *testing.T) {
	status, checks, repairs := probeRuntimeForPlugin("echo-fixture")
	if status != "" {
		t.Fatalf("status = %q, want empty", status)
	}
	if len(checks) != 0 {
		t.Fatalf("checks = %v, want empty", checks)
	}
	if len(repairs) != 0 {
		t.Fatalf("repairs = %v, want empty", repairs)
	}
}

