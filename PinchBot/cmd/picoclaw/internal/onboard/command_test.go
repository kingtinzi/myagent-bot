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
	if _, err := os.Stat(filepath.Join("..", "..", "..", "..", "workspace")); !os.IsNotExist(err) {
		t.Fatal("expected legacy PinchBot/workspace directory to be removed so onboarding has a single canonical template source")
	}
}
