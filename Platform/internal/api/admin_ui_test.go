package api

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func readAdminUI(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("admin_index.html"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	return string(content)
}

func extractJSFunction(t *testing.T, ui, signature string) string {
	t.Helper()
	start := strings.Index(ui, signature)
	if start < 0 {
		t.Fatalf("signature %q not found", signature)
	}
	searchStart := start + len(signature)
	bodyStart := strings.Index(ui[searchStart:], "{")
	if bodyStart < 0 {
		t.Fatalf("opening brace for %q not found", signature)
	}
	depth := 0
	for i := searchStart + bodyStart; i < len(ui); i++ {
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

func runNodeScript(t *testing.T, script string) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not available in PATH")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "admin-ui-smoke.js")
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

func TestAdminUIProvidesAppShellAndModuleNavigation(t *testing.T) {
	ui := readAdminUI(t)

	for _, marker := range []string{
		`id="adminConsole" data-shell="admin-console"`,
		`id="sidebarNav"`,
		`data-nav-target="dashboard"`,
		`data-nav-target="users"`,
		`data-nav-target="operators"`,
		`data-nav-target="orders"`,
		`data-nav-target="wallet"`,
		`data-nav-target="catalog"`,
		`data-nav-target="audits"`,
		`data-nav-target="refunds"`,
		`data-nav-target="infringement"`,
		`data-nav-target="governance"`,
		`data-module="dashboard"`,
		`data-module="users"`,
		`data-module="operators"`,
		`data-module="orders"`,
		`data-module="wallet"`,
		`data-module="catalog"`,
		`data-module="audits"`,
		`data-module="refunds"`,
		`data-module="infringement"`,
		`data-module="governance"`,
		`id="userDetailContent"`,
	} {
		if !strings.Contains(ui, marker) {
			t.Fatalf("expected admin UI marker %q", marker)
		}
	}

	if !strings.Contains(ui, `function renderShell()`) {
		t.Fatal("expected renderShell() to build the admin application shell")
	}
	if !strings.Contains(ui, `function showModule(moduleId)`) {
		t.Fatal("expected showModule() for sidebar-driven module navigation")
	}
	if got := strings.Count(ui, `id="sidebarNav"`); got != 1 {
		t.Fatalf("sidebarNav id count = %d, want 1 to avoid mobile navigation binding regressions", got)
	}
}

func TestAdminUIUsesPinchBotBrandingForVisibleChrome(t *testing.T) {
	ui := readAdminUI(t)

	for _, marker := range []string{
		`<title>PinchBot 管理后台</title>`,
		`<h1>PinchBot 管理后台</h1>`,
		`<strong>PinchBot 管理后台</strong>`,
		`['PinchBot Admin Console', 'PinchBot 管理后台']`,
	} {
		if !strings.Contains(ui, marker) {
			t.Fatalf("expected admin branding marker %q", marker)
		}
	}

	if strings.Contains(ui, `OpenClaw 管理后台`) {
		t.Fatal("expected visible admin branding to stop referencing OpenClaw")
	}
}

func TestAdminUIIncludesResponsiveAccessibleConsolePrimitives(t *testing.T) {
	ui := readAdminUI(t)

	for _, marker := range []string{
		`id="appStatus" role="status" aria-live="polite" class="status-line"`,
		`id="appAlert" role="alert" aria-live="assertive" class="status-line"`,
		`id="confirmDialog" role="dialog" aria-modal="true"`,
		`class="table-scroll"`,
		`id="mobileNavToggle"`,
		`nav-toggle`,
		`@media (max-width: 1080px)`,
		`@media (max-width: 840px)`,
	} {
		if !strings.Contains(ui, marker) {
			t.Fatalf("expected responsive/accessibility marker %q", marker)
		}
	}

	if !strings.Contains(ui, `async function confirmDangerousAction(config)`) {
		t.Fatal("expected confirmDangerousAction() for high-risk operator actions")
	}
	if !strings.Contains(ui, `function setStatus(message, level)`) {
		t.Fatal("expected centralized live-status helper for admin feedback")
	}
	if !strings.Contains(ui, `function handleConfirmDialogKeydown(event)`) {
		t.Fatal("expected keyboard focus trapping for the confirmation dialog")
	}
}

func TestAdminUIImplementsPermissionAwareDataLoadingAndMutations(t *testing.T) {
	ui := readAdminUI(t)

	for _, marker := range []string{
		`function hasCapability(capability)`,
		`function apiFetch(path, options = {})`,
		`function loadAdminBootstrap()`,
		`async function resumeAdminSession()`,
		`async function loadDashboardModule()`,
		`async function loadUsersModule()`,
		`async function loadOperatorsModule()`,
		`async function loadOrdersModule()`,
		`async function loadWalletModule()`,
		`async function loadCatalogModule()`,
		`async function loadAuditsModule()`,
		`async function loadRefundsModule()`,
		`async function loadInfringementModule()`,
		`async function loadGovernanceModule()`,
		`data-permission-required=`,
	} {
		if !strings.Contains(ui, marker) {
			t.Fatalf("expected permission/data-loading marker %q", marker)
		}
	}

	hasCapabilityBody := extractJSFunction(t, ui, `function hasCapability(capability)`)
	if !strings.Contains(hasCapabilityBody, `state.operator.capabilities`) {
		t.Fatal("expected hasCapability() to read operator capabilities from admin bootstrap state")
	}
	apiFetchBody := extractJSFunction(t, ui, `function apiFetch(path, options = {})`)
	if !strings.Contains(apiFetchBody, `settings.credentials = 'same-origin'`) {
		t.Fatal("expected apiFetch() to rely on same-origin credentials for the admin cookie session")
	}
	if strings.Contains(apiFetchBody, `Authorization`) || strings.Contains(ui, `sessionStorage`) || strings.Contains(ui, `TOKEN_STORAGE_KEY`) {
		t.Fatal("expected admin UI to avoid browser-managed token storage and bearer header injection")
	}
	setAlertBody := extractJSFunction(t, ui, `function setAlert(text)`)
	if !strings.Contains(setAlertBody, `target.textContent = text || ''`) {
		t.Fatal("expected setAlert() to use its text argument directly")
	}
	setModuleStatusBody := extractJSFunction(t, ui, `function setModuleStatus(moduleId, text, kind)`)
	if !strings.Contains(setModuleStatusBody, `target.textContent = text || ''`) {
		t.Fatal("expected setModuleStatus() to use its text argument directly")
	}
	setLoginStatusBody := extractJSFunction(t, ui, `function setLoginStatus(text, kind)`)
	if !strings.Contains(setLoginStatusBody, `const tone = kind || ''`) || !strings.Contains(setLoginStatusBody, `target.textContent = text || ''`) {
		t.Fatal("expected setLoginStatus() to normalize the provided status tone and text")
	}
	loginBody := extractJSFunction(t, ui, `async function handleLoginSubmit(event)`)
	if !strings.Contains(loginBody, `/admin/session/login`) {
		t.Fatal("expected admin login form to target /admin/session/login")
	}
	signOutBody := extractJSFunction(t, ui, `async function handleSignOut()`)
	if !strings.Contains(signOutBody, `/admin/session/logout`) {
		t.Fatal("expected admin sign-out to clear the cookie-backed session")
	}
}

func TestAdminUIResetsSensitiveStateWhenSessionEnds(t *testing.T) {
	ui := readAdminUI(t)

	if !strings.Contains(ui, `function resetAdminState()`) {
		t.Fatal("expected admin UI to define a dedicated state reset helper for sign-out and expired sessions")
	}
	resetBody := extractJSFunction(t, ui, `function resetAdminState()`)
	for _, marker := range []string{
		`state.dashboard = null`,
		`state.users = []`,
		`state.userDetail = null`,
		`state.selectedUserID = ''`,
		`state.walletContextUserID = ''`,
		`state.userDetailRequestID += 1`,
		`state.operators = []`,
		`state.orders = []`,
		`state.walletAdjustments = []`,
		`state.audits = []`,
		`state.refunds = []`,
		`state.selectedRefundID = ''`,
		`state.infringementReports = []`,
		`state.selectedReportID = ''`,
		`state.editors = { catalog: {}, governance: {} }`,
		`renderCatalogModule();`,
		`renderGovernanceModule();`,
	} {
		if !strings.Contains(resetBody, marker) {
			t.Fatalf("expected resetAdminState() to include %q", marker)
		}
	}

	setAuthStateBody := extractJSFunction(t, ui, `function setAuthState(payload)`)
	if !strings.Contains(setAuthStateBody, `if (!payload) resetAdminState();`) {
		t.Fatal("expected setAuthState() to clear cached admin data when the session ends")
	}
}

func TestAdminUIKeepsEditorModulesAvailableWhenOneEditorFails(t *testing.T) {
	ui := readAdminUI(t)

	if !strings.Contains(ui, `async function loadEditorNamespace(namespace, definitions, moduleId, successMessage)`) {
		t.Fatal("expected shared editor namespace loader for partial-failure handling")
	}
	catalogBody := extractJSFunction(t, ui, `async function loadCatalogModule()`)
	if strings.Contains(catalogBody, `Promise.all(`) || !strings.Contains(catalogBody, `loadEditorNamespace('catalog'`) {
		t.Fatal("expected catalog module loading to avoid all-or-nothing Promise.all behavior")
	}
	governanceBody := extractJSFunction(t, ui, `async function loadGovernanceModule()`)
	if strings.Contains(governanceBody, `Promise.all(`) || !strings.Contains(governanceBody, `loadEditorNamespace('governance'`) {
		t.Fatal("expected governance module loading to avoid all-or-nothing Promise.all behavior")
	}
	namespaceBody := extractJSFunction(t, ui, `async function loadEditorNamespace(namespace, definitions, moduleId, successMessage)`)
	if !strings.Contains(namespaceBody, `Promise.allSettled`) || !strings.Contains(namespaceBody, `当前显示最近一次成功加载的快照`) {
		t.Fatal("expected editor namespace loader to degrade gracefully when one editor reload fails")
	}
}

func TestAdminUITracksEditorRevisionForOptimisticConcurrency(t *testing.T) {
	ui := readAdminUI(t)

	ensureBody := extractJSFunction(t, ui, `function ensureEditorState(namespace, key)`)
	if !strings.Contains(ensureBody, `revision: ''`) {
		t.Fatal("expected editor state to initialize a revision field for optimistic concurrency")
	}
	loadBody := extractJSFunction(t, ui, `async function loadEditorData(namespace, key, options = {})`)
	if !strings.Contains(loadBody, `includeResponseMeta: true`) || !strings.Contains(loadBody, `editor.revision =`) {
		t.Fatal("expected editor loads to capture response revision metadata")
	}
	saveBody := extractJSFunction(t, ui, `async function saveEditorData(namespace, key)`)
	if !strings.Contains(saveBody, `If-Match`) || !strings.Contains(saveBody, `editor.revision`) {
		t.Fatal("expected editor saves to send If-Match with the loaded revision")
	}
}

func TestAdminUIPreservesUnsavedEditorDraftsDuringBackgroundReloads(t *testing.T) {
	ui := readAdminUI(t)

	ensureBody := extractJSFunction(t, ui, `function ensureEditorState(namespace, key)`)
	if !strings.Contains(ensureBody, `dirty: false`) {
		t.Fatal("expected editor state to track whether the current JSON draft is dirty")
	}
	inputBody := extractJSFunction(t, ui, `document.addEventListener('input', event => {`)
	if !strings.Contains(inputBody, `editor.dirty = true`) {
		t.Fatal("expected editor input handling to mark unsaved drafts as dirty")
	}
	loadBody := extractJSFunction(t, ui, `async function loadEditorData(namespace, key, options = {})`)
	if !strings.Contains(loadBody, `editor.dirty && !options.force`) || !strings.Contains(loadBody, `高级编辑器中存在未保存更改`) {
		t.Fatal("expected editor reloads to avoid overwriting dirty drafts unless the operator explicitly forces a reload")
	}
}

func TestAdminUIShowsReloadGuidanceAfterEditorRevisionConflict(t *testing.T) {
	ui := readAdminUI(t)

	saveBody := extractJSFunction(t, ui, `async function saveEditorData(namespace, key)`)
	if !strings.Contains(saveBody, `configuration changed, please reload and retry the save`) {
		t.Fatal("expected editor save flow to recognize stale revision conflicts")
	}
	if !strings.Contains(saveBody, `配置已被其他管理员更新，请重新加载编辑器，与最新服务端快照对比后再重试保存。`) {
		t.Fatal("expected editor save flow to explain how to recover from a 412 revision conflict")
	}
}

func TestAdminUIRequiresConfirmationForOperatorPrivilegeChanges(t *testing.T) {
	ui := readAdminUI(t)

	body := extractJSFunction(t, ui, `async function handleOperatorSubmit(event)`)
	if !strings.Contains(body, `confirmAction({`) {
		t.Fatal("expected operator role changes to require explicit confirmation")
	}
	if !strings.Contains(body, `超级管理员变更会授予所有敏感后台模块的访问权限。`) {
		t.Fatal("expected operator confirmation copy to call out high-risk super admin changes")
	}
	if !strings.Contains(body, `管理员更新已取消。`) {
		t.Fatal("expected operator confirmation flow to report cancellation explicitly")
	}
}

func TestAdminUICoversBusinessAndGovernanceActionCopy(t *testing.T) {
	ui := readAdminUI(t)

	for _, marker := range []string{
		`管理员手动充值会直接增加用户余额`,
		`钱包调账会直接修改用户余额`,
		`退款审核会写入账本与审计日志`,
		`发布协议后会影响后续充值前的用户知情确认`,
		`风控规则与保留策略变更会影响线上治理行为`,
		`侵权处理记录需要保留审计痕迹`,
		`系统公告发布后将立即影响用户端展示`,
	} {
		if !strings.Contains(ui, marker) {
			t.Fatalf("expected dangerous-action copy marker %q", marker)
		}
	}
}

func TestAdminUIIncludesManualRechargeControls(t *testing.T) {
	ui := readAdminUI(t)

	for _, marker := range []string{
		`<h2>管理员手动充值</h2>`,
		`POST /admin/manual-recharges`,
		`id="walletManualRechargeForm"`,
		`id="walletRechargeUserID"`,
		`id="walletRechargeAmount"`,
		`id="walletRechargeDescription"`,
		`id="walletRechargeSubmit"`,
		`function handleManualRechargeSubmit(event)`,
	} {
		if !strings.Contains(ui, marker) {
			t.Fatalf("expected manual recharge UI marker %q", marker)
		}
	}
}

func TestAdminUIUserDetailProvidesManualRechargeShortcut(t *testing.T) {
	ui := readAdminUI(t)

	for _, marker := range []string{
		`data-user-recharge="`,
		`function openManualRechargeForUser(userID)`,
		`function returnToUserDetail(userID)`,
		`id="walletContextBanner"`,
	} {
		if !strings.Contains(ui, marker) {
			t.Fatalf("expected user-detail manual recharge marker %q", marker)
		}
	}

	body := extractJSFunction(t, ui, `function openManualRechargeForUser(userID)`)
	for _, marker := range []string{
		`walletRechargeUserID`,
		`walletFilterUserID`,
		`state.walletContextUserID = nextUserID`,
		`switchModule('wallet')`,
		`walletRechargeAmount`,
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("expected openManualRechargeForUser() to include %q", marker)
		}
	}

	returnBody := extractJSFunction(t, ui, `function returnToUserDetail(userID)`)
	for _, marker := range []string{
		`switchModule('users')`,
		`loadUserDetail(nextUserID)`,
	} {
		if !strings.Contains(returnBody, marker) {
			t.Fatalf("expected returnToUserDetail() to include %q", marker)
		}
	}
}

func TestAdminUIWalletContextClearsLinkedFields(t *testing.T) {
	ui := readAdminUI(t)

	clearBody := extractJSFunction(t, ui, `function clearWalletContext()`)
	for _, marker := range []string{
		`state.walletContextUserID = ''`,
		`walletRechargeUserID`,
		`walletFilterUserID`,
	} {
		if !strings.Contains(clearBody, marker) {
			t.Fatalf("expected clearWalletContext() to include %q", marker)
		}
	}

	submitBody := extractJSFunction(t, ui, `async function submitWalletMutation(config)`)
	for _, marker := range []string{
		`qs(config.formID).reset()`,
		`config.preserveUserContext`,
		`state.walletContextUserID`,
	} {
		if !strings.Contains(submitBody, marker) {
			t.Fatalf("expected submitWalletMutation() to include %q", marker)
		}
	}
}

func TestAdminUIUserDetailHonorsScopedReadCapabilities(t *testing.T) {
	ui := readAdminUI(t)

	for _, marker := range []string{
		`function canInspectUserWallet()`,
		`function canInspectUserOrders()`,
		`function canInspectUserUsage()`,
		`需要 wallet.read 权限才能查看钱包概览与流水。`,
		`需要 orders.read 权限才能查看订单记录。`,
		`需要 usage.read 权限才能查看模型用量记录。`,
	} {
		if !strings.Contains(ui, marker) {
			t.Fatalf("expected scoped user-detail marker %q", marker)
		}
	}
}

func TestAdminUIIncludesSafeLinkAndAdvancedEditorSafeguards(t *testing.T) {
	ui := readAdminUI(t)

	for _, marker := range []string{
		`function safeExternalURL(value)`,
		`已拦截不安全链接`,
		`显示高级编辑器`,
		`隐藏高级编辑器`,
		`需要 agreements.read 权限才能查看协议签署记录。`,
		`<caption class="sr-only">`,
	} {
		if !strings.Contains(ui, marker) {
			t.Fatalf("expected safeguard marker %q", marker)
		}
	}
}

func TestAdminUIIncludesStructuredOfficialRouteProtocolSelector(t *testing.T) {
	ui := readAdminUI(t)

	for _, marker := range []string{
		`协议类型 / 调用方式`,
		`OpenAI Chat Completions（/chat/completions）`,
		`Responses API（/responses）`,
		`自定义 / 其他`,
		`function inferRouteProtocol(model)`,
		`function normaliseRouteEditorData(items)`,
		`function serialiseRouteHelperData(rows)`,
		`function renderRouteHelper(namespace, key)`,
		`function updateRouteHelperDraft(namespace, key, rows, options = {})`,
		`data-route-helper-add=`,
		`data-route-helper-field=`,
	} {
		if !strings.Contains(ui, marker) {
			t.Fatalf("expected structured route helper marker %q", marker)
		}
	}
}

func TestAdminUIRouteProtocolHelperFunctionsSmokeInNode(t *testing.T) {
	ui := readAdminUI(t)
	script := strings.Join([]string{
		`globalThis.console = console;`,
		extractJSFunction(t, ui, `function firstNonEmptyString(...values)`),
		extractJSFunction(t, ui, `function routeProtocolOptions()`),
		extractJSFunction(t, ui, `function routeProtocolMeta(protocol)`),
		extractJSFunction(t, ui, `function inferRouteProtocol(model)`),
		extractJSFunction(t, ui, `function pickRouteModelConfigExtras(modelConfig)`),
		extractJSFunction(t, ui, `function createEmptyRouteHelperRow()`),
		extractJSFunction(t, ui, `function normaliseRouteEditorData(items)`),
		extractJSFunction(t, ui, `function buildRouteModelValue(row)`),
		extractJSFunction(t, ui, `function serialiseRouteHelperData(rows)`),
		`const inferred = inferRouteProtocol('responses/gpt-5.2');`,
		`if (inferred.protocol !== 'responses' || inferred.modelValue !== 'gpt-5.2') { throw new Error('responses protocol inference failed'); }`,
		`const inferredOpenAI = inferRouteProtocol('openai/gpt-4.1');`,
		`if (inferredOpenAI.protocol !== 'openai' || inferredOpenAI.modelValue !== 'gpt-4.1') { throw new Error('openai protocol inference failed'); }`,
		`const inferredCustom = inferRouteProtocol('litellm/proxy-alias');`,
		`if (inferredCustom.protocol !== 'custom' || inferredCustom.modelValue !== 'litellm/proxy-alias') { throw new Error('custom protocol inference failed'); }`,
		`const rows = normaliseRouteEditorData([{ public_model_id: 'official-gpt-5-2', model_config: { model_name: '官方 GPT-5.2', model: 'responses/gpt-5.2', api_base: 'https://example.com/v1', api_key: '__KEEP_EXISTING_SECRET__', request_timeout: 30 } }]);`,
		`if (rows.length !== 1 || rows[0].protocol !== 'responses' || rows[0].modelValue !== 'gpt-5.2') { throw new Error('route normalisation failed'); }`,
		`const serialised = serialiseRouteHelperData(rows);`,
		`if (serialised[0].model_config.model !== 'responses/gpt-5.2') { throw new Error('route serialisation model failed'); }`,
		`if (serialised[0].model_config.api_key !== '__KEEP_EXISTING_SECRET__') { throw new Error('route serialisation secret preservation failed'); }`,
		`if (serialised[0].model_config.request_timeout !== 30) { throw new Error('route serialisation extras failed'); }`,
	}, "\n\n")
	runNodeScript(t, script)
}

func TestAdminUIAuditModuleIncludesRichFiltersAndCSVExport(t *testing.T) {
	ui := readAdminUI(t)

	for _, marker := range []string{
		`id="auditFilterTargetID"`,
		`id="auditFilterRiskLevel"`,
		`id="auditFilterSinceUnix"`,
		`id="auditFilterUntilUnix"`,
		`id="auditExportCSV"`,
		`async function exportAuditLogsCSV()`,
		`/admin/audit-logs`,
		`format=csv`,
	} {
		if !strings.Contains(ui, marker) {
			t.Fatalf("expected audit enhancement marker %q", marker)
		}
	}
}

func TestAdminUIDashboardSupportsWindowSelection(t *testing.T) {
	ui := readAdminUI(t)

	for _, marker := range []string{
		`id="dashboardFilterForm"`,
		`id="dashboardFilterSinceDays"`,
		`最近 7 天`,
		`最近 30 天`,
		`最近 90 天`,
		`buildQuery(readFormValues(qs('dashboardFilterForm')))`,
	} {
		if !strings.Contains(ui, marker) {
			t.Fatalf("expected dashboard window marker %q", marker)
		}
	}
}

func TestAdminUIHelperFunctionsSmokeInNode(t *testing.T) {
	ui := readAdminUI(t)
	script := strings.Join([]string{
		`const elements = {};`,
		`function makeEl() { return { textContent: '', className: '', hidden: false, value: '', disabled: false, dataset: {}, attrs: {}, focus() {}, setAttribute(key, value) { this.attrs[key] = String(value); }, getAttribute(key) { return this.attrs[key] || ''; }, querySelectorAll() { return []; } }; }`,
		`elements.appAlert = makeEl();`,
		`elements.usersModuleStatus = makeEl();`,
		`elements.loginStatus = makeEl();`,
		`elements.globalStatus = makeEl();`,
		`globalThis.document = { getElementById(id) { return elements[id] || null; }, activeElement: { focus() {} }, contains() { return true; } };`,
		`globalThis.window = { location: { href: 'https://admin.example.com/' } };`,
		extractJSFunction(t, ui, `function qs(id)`),
		extractJSFunction(t, ui, `function safeExternalURL(value)`),
		extractJSFunction(t, ui, `function setAlert(text)`),
		extractJSFunction(t, ui, `function setModuleStatus(moduleId, text, kind)`),
		extractJSFunction(t, ui, `function setLoginStatus(text, kind)`),
		`setAlert('boom');`,
		`if (elements.appAlert.textContent !== 'boom' || elements.appAlert.className !== 'status-line error') { throw new Error('setAlert runtime regression'); }`,
		`setModuleStatus('users', 'loaded', 'success');`,
		`if (elements.usersModuleStatus.textContent !== 'loaded' || elements.usersModuleStatus.className !== 'module-inline-status success') { throw new Error('setModuleStatus runtime regression'); }`,
		`setLoginStatus('Signed in', 'success');`,
		`if (elements.loginStatus.textContent !== 'Signed in' || elements.loginStatus.className !== 'status-line success') { throw new Error('setLoginStatus runtime regression'); }`,
		`if (elements.globalStatus.textContent !== 'Signed in' || elements.globalStatus.className !== 'status-banner success') { throw new Error('setLoginStatus did not mirror global status'); }`,
		`if (safeExternalURL('javascript:alert(1)') !== '') { throw new Error('unsafe scheme was not blocked'); }`,
		`if (safeExternalURL('https://example.com/evidence') !== 'https://example.com/evidence') { throw new Error('safe url was not preserved'); }`,
	}, "\n\n")
	runNodeScript(t, script)
}

func TestAdminUIChineseLocalizationDoesNotMutateBusinessData(t *testing.T) {
	ui := readAdminUI(t)
	start := strings.Index(ui, `const ADMIN_ZH_REPLACEMENTS = [`)
	end := strings.Index(ui, `function localizeAdminNode(root)`)
	if start < 0 || end < 0 || end <= start {
		t.Fatal("expected admin UI localization definitions before localizeAdminNode()")
	}

	script := strings.Join([]string{
		ui[start:end],
		`if (localizeAdminString('The collection is empty.') !== '当前没有数据。') { throw new Error('expected exact UI string to localize'); }`,
		`if (localizeAdminString('active@example.com') !== 'active@example.com') { throw new Error('email text was mutated'); }`,
		`if (localizeAdminString('manual_adjustment') !== 'manual_adjustment') { throw new Error('reference type was mutated'); }`,
		`if (localizeAdminString('admin_manual_recharge') !== 'admin_manual_recharge') { throw new Error('manual recharge type was mutated'); }`,
		`if (localizeAdminString('credit_balance') !== 'credit_balance') { throw new Error('wallet kind was mutated'); }`,
		`if (localizeAdminString('orders.read') !== 'orders.read') { throw new Error('capability token was mutated'); }`,
	}, "\n\n")

	runNodeScript(t, script)
}

func TestAdminUILocalizationAvoidsSubstringReplacement(t *testing.T) {
	ui := readAdminUI(t)
	body := extractJSFunction(t, ui, `function localizeAdminString(value)`)
	if strings.Contains(body, `text.includes(source)`) || strings.Contains(body, `text.split(source).join(target)`) {
		t.Fatal("expected admin UI localization to avoid broad substring replacement that can mutate business data")
	}
}
