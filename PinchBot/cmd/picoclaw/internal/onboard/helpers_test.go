package onboard

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCopyEmbeddedToTargetUsesAgentsMarkdown(t *testing.T) {
	targetDir := t.TempDir()

	if err := copyEmbeddedToTarget(targetDir); err != nil {
		t.Fatalf("copyEmbeddedToTarget() error = %v", err)
	}

	agentsPath := filepath.Join(targetDir, "AGENTS.md")
	if _, err := os.Stat(agentsPath); err != nil {
		t.Fatalf("expected %s to exist: %v", agentsPath, err)
	}

	legacyPath := filepath.Join(targetDir, "AGENT.md")
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy file %s to be absent, got err=%v", legacyPath, err)
	}
}

func TestCopyEmbeddedToTargetIncludesEmailSkill(t *testing.T) {
	targetDir := t.TempDir()

	if err := copyEmbeddedToTarget(targetDir); err != nil {
		t.Fatalf("copyEmbeddedToTarget() error = %v", err)
	}

	emailSkillPath := filepath.Join(targetDir, "skills", "email", "SKILL.md")
	if _, err := os.Stat(emailSkillPath); err != nil {
		t.Fatalf("expected %s to exist: %v", emailSkillPath, err)
	}
}

func TestCopyEmbeddedToTargetUsesSanitizedUserTemplates(t *testing.T) {
	targetDir := t.TempDir()

	if err := copyEmbeddedToTarget(targetDir); err != nil {
		t.Fatalf("copyEmbeddedToTarget() error = %v", err)
	}

	userContent, err := os.ReadFile(filepath.Join(targetDir, "USER.md"))
	if err != nil {
		t.Fatalf("ReadFile(USER.md) error = %v", err)
	}
	if strings.Contains(string(userContent), "dingtalk:") || strings.Contains(string(userContent), "@dingtalk.com") {
		t.Fatal("expected onboard USER.md template to avoid tracked personal identifiers")
	}

	emailSkillContent, err := os.ReadFile(filepath.Join(targetDir, "skills", "email", "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile(email skill) error = %v", err)
	}
	if strings.Contains(string(emailSkillContent), "无需再向用户确认") {
		t.Fatal("expected email skill to require confirmation unless a real workspace-specific recipient is known")
	}
}

func TestCopyEmbeddedToTargetMakesShellHelpersExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable mode bits are not reliable on Windows")
	}

	targetDir := t.TempDir()
	if err := copyEmbeddedToTarget(targetDir); err != nil {
		t.Fatalf("copyEmbeddedToTarget() error = %v", err)
	}

	for _, relPath := range []string{
		filepath.Join("skills", "tmux", "scripts", "find-sessions.sh"),
		filepath.Join("skills", "tmux", "scripts", "wait-for-text.sh"),
	} {
		info, err := os.Stat(filepath.Join(targetDir, relPath))
		if err != nil {
			t.Fatalf("expected %s to exist: %v", relPath, err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("expected %s to be executable, got mode %o", relPath, info.Mode().Perm())
		}
	}
}

func TestCopyEmbeddedToTargetKeepsTemplatesUserWritable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not reliable on Windows")
	}

	targetDir := t.TempDir()
	if err := copyEmbeddedToTarget(targetDir); err != nil {
		t.Fatalf("copyEmbeddedToTarget() error = %v", err)
	}

	for _, relPath := range []string{
		"USER.md",
		filepath.Join("skills", "tmux", "scripts", "find-sessions.sh"),
	} {
		info, err := os.Stat(filepath.Join(targetDir, relPath))
		if err != nil {
			t.Fatalf("expected %s to exist: %v", relPath, err)
		}
		if info.Mode().Perm()&0o200 == 0 {
			t.Fatalf("expected %s to remain user-writable, got mode %o", relPath, info.Mode().Perm())
		}
	}
}
