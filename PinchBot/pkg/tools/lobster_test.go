package tools

import (
	"path/filepath"
	"testing"
)

func TestBuildLobsterArgv_Run(t *testing.T) {
	argv, err := buildLobsterArgv("run", map[string]any{
		"pipeline": "demo.pipeline",
		"argsJson": `{"k":"v"}`,
	})
	if err != nil {
		t.Fatalf("buildLobsterArgv(run) error: %v", err)
	}
	want := []string{"run", "--mode", "tool", "demo.pipeline", "--args-json", `{"k":"v"}`}
	if len(argv) != len(want) {
		t.Fatalf("argv len=%d want=%d", len(argv), len(want))
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv[%d]=%q want=%q", i, argv[i], want[i])
		}
	}
}

func TestBuildLobsterArgv_Resume(t *testing.T) {
	argv, err := buildLobsterArgv("resume", map[string]any{
		"token":   "abc",
		"approve": true,
	})
	if err != nil {
		t.Fatalf("buildLobsterArgv(resume) error: %v", err)
	}
	want := []string{"resume", "--token", "abc", "--approve", "yes"}
	if len(argv) != len(want) {
		t.Fatalf("argv len=%d want=%d", len(argv), len(want))
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv[%d]=%q want=%q", i, argv[i], want[i])
		}
	}
}

func TestParseLobsterEnvelope_WithPrefixNoise(t *testing.T) {
	parsed, err := parseLobsterEnvelope("warn\n{\"ok\":true,\"status\":\"ok\"}")
	if err != nil {
		t.Fatalf("parseLobsterEnvelope error: %v", err)
	}
	ok, _ := parsed["ok"].(bool)
	if !ok {
		t.Fatalf("expected ok=true, got %v", parsed["ok"])
	}
}

func TestResolveCwd_RejectEscapeWhenRestricted(t *testing.T) {
	base := t.TempDir()
	tool := &lobsterTool{workspace: base, restrict: true}
	_, err := tool.resolveCwd("../../etc")
	if err == nil {
		t.Fatal("expected error for cwd escaping workspace")
	}
}

func TestResolveCwd_AllowsNestedPath(t *testing.T) {
	base := t.TempDir()
	tool := &lobsterTool{workspace: base, restrict: true}
	got, err := tool.resolveCwd("sub/dir")
	if err != nil {
		t.Fatalf("resolveCwd error: %v", err)
	}
	want := filepath.Join(base, "sub", "dir")
	if got != want {
		t.Fatalf("resolveCwd=%q want=%q", got, want)
	}
}
