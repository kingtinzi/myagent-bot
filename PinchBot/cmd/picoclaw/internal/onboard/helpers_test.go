package onboard

import (
	"os"
	"path/filepath"
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
