package migrate

import (
	"strings"
	"testing"
)

func TestNewMigrateCommandShape(t *testing.T) {
	cmd := NewMigrateCommand()

	if cmd == nil {
		t.Fatal("expected command to be created")
	}
	if cmd.Use != "migrate" {
		t.Fatalf("cmd.Use = %q, want %q", cmd.Use, "migrate")
	}
	if cmd.Short != "Migrate from xxxclaw(openclaw, etc.) to PinchBot" {
		t.Fatalf("cmd.Short = %q", cmd.Short)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE to be configured")
	}

	for _, flagName := range []string{
		"dry-run",
		"from",
		"refresh",
		"config-only",
		"workspace-only",
		"force",
		"source-home",
		"target-home",
	} {
		if flag := cmd.Flags().Lookup(flagName); flag == nil {
			t.Fatalf("expected %q flag to exist", flagName)
		}
	}
}

func TestMigrateCommandMentionsCanonicalDefaultTargetHome(t *testing.T) {
	cmd := NewMigrateCommand()
	flag := cmd.Flags().Lookup("target-home")
	if flag == nil {
		t.Fatal("expected target-home flag to exist")
	}
	if !strings.Contains(flag.Usage, ".pinchbot") {
		t.Fatalf("expected target-home help to mention canonical .pinchbot home, got %q", flag.Usage)
	}
}
