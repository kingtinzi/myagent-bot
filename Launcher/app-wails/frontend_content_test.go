package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readDesktopFrontend(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("frontend", "index.html"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	return string(content)
}

func extractDesktopFunction(t *testing.T, ui, signature string) string {
	t.Helper()
	start := strings.Index(ui, signature)
	if start < 0 {
		t.Fatalf("signature %q not found", signature)
	}
	open := strings.Index(ui[start:], "{")
	if open < 0 {
		t.Fatalf("opening brace for %q not found", signature)
	}
	depth := 0
	for i := start + open; i < len(ui); i++ {
		switch ui[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return ui[start : i+1]
			}
		}
	}
	t.Fatalf("closing brace for %q not found", signature)
	return ""
}

func TestDesktopFrontendUsesAccessibleAuthDialog(t *testing.T) {
	ui := readDesktopFrontend(t)

	if !strings.Contains(ui, `id="authDialog" role="dialog" aria-modal="true"`) {
		t.Fatal("expected auth dialog semantics for the desktop login gate")
	}
	if !strings.Contains(ui, `id="authError" role="alert" aria-live="assertive"`) {
		t.Fatal("expected auth errors to be announced to assistive technology")
	}
	if !strings.Contains(ui, `function announceStatus(text)`) {
		t.Fatal("expected a shared status announcer for login/session updates")
	}
	if !strings.Contains(ui, `document.getElementById('authPanel').addEventListener('keydown'`) {
		t.Fatal("expected auth dialog focus trapping for keyboard users")
	}
	if !strings.Contains(ui, `function setBackgroundInteractive(enabled)`) {
		t.Fatal("expected auth gate to disable background content while active")
	}
}

func TestDesktopFrontendExposesLiveChatStatusAndButtons(t *testing.T) {
	ui := readDesktopFrontend(t)

	if !strings.Contains(ui, `id="liveStatus" role="status" aria-live="polite"`) {
		t.Fatal("expected a polite live region for desktop status updates")
	}
	if !strings.Contains(ui, `class="header-link" id="btnSettings"`) {
		t.Fatal("expected settings entry point to use button semantics")
	}
	if !strings.Contains(ui, `class="secondary-link" id="toggleAuthMode"`) {
		t.Fatal("expected auth mode toggle to use button semantics")
	}
}

func TestDesktopFrontendUsesButtonSemanticsForAttachmentRemoval(t *testing.T) {
	ui := readDesktopFrontend(t)

	renderBody := extractDesktopFunction(t, ui, `function renderComposerFiles()`)
	if !strings.Contains(renderBody, `document.createElement('button')`) {
		t.Fatal("expected composer attachment removal to use a real button")
	}
	if !strings.Contains(renderBody, `remove.setAttribute('aria-label', '移除附件 ' + (f.name || ''));`) {
		t.Fatal("expected composer attachment removal button to expose an accessible name")
	}
}

func TestDesktopFrontendClearsStoredHistoryOnSignOut(t *testing.T) {
	ui := readDesktopFrontend(t)

	if !strings.Contains(ui, `function clearHistoryForKey(storageKey)`) {
		t.Fatal("expected desktop frontend to centralize per-user history clearing")
	}
	applyBody := extractDesktopFunction(t, ui, `function applyAuthState(state)`)
	if !strings.Contains(applyBody, `clearHistoryForKey(previousHistoryKey);`) {
		t.Fatal("expected auth transitions to clear stored history for the previous user")
	}
}
