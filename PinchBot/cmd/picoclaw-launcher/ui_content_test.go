package main

import (
	"os"
	"os/exec"
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

func runLauncherNodeScript(t *testing.T, script string) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not available in PATH")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "launcher-ui-smoke.js")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	cmd := exec.Command("node", path)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node smoke test failed: %v\n%s", err, output)
	}
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
    if !strings.Contains(ui, `showStatus('已拦截无效的外部链接', 'error');`) {
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

    if !strings.Contains(ui, `<label class="form-label" for="appEmail">邮箱</label>`) {
        t.Fatal("expected app account email field to expose a visible label")
    }
    if !strings.Contains(ui, `<label class="form-label" for="appPassword">密码</label>`) {
        t.Fatal("expected app account password field to expose a visible label")
    }
    if !strings.Contains(ui, `<label class="form-label" for="rechargeAmountFen">充值金额（分）</label>`) {
        t.Fatal("expected recharge amount field to expose a visible label")
    }
}

func TestLauncherUIBlocksSignupWhenAgreementLoadFails(t *testing.T) {
	ui := readLauncherUI(t)

	submitBody := extractLauncherFunction(t, ui, `async function submitAppAuth(mode)`)
	if !strings.Contains(submitBody, `currentAppSignupAgreementState.loading`) {
		t.Fatal("expected launcher signup flow to block while signup agreements are still loading")
	}
	if !strings.Contains(submitBody, `currentAppSignupAgreementState.error || !currentAppSignupAgreementState.loaded`) {
		t.Fatal("expected launcher signup flow to block when signup agreements fail to load")
	}
	if !strings.Contains(ui, `let currentAppSignupAgreementState = { loading: false, loaded: false, error: '' };`) {
		t.Fatal("expected launcher UI to persist signup agreement loading state")
	}
	loadBody := extractLauncherFunction(t, ui, `async function loadAppAuthAgreements()`)
    if !strings.Contains(loadBody, `safeExternalLinkHTML(d.url, '查看完整内容')`) {
        t.Fatal("expected signup agreement links to use safe external URL rendering")
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

func TestLauncherUIKeepsOfficialModelListWhenAccessSummaryFails(t *testing.T) {
	ui := readLauncherUI(t)

	loadBody := extractLauncherFunction(t, ui, `async function loadOfficialModels()`)
	if strings.Contains(loadBody, `Promise.all([`) {
		t.Fatal("expected launcher official model loading to degrade access-state failures independently instead of failing everything together")
	}
	if !strings.Contains(loadBody, `fetch('/api/app/models')`) || !strings.Contains(loadBody, `fetch('/api/app/official-access')`) {
		t.Fatal("expected launcher official model loading to fetch both catalog and access summary")
	}
}

func TestLauncherAgreementSignatureTracksPublishedContentDrift(t *testing.T) {
	ui := readLauncherUI(t)

	signatureBody := extractLauncherFunction(t, ui, `function getCurrentAgreementSignature(docs)`)
	for _, marker := range []string{
		`d.title || ''`,
		`d.content || ''`,
		`d.url || ''`,
		`d.effective_from_unix || 0`,
	} {
		if !strings.Contains(signatureBody, marker) {
			t.Fatalf("expected agreement signature to include %q so acknowledgements reset when published content changes", marker)
		}
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
    if !strings.Contains(ui, `PinchBot 配置中心`) {
        t.Fatal("expected launcher UI branding to use PinchBot")
    }
}

func TestLauncherUIShowsPersistentAgreementRecoveryWarnings(t *testing.T) {
	ui := readLauncherUI(t)

	renderBody := extractLauncherFunction(t, ui, `function renderAppSession(data)`)
	if !strings.Contains(renderBody, `data.session.agreement_sync_pending`) {
		t.Fatal("expected launcher account panel to detect pending agreement recovery state")
	}
	if !strings.Contains(renderBody, `协议同步待完成`) {
		t.Fatal("expected launcher account panel to show a persistent agreement recovery warning")
	}
}

func TestLauncherChineseLocalizationDoesNotMutateBusinessData(t *testing.T) {
	ui := readLauncherUI(t)
	start := strings.Index(ui, `const launcherZhReplacements = [`)
	end := strings.Index(ui, `function localizeLauncherNode(root)`)
	if start < 0 || end < 0 || end <= start {
		t.Fatal("expected launcher localization definitions before localizeLauncherNode()")
	}

	script := strings.Join([]string{
		ui[start:end],
		`if (localizeLauncherString('Account & Wallet') !== '账户与钱包') { throw new Error('expected exact UI string to localize'); }`,
		`if (localizeLauncherString('active@example.com') !== 'active@example.com') { throw new Error('email text was mutated'); }`,
		`if (localizeLauncherString('No Key Model') !== 'No Key Model') { throw new Error('model name text was mutated'); }`,
		`if (localizeLauncherString('manual_adjustment') !== 'manual_adjustment') { throw new Error('reference type was mutated'); }`,
		`if (localizeLauncherString('orders.read') !== 'orders.read') { throw new Error('capability token was mutated'); }`,
	}, "\n\n")

	runLauncherNodeScript(t, script)
}

func TestLauncherLocalizationAvoidsSubstringReplacement(t *testing.T) {
	ui := readLauncherUI(t)
	body := extractLauncherFunction(t, ui, `function localizeLauncherString(value)`)
	if strings.Contains(body, `text.includes(source)`) || strings.Contains(body, `text.split(source).join(target)`) {
		t.Fatal("expected launcher localization to avoid broad substring replacement that can mutate business data")
	}
}
