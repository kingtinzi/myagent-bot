package onboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOnboardCommand(t *testing.T) {
	cmd := NewOnboardCommand()

	require.NotNil(t, cmd)

	assert.Equal(t, "onboard", cmd.Use)
	assert.Equal(t, "Initialize PinchBot configuration and workspace", cmd.Short)

	assert.Len(t, cmd.Aliases, 1)
	assert.True(t, cmd.HasAlias("o"))

	assert.NotNil(t, cmd.Run)
	assert.Nil(t, cmd.RunE)

	assert.Nil(t, cmd.PersistentPreRun)
	assert.Nil(t, cmd.PersistentPostRun)

	assert.False(t, cmd.HasFlags())
	assert.False(t, cmd.HasSubCommands())
}

func TestOnboardCommandDoesNotUseGenerateCopyStep(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("command.go"))
	if err != nil {
		t.Fatalf("ReadFile(command.go) error = %v", err)
	}
	if strings.Contains(string(content), "go:generate") {
		t.Fatal("expected onboard command to stop using go:generate copy steps for workspace templates")
	}
}

func TestLegacyRepoWorkspaceDirectoryIsAbsent(t *testing.T) {
	legacyWorkspace := filepath.Join("..", "..", "..", "..", "workspace")

	// In clean checkouts this directory should not exist at all.
	// In local/dev runs a runtime workspace directory may appear (sessions/memory),
	// so the real invariant is: no legacy template source files are allowed here.
	if _, err := os.Stat(legacyWorkspace); os.IsNotExist(err) {
		return
	} else if err != nil {
		t.Fatalf("stat legacy workspace directory failed: %v", err)
	}

	forbiddenTemplateArtifacts := []string{
		"AGENTS.md",
		"IDENTITY.md",
		"SOUL.md",
		"USER.md",
		"skills",
		filepath.Join("memory", "MEMORY.md"),
	}

	for _, rel := range forbiddenTemplateArtifacts {
		path := filepath.Join(legacyWorkspace, rel)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf(
				"found legacy template artifact at %s; onboarding must use internal/workspacetpl as the single canonical source",
				path,
			)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat legacy template artifact %s failed: %v", path, err)
		}
	}
}
