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

func TestDesktopFrontendBlocksSignupWhenAgreementLoadFails(t *testing.T) {
	ui := readDesktopFrontend(t)

	if !strings.Contains(ui, `id="authUsername"`) {
		t.Fatal("expected desktop signup form to expose a username field")
	}
	submitBody := extractDesktopFunction(t, ui, `function submitAuth()`)
	if !strings.Contains(submitBody, `var username = (document.getElementById('authUsername').value || '').trim();`) {
		t.Fatal("expected desktop signup flow to read the username field")
	}
	if !strings.Contains(ui, `function looksLikeEmailAddress(value)`) {
		t.Fatal("expected desktop auth flow to expose a shared email format helper")
	}
	if !strings.Contains(submitBody, `if (authMode !== 'login' && !username)`) {
		t.Fatal("expected desktop signup flow to require a non-empty username")
	}
	if !strings.Contains(submitBody, `if (!looksLikeEmailAddress(email))`) {
		t.Fatal("expected desktop auth flow to reject malformed email addresses before bridge submission")
	}
	if !strings.Contains(submitBody, `signupAgreementState.loading`) {
		t.Fatal("expected desktop signup flow to block while signup agreements are still loading")
	}
	if !strings.Contains(submitBody, `signupAgreementState.error || !signupAgreementState.loaded`) {
		t.Fatal("expected desktop signup flow to block when signup agreements fail to load")
	}
	if strings.Contains(submitBody, `app.SignUpWithAgreements || app.SignUp`) {
		t.Fatal("expected desktop signup flow to stop falling back to the legacy no-agreement signup bridge")
	}
	if !strings.Contains(submitBody, `[email, password, username, signupAgreements]`) {
		t.Fatal("expected desktop signup flow to forward username together with agreements")
	}
}

func TestDesktopFrontendPlacesUsernameAboveEmailInAuthForm(t *testing.T) {
	ui := readDesktopFrontend(t)

	usernameIdx := strings.Index(ui, `id="authUsername"`)
	emailIdx := strings.Index(ui, `id="authEmail"`)
	if usernameIdx < 0 || emailIdx < 0 {
		t.Fatal("expected desktop auth form to contain both username and email fields")
	}
	if usernameIdx > emailIdx {
		t.Fatal("expected desktop auth form to place the username input above the email input")
	}
}

func TestDesktopFrontendKeepsOfficialPanelInSyncAfterUsageAndRefocus(t *testing.T) {
	ui := readDesktopFrontend(t)

	if !strings.Contains(ui, `window.setInterval(function() {`) || !strings.Contains(ui, `refreshOfficialPanel({ showLoading: false });`) {
		t.Fatal("expected desktop frontend to poll the official model panel in the background")
	}
	if !strings.Contains(ui, `window.addEventListener('focus'`) || !strings.Contains(ui, `document.addEventListener('visibilitychange'`) {
		t.Fatal("expected desktop frontend to refresh official model state when the window regains focus")
	}
	sendBody := extractDesktopFunction(t, ui, `function send()`)
	if !strings.Contains(sendBody, `}).finally(function() {`) || !strings.Contains(sendBody, `refreshOfficialPanel({ showLoading: false });`) {
		t.Fatal("expected desktop chat completion to trigger an official model panel refresh")
	}
	refreshBody := extractDesktopFunction(t, ui, `function refreshOfficialPanel(options)`)
	if !strings.Contains(refreshBody, `app.GetOfficialPanelSnapshot()`) {
		t.Fatal("expected official panel refresh to use a single bridge call for access and model data")
	}
	if strings.Contains(refreshBody, `Promise.all([app.GetOfficialAccessState(), app.ListOfficialModels()])`) {
		t.Fatal("expected official panel refresh to stop racing two independent bridge calls")
	}
}

func TestDesktopFrontendUsesSafeAgreementLinks(t *testing.T) {
	ui := readDesktopFrontend(t)

	if !strings.Contains(ui, `function safeExternalURL(raw)`) || !strings.Contains(ui, `function safeExternalLinkHTML(raw, label)`) {
		t.Fatal("expected desktop frontend to centralize safe external URL rendering")
	}
	renderBody := extractDesktopFunction(t, ui, `function renderAuthAgreementModalBody(doc)`)
	if !strings.Contains(renderBody, `safeExternalLinkHTML(doc.url, '查看完整协议')`) {
		t.Fatal("expected desktop signup agreement modal links to go through the safe link helper")
	}
}

func TestDesktopFrontendShowsSignupAgreementsInModalPreview(t *testing.T) {
	ui := readDesktopFrontend(t)

	if !strings.Contains(ui, `id="authAgreementModal" role="presentation"`) {
		t.Fatal("expected desktop signup flow to expose an agreement preview overlay")
	}
	if strings.Contains(ui, `id="authAgreementLinkRow"`) {
		t.Fatal("expected desktop signup flow to stop rendering a separate agreement button row")
	}
	renderBody := extractDesktopFunction(t, ui, `function renderSignupAgreements()`)
	if !strings.Contains(renderBody, `openAuthAgreementModal(`) {
		t.Fatal("expected desktop signup agreements to open a modal preview when clicked")
	}
	if !strings.Contains(renderBody, `document.getElementById('authAgreementConsentLabel')`) {
		t.Fatal("expected desktop signup agreements to rewrite the consent copy so the agreement names are clickable inline")
	}
	if strings.Contains(renderBody, `<button type="button" class="auth-agreement-inline-link"`) {
		t.Fatal("expected desktop signup agreements to stop rendering the agreement names as buttons")
	}
	if !strings.Contains(renderBody, `<span role="button" tabindex="0" onclick="openAuthAgreementModal(`) {
		t.Fatal("expected desktop signup agreements to keep inline text style while exposing keyboard-focusable clickable agreement names")
	}
	if !strings.Contains(ui, `function handleAuthAgreementTriggerKey(event, index)`) {
		t.Fatal("expected desktop signup agreements to support Enter/Space keyboard activation")
	}
	if strings.Contains(renderBody, `请点击以下协议名称查看完整内容：`) {
		t.Fatal("expected desktop signup flow to stop showing a separate agreement action row")
	}
	if strings.Contains(renderBody, `escapeHTML(doc.content)`) {
		t.Fatal("expected desktop signup agreements to stop inlining full agreement content in the auth form")
	}
}

func TestDesktopFrontendMasksSignupAgreementLoadErrors(t *testing.T) {
	ui := readDesktopFrontend(t)

	loadBody := extractDesktopFunction(t, ui, `function loadSignupAgreements()`)
	if !strings.Contains(loadBody, `error: '加载注册协议失败，请刷新后重试。'`) {
		t.Fatal("expected desktop signup agreement loading failures to be normalized to a safe user-facing message")
	}
	if strings.Contains(loadBody, `String(err)`) || strings.Contains(loadBody, `err.message`) {
		t.Fatal("expected desktop signup agreement loading failures to stop exposing raw transport errors")
	}
}

func TestDesktopFrontendSeparatesAgreementLinksFromConsentLabel(t *testing.T) {
	ui := readDesktopFrontend(t)

	if strings.Contains(ui, `<label class="auth-agreement-consent">`) {
		t.Fatal("expected desktop auth agreement consent wrapper to stop wrapping interactive links in a label")
	}
	if !strings.Contains(ui, `<label for="authAgreementConsent"`) {
		t.Fatal("expected desktop auth consent copy to be associated to the checkbox without wrapping the agreement links")
	}
	if !strings.Contains(ui, `id="authAgreementConsentLabel" style="white-space:nowrap;"`) {
		t.Fatal("expected desktop auth consent copy to keep the clickable agreement sentence on a single line")
	}
	if strings.Contains(ui, `class="auth-agreement-consent-body"`) {
		t.Fatal("expected desktop auth consent copy to stop using the old stacked agreement layout container")
	}
}

func TestDesktopFrontendTreatsAgreementPreviewAsSingleActiveDialog(t *testing.T) {
	ui := readDesktopFrontend(t)

	openBody := extractDesktopFunction(t, ui, `function openAuthAgreementModal(index, trigger)`)
	if !strings.Contains(openBody, `authDialog.setAttribute('aria-hidden', 'true')`) {
		t.Fatal("expected desktop auth dialog to be hidden from assistive technology while the agreement preview dialog is open")
	}
	closeBody := extractDesktopFunction(t, ui, `function closeAuthAgreementModal(options)`)
	if !strings.Contains(closeBody, `authDialog.removeAttribute('aria-hidden')`) {
		t.Fatal("expected desktop auth dialog semantics to be restored after closing the agreement preview")
	}
}

func TestDesktopFrontendDoesNotRestoreFocusToHiddenAgreementTrigger(t *testing.T) {
	ui := readDesktopFrontend(t)

	setModeBody := extractDesktopFunction(t, ui, `function setAuthMode(mode)`)
	if !strings.Contains(setModeBody, `closeAuthAgreementModal({ restoreFocus: false })`) {
		t.Fatal("expected auth mode switches to close the agreement preview without restoring focus to a now-hidden trigger")
	}
	closeBody := extractDesktopFunction(t, ui, `function closeAuthAgreementModal(options)`)
	if !strings.Contains(closeBody, `options && options.restoreFocus === false`) {
		t.Fatal("expected agreement preview close flow to support suppressing focus restoration during auth mode switches")
	}
}

func TestDesktopFrontendShowsPersistentAgreementRecoveryWarnings(t *testing.T) {
	ui := readDesktopFrontend(t)

	renderBody := extractDesktopFunction(t, ui, `function renderAuthState()`)
	if !strings.Contains(renderBody, `authState.agreement_sync_pending`) {
		t.Fatal("expected desktop auth state rendering to react to pending agreement recovery state")
	}
	if !strings.Contains(renderBody, `协议确认待同步`) {
		t.Fatal("expected desktop auth chrome to expose a persistent pending-agreement warning")
	}
}

func TestDesktopFrontendUpdatesSubtitleForSignupMode(t *testing.T) {
	ui := readDesktopFrontend(t)

	setModeBody := extractDesktopFunction(t, ui, `function setAuthMode(mode)`)
	if !strings.Contains(setModeBody, `document.getElementById('authSubtitle').textContent = mode === 'login' ? '登录后才能使用聊天与官方模型能力。' : '注册后即可使用聊天与官方模型能力。';`) {
		t.Fatal("expected desktop auth subtitle to switch between login and signup guidance")
	}
}

func TestDesktopFrontendPrefersUsernameInAccountStatus(t *testing.T) {
	ui := readDesktopFrontend(t)

	renderBody := extractDesktopFunction(t, ui, `function renderAuthState()`)
	if !strings.Contains(renderBody, `var accountIdentity = authState.username || authState.email || '';`) {
		t.Fatal("expected desktop auth chrome to prefer the username before falling back to email")
	}
	if !strings.Contains(renderBody, `var accountStatus = accountIdentity ? (accountIdentity + ' · 余额 ' + (authState.balance_fen || 0) / 100 + ' 元') : '已登录';`) {
		t.Fatal("expected desktop auth chrome to build the top status line from the preferred account identity")
	}
}

func TestDesktopFrontendTracksPendingRepliesByStableIndex(t *testing.T) {
	ui := readDesktopFrontend(t)

	sendBody := extractDesktopFunction(t, ui, `function send()`)
	if strings.Contains(sendBody, `messages[messages.length - 1]`) {
		t.Fatal("expected desktop send flow to stop mutating only the last message so concurrent replies cannot overwrite each other")
	}
	if !strings.Contains(sendBody, `replyIndex`) || !strings.Contains(sendBody, `messages[replyIndex]`) {
		t.Fatal("expected desktop send flow to track the pending assistant placeholder by a stable index")
	}
}

func TestDesktopFrontendPreventsOverlappingAuthSubmissions(t *testing.T) {
	ui := readDesktopFrontend(t)

	if !strings.Contains(ui, `var authSubmitPending = false;`) {
		t.Fatal("expected desktop auth flow to track in-flight submissions")
	}
	submitBody := extractDesktopFunction(t, ui, `function submitAuth()`)
	for _, marker := range []string{
		`if (authSubmitPending)`,
		`authSubmitPending = true;`,
		`document.getElementById('btnAuthSubmit').disabled = true;`,
		`}).finally(function() {`,
		`authSubmitPending = false;`,
	} {
		if !strings.Contains(submitBody, marker) {
			t.Fatalf("expected submitAuth() to include %q", marker)
		}
	}
}

func TestDesktopFrontendUsesUserFacingCopyInsteadOfTechnicalAuthText(t *testing.T) {
	ui := readDesktopFrontend(t)

	if !strings.Contains(ui, `我已阅读并同意《用户协议》《隐私政策》。`) {
		t.Fatal("expected desktop auth consent copy to name the exact agreements")
	}
	if strings.Contains(ui, `我已阅读并同意当前协议。`) {
		t.Fatal("expected desktop auth consent copy to stop using vague agreement wording")
	}
	if strings.Contains(ui, `当前未配置注册协议，可直接注册。`) {
		t.Fatal("expected desktop signup copy to stop implying registration can continue without configured agreements")
	}
	if strings.Contains(ui, `桌面桥接暂未暴露官方模型接口。`) {
		t.Fatal("expected desktop official-model fallback copy to avoid technical bridge jargon")
	}
	if strings.Contains(ui, `桌面聊天桥接未绑定，请重新编译运行`) {
		t.Fatal("expected desktop chat fallback copy to avoid technical bridge jargon")
	}
	if strings.Contains(ui, `当前客户端未提供登录接口`) || strings.Contains(ui, `当前客户端未提供带协议校验的注册接口，请升级后重试`) {
		t.Fatal("expected desktop auth failure copy to avoid technical interface wording")
	}
	if strings.Contains(ui, `alert((err && err.message) ? err.message : String(err));`) {
		t.Fatal("expected desktop sign-out failure feedback to stop using alert dialogs")
	}
}
