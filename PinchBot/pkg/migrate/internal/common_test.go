package internal

import (
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/sipeed/pinchbot/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandHome(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		result := ExpandHome(tt.input)
		assert.Equal(t, tt.expected, result)
	}
}

func TestExpandHomeWithTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	result := ExpandHome("~/path")
	assert.Equal(t, filepath.Join(home, "path"), result)

	result = ExpandHome("~")
	assert.Equal(t, home, result)
}

func TestResolveWorkspace(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "user", ".PinchBot")
	result := ResolveWorkspace(home)
	assert.Equal(t, filepath.Join(home, "workspace"), result)
}

func TestRelPath(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "home", "user", ".PinchBot")
	result := RelPath(filepath.Join(base, "workspace", "file.txt"), base)
	assert.Equal(t, filepath.Join("workspace", "file.txt"), result)
}

func TestRelPathError(t *testing.T) {
	result := RelPath("relative/path", "/different/base")
	assert.Equal(t, "path", result)
}

func TestResolveTargetHome(t *testing.T) {
	result, err := ResolveTargetHome("")
	require.NoError(t, err)
	assert.Equal(t, appconfig.GetPinchBotHome(), result)
}

func TestResolveTargetHomeWithOverride(t *testing.T) {
	expected := filepath.Join(t.TempDir(), "custom", "path")
	result, err := ResolveTargetHome(expected)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestResolveTargetHomeWithPinchBotHomeEnv(t *testing.T) {
	expected := filepath.Join(t.TempDir(), "env", "path")
	t.Setenv("PINCHBOT_HOME", expected)

	result, err := ResolveTargetHome("")
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestResolveTargetHomeWithLegacyHomeEnv(t *testing.T) {
	expected := filepath.Join(t.TempDir(), "legacy", "path")
	t.Setenv("PICOCLAW_HOME", expected)

	result, err := ResolveTargetHome("")
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestResolveTargetHomeExpandsTildeEnv(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	t.Setenv(appconfig.PinchBotHomeEnv, "~/pinchbot-home")
	t.Setenv(appconfig.LegacyHomeEnv, "")

	result, err := ResolveTargetHome("")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(homeDir, "pinchbot-home"), result)
}

func TestResolveTargetHomeUsesCanonicalPinchBotHomeByDefault(t *testing.T) {
	t.Setenv(appconfig.PinchBotHomeEnv, "")
	t.Setenv(appconfig.LegacyHomeEnv, "")

	result, err := ResolveTargetHome("")
	require.NoError(t, err)
	assert.Equal(t, appconfig.GetPinchBotHome(), result)
}

func TestResolveTargetHomeTrimsAndCleansEnvHome(t *testing.T) {
	t.Setenv(appconfig.PinchBotHomeEnv, "  ./custom-home/../canonical-home  ")

	result, err := ResolveTargetHome("")
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(result), "expected absolute home path, got %q", result)
	assert.Equal(t, "canonical-home", filepath.Base(result))
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	sourceFile := filepath.Join(tmpDir, "source.txt")
	err := os.WriteFile(sourceFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	dstFile := filepath.Join(tmpDir, "dest.txt")
	err = CopyFile(sourceFile, dstFile)
	require.NoError(t, err)

	content, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, "test content", string(content))
}

func TestCopyFileSourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	err := CopyFile(filepath.Join(tmpDir, "nonexistent.txt"), filepath.Join(tmpDir, "dest.txt"))
	require.Error(t, err)
}

func TestPlanWorkspaceMigration(t *testing.T) {
	tmpDir := t.TempDir()
	srcWorkspace := filepath.Join(tmpDir, "src", "workspace")
	dstWorkspace := filepath.Join(tmpDir, "dst", "workspace")

	err := os.MkdirAll(srcWorkspace, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(srcWorkspace, "file1.txt"), []byte("content"), 0o644)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(srcWorkspace, "subdir"), 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(srcWorkspace, "subdir", "file2.txt"), []byte("content"), 0o644)
	require.NoError(t, err)

	actions, err := PlanWorkspaceMigration(
		srcWorkspace,
		dstWorkspace,
		[]string{"file1.txt"},
		[]string{"subdir"},
		false,
	)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(actions), 1)
}

func TestPlanWorkspaceMigrationExistingFile(t *testing.T) {
	tests := []struct {
		name           string
		force          bool
		wantActionType ActionType
	}{
		{
			name:           "backup when not forced",
			force:          false,
			wantActionType: ActionBackup,
		},
		{
			name:           "copy when forced",
			force:          true,
			wantActionType: ActionCopy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			srcWorkspace := filepath.Join(tmpDir, "src", "workspace")
			dstWorkspace := filepath.Join(tmpDir, "dst", "workspace")

			err := os.MkdirAll(srcWorkspace, 0o755)
			require.NoError(t, err)

			err = os.MkdirAll(dstWorkspace, 0o755)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(srcWorkspace, "file1.txt"), []byte("source"), 0o644)
			require.NoError(t, err)

			err = os.WriteFile(filepath.Join(dstWorkspace, "file1.txt"), []byte("existing"), 0o644)
			require.NoError(t, err)

			actions, err := PlanWorkspaceMigration(
				srcWorkspace,
				dstWorkspace,
				[]string{"file1.txt"},
				[]string{},
				tt.force,
			)
			require.NoError(t, err)

			require.GreaterOrEqual(t, len(actions), 1)
			assert.Equal(t, tt.wantActionType, actions[0].Type)
		})
	}
}

func TestPlanWorkspaceMigrationNonExistentSource(t *testing.T) {
	tmpDir := t.TempDir()

	actions, err := PlanWorkspaceMigration(
		filepath.Join(tmpDir, "nonexistent"),
		filepath.Join(tmpDir, "dst", "workspace"),
		[]string{"file1.txt"},
		[]string{},
		false,
	)
	require.NoError(t, err)
	require.Len(t, actions, 1)
	assert.Equal(t, ActionSkip, actions[0].Type)
	assert.Contains(t, actions[0].Description, "source file not found")
}
