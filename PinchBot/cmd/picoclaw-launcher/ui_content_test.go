package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readLauncherUI(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("internal", "ui", "index.html"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	return string(content)
}

func extractLauncherFunction(t *testing.T, ui, signature string) string {
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

func TestLauncherUIRendersReadableAgreementDetails(t *testing.T) {
	ui := readLauncherUI(t)

	if !strings.Contains(ui, "d.content") {
		t.Fatal("expected launcher UI to render agreement content for users before recharge")
	}
	if !strings.Contains(ui, "d.url") {
		t.Fatal("expected launcher UI to render agreement URL for users before recharge")
	}
}

func TestCreateRechargeOrderDoesNotAutoAcceptAgreements(t *testing.T) {
	ui := readLauncherUI(t)

	createStart := strings.Index(ui, "async function createRechargeOrder()")
	if createStart < 0 {
		t.Fatal("createRechargeOrder() not found")
	}
	createEnd := strings.Index(ui[createStart:], "async function syncOfficialModelsToConfig()")
	if createEnd < 0 {
		t.Fatal("syncOfficialModelsToConfig() not found after createRechargeOrder()")
	}
	createBody := ui[createStart : createStart+createEnd]

	if strings.Contains(createBody, "/api/app/agreements/accept") {
		t.Fatal("createRechargeOrder() should not auto-accept agreements during order creation")
	}
	if !strings.Contains(ui, "async function acceptAppAgreements()") {
		t.Fatal("expected a dedicated acceptAppAgreements() flow before order creation")
	}
}

func TestLauncherUIEscapesTransactionDescriptions(t *testing.T) {
	ui := readLauncherUI(t)

	if !strings.Contains(ui, `esc(item.description || '')`) {
		t.Fatal("expected wallet transaction descriptions to be escaped before rendering")
	}
	if !strings.Contains(ui, `function safeExternalURL(raw)`) || !strings.Contains(ui, `function openExternalURL(raw)`) {
		t.Fatal("expected launcher UI to centralize external URL allowlisting")
	}
	if !strings.Contains(ui, `safeExternalLinkHTML(d.url, d.url)`) {
		t.Fatal("expected launcher agreement links to use URL allowlisting and rel protections")
	}
	if !strings.Contains(ui, `showStatus('Refused to open an invalid external URL', 'error');`) {
		t.Fatal("expected launcher UI to reject invalid external URLs instead of opening them")
	}
}

func TestLauncherUIHasAccessibleDialogAndStatusRegions(t *testing.T) {
	ui := readLauncherUI(t)

	if !strings.Contains(ui, `id="modelModal" role="presentation"`) {
		t.Fatal("expected model modal overlay to declare presentational semantics")
	}
	if !strings.Contains(ui, `class="modal" role="dialog" aria-modal="true" aria-labelledby="modalTitle"`) {
		t.Fatal("expected model modal dialog semantics for accessibility")
	}
	if !strings.Contains(ui, `class="collapsible-header`) || !strings.Contains(ui, `aria-expanded="`) {
		t.Fatal("expected advanced model options toggle to expose aria-expanded state")
	}
	if !strings.Contains(ui, `id="toastContainer" role="status" aria-live="polite"`) {
		t.Fatal("expected toast container to announce status updates")
	}
	if !strings.Contains(ui, `id="jsonStatus" role="status" aria-live="polite"`) {
		t.Fatal("expected JSON status footer to expose a live region")
	}
}

func TestLauncherUIUsesButtonsForKeyboardReachability(t *testing.T) {
	ui := readLauncherUI(t)

	if !strings.Contains(ui, `<button class="sidebar-item active" type="button"`) {
		t.Fatal("expected sidebar navigation items to use button semantics")
	}
	if !strings.Contains(ui, `<button class="sidebar-group-title" type="button"`) {
		t.Fatal("expected sidebar group titles to use button semantics")
	}
	if !strings.Contains(ui, `function togglePressed(el)`) {
		t.Fatal("expected toggle controls to keep aria-pressed in sync")
	}
	if !strings.Contains(ui, `function toggleAdvancedOptions(el)`) {
		t.Fatal("expected advanced options toggle helper to keep aria-expanded in sync")
	}
	if !strings.Contains(ui, `<button type="button" class="array-add"`) {
		t.Fatal("expected array add controls to use keyboard-reachable buttons")
	}
}

func TestLauncherUILabelsAccountAndRechargeFields(t *testing.T) {
	ui := readLauncherUI(t)

	if !strings.Contains(ui, `<label class="form-label" for="appEmail">Email</label>`) {
		t.Fatal("expected app account email field to expose a visible label")
	}
	if !strings.Contains(ui, `<label class="form-label" for="appPassword">Password</label>`) {
		t.Fatal("expected app account password field to expose a visible label")
	}
	if !strings.Contains(ui, `<label class="form-label" for="rechargeAmountFen">Recharge Amount (fen)</label>`) {
		t.Fatal("expected recharge amount field to expose a visible label")
	}
}

func TestLauncherUIOpensOnlyValidatedExternalURLs(t *testing.T) {
	ui := readLauncherUI(t)

	createOrderBody := extractLauncherFunction(t, ui, `async function createRechargeOrder()`)
	if !strings.Contains(createOrderBody, `openExternalURL(order.pay_url)`) {
		t.Fatal("expected recharge order flow to validate pay_url before opening it")
	}
	loginBody := extractLauncherFunction(t, ui, `async function loginProvider(provider)`)
	if !strings.Contains(loginBody, `openExternalURL(data.auth_url)`) {
		t.Fatal("expected OAuth login flow to validate auth_url before opening it")
	}
	if !strings.Contains(loginBody, `safeExternalLinkHTML(data.device_url, data.device_url + ' ↗')`) {
		t.Fatal("expected device-code links to be rendered through the safe link helper")
	}
}

func TestLauncherUIUsesStopOnlyGatewayControls(t *testing.T) {
	ui := readLauncherUI(t)

	if strings.Contains(ui, `id="btnRunStop"`) {
		t.Fatal("expected launcher UI to stop exposing the start/stop toggle button")
	}
	if !strings.Contains(ui, `id="btnStopGateway"`) {
		t.Fatal("expected launcher UI to keep a dedicated stop button for the managed gateway")
	}
	if strings.Contains(ui, `/api/process/start`) {
		t.Fatal("expected launcher UI to stop calling the start endpoint directly")
	}
	if !strings.Contains(ui, `/api/process/stop`) {
		t.Fatal("expected launcher UI to keep stop endpoint wiring")
	}
}

func TestLauncherUIDocumentsPinchBotDataDirectory(t *testing.T) {
	ui := readLauncherUI(t)

	if !strings.Contains(ui, `.pinchbot/auth.json`) {
		t.Fatal("expected auth help text to point at the PinchBot data directory")
	}
	if !strings.Contains(ui, `placeholder="workspace"`) {
		t.Fatal("expected workspace settings to recommend the relative workspace path")
	}
	if strings.Contains(ui, `~/.picoclaw/workspace`) {
		t.Fatal("expected launcher UI to stop referencing the legacy ~/.picoclaw workspace path")
	}
	if !strings.Contains(ui, `PinchBot Config`) {
		t.Fatal("expected launcher UI branding to use PinchBot")
	}
}
