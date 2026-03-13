package api

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func readRepoDoc(t *testing.T, parts ...string) string {
	t.Helper()
	pathParts := append([]string{"..", "..", ".."}, parts...)
	content, err := os.ReadFile(filepath.Join(pathParts...))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", filepath.Join(parts...), err)
	}
	return string(content)
}

func TestMacReleaseScriptExplainsAppBundleSigningFlow(t *testing.T) {
	script := readRepoDoc(t, "scripts", "build-release.sh")

	if !strings.Contains(script, `launcher-chat.app`) || !strings.Contains(script, `Contents/MacOS/platform-server`) {
		t.Fatal("expected mac release script to package the desktop app as a .app bundle with bundled binaries")
	}
	if !strings.Contains(script, `PinchBot-$VERSION-$PLATFORM`) {
		t.Fatal("expected mac release script to brand the release package as PinchBot")
	}
	if !strings.Contains(script, `-X main.Version=$VERSION`) {
		t.Fatal("expected mac release script to inject the package version into launcher-chat")
	}
	if !strings.Contains(script, `launcher-chat itself`) || !strings.Contains(script, `live config/platform.env`) {
		t.Fatal("expected mac release script README to explain that the chat app depends on live platform config")
	}
	if !strings.Contains(script, `.pinchbot`) {
		t.Fatal("expected mac release script README to describe the executable-local .pinchbot data directory")
	}
	if strings.Contains(script, `workspace-example`) {
		t.Fatal("expected mac release script to stop shipping a workspace-example directory")
	}
	if !strings.Contains(script, `sanitize_bundle_version()`) || !strings.Contains(script, `<string>$BUNDLE_SHORT_VERSION</string>`) {
		t.Fatal("expected mac release script to sanitize bundle version strings for Info.plist")
	}
	if !strings.Contains(script, `MAC_CODESIGN_IDENTITY`) || !strings.Contains(script, `Apple notarization`) {
		t.Fatal("expected mac release script to describe signing and notarization requirements")
	}
	signCall := strings.LastIndex(script, "maybe_codesign")
	readmeCall := strings.LastIndex(script, "write_readme")
	if signCall == -1 || readmeCall == -1 || readmeCall < signCall {
		t.Fatal("expected README generation to happen after optional codesign so the status text stays accurate")
	}
	if strings.Contains(script, `Ship the folder '`) {
		t.Fatal("expected mac release script to avoid claiming unsigned bundles are ready for customer distribution")
	}
}

func TestMacReleaseScriptValidatesResolvedGoCandidates(t *testing.T) {
	script := readRepoDoc(t, "scripts", "build-release.sh")

	if !strings.Contains(script, `go_candidate_works "$candidate"`) &&
		!strings.Contains(script, `"$candidate" version >/dev/null 2>&1`) {
		t.Fatal("expected mac release script to validate a resolved Go candidate before returning it")
	}
}

func TestMacReleaseScriptBundlesAllRequiredAppBinaries(t *testing.T) {
	script := readRepoDoc(t, "scripts", "build-release.sh")

	if !strings.Contains(script, `"$APP_MACOS_DIR/pinchbot"`) {
		t.Fatal("expected mac release script to bundle the pinchbot gateway inside launcher-chat.app")
	}
	if !strings.Contains(script, `"$APP_MACOS_DIR/pinchbot-launcher"`) {
		t.Fatal("expected mac release script to bundle the pinchbot-launcher settings service inside launcher-chat.app")
	}
	if !strings.Contains(script, `"$APP_MACOS_DIR/platform-server"`) {
		t.Fatal("expected mac release script to bundle platform-server inside launcher-chat.app")
	}
}

func TestBuildReleaseDocsDescribeCurrentBundleLayout(t *testing.T) {
	doc := readRepoDoc(t, "docs", "build-and-release.md")

	if !strings.Contains(doc, `launcher-chat.app/`) || !strings.Contains(doc, `launcher-chat.app/Contents/MacOS/platform-server`) {
		t.Fatal("expected release docs to describe the bundled macOS app layout and platform-server path")
	}
	if !strings.Contains(doc, `PinchBot-1.0.0-Windows-x86_64`) || !strings.Contains(doc, `PinchBot-1.0.0-Darwin-arm64`) {
		t.Fatal("expected release docs to show PinchBot package names")
	}
	if !strings.Contains(doc, `README.txt`) {
		t.Fatal("expected release docs to reference README.txt as the packaged usage guide")
	}
	if strings.Contains(doc, `使用说明.txt`) {
		t.Fatal("expected release docs to stop referencing the removed usage file name")
	}
	if strings.Contains(doc, `workspace-example`) {
		t.Fatal("expected release docs to stop documenting the removed workspace-example bundle directory")
	}
	if !strings.Contains(doc, `.pinchbot`) {
		t.Fatal("expected release docs to describe the executable-local .pinchbot data directory")
	}
	if !strings.Contains(doc, `首次运行自动创建`) {
		t.Fatal("expected release docs to explain first-run .pinchbot bootstrap")
	}
	if !strings.Contains(doc, `点击“设置”时按需启动`) {
		t.Fatal("expected release docs to explain that the settings service starts on demand")
	}
	if !strings.Contains(doc, `代码签名`) {
		t.Fatal("expected release docs to warn that Windows binaries should also be code-signed before external distribution")
	}
	if !strings.Contains(doc, `MAC_CODESIGN_IDENTITY`) || !strings.Contains(doc, `notarization`) {
		t.Fatal("expected release docs to explain signing and notarization for macOS distribution")
	}
	if !strings.Contains(doc, `scripts/notarize-macos.sh`) || !strings.Contains(doc, `scripts/package-macos-dmg.sh`) {
		t.Fatal("expected release docs to mention notarization and DMG automation scripts")
	}
}

func TestWindowsReleaseScriptDocumentsRunnableCommandsAndSigning(t *testing.T) {
	script := readRepoDoc(t, "scripts", "build-release.ps1")

	if !strings.Contains(script, `PinchBot-$Version-$Platform`) {
		t.Fatal("expected windows release script to brand the package as PinchBot")
	}
	if !strings.Contains(script, `Remove-Item -Recurse -Force $OutDir`) {
		t.Fatal("expected windows release script to clear old output directories before rebuilding")
	}
	if !strings.Contains(script, `settings starts PinchBot-launcher on demand`) {
		t.Fatal("expected windows release script README to document on-demand settings launcher startup")
	}
	if !strings.Contains(script, `launcher-chat itself`) {
		t.Fatal("expected windows release script README to explain that chat usage depends on live platform config")
	}
	if !strings.Contains(script, `.pinchbot`) {
		t.Fatal("expected windows release script README to describe the executable-local .pinchbot data directory")
	}
	if strings.Contains(script, `workspace-example`) {
		t.Fatal("expected windows release script to stop copying a workspace-example directory into dist")
	}
	if !strings.Contains(script, `代码签名`) && !strings.Contains(script, `code-signed`) {
		t.Fatal("expected windows release script output to warn about signing before external distribution")
	}
}

func TestLocalPlatformStartupScriptsPinPinchBotStateToRepoDirectory(t *testing.T) {
	psScript := readRepoDoc(t, "scripts", "start-local-platform.ps1")
	shScript := readRepoDoc(t, "scripts", "start-local-platform.sh")

	if !strings.Contains(psScript, "PINCHBOT_HOME") || !strings.Contains(psScript, "PINCHBOT_CONFIG") {
		t.Fatal("expected PowerShell local startup script to pin PINCHBOT_HOME and PINCHBOT_CONFIG for go run processes")
	}
	if !strings.Contains(shScript, "PINCHBOT_HOME") || !strings.Contains(shScript, "PINCHBOT_CONFIG") {
		t.Fatal("expected shell local startup script to pin PINCHBOT_HOME and PINCHBOT_CONFIG for go run processes")
	}
	if strings.Contains(psScript, "platform.example.env") && !strings.Contains(psScript, "Specify -PlatformEnv explicitly") {
		t.Fatal("expected PowerShell startup script to stop silently falling back to platform.example.env")
	}
	if strings.Contains(shScript, "platform.example.env") && !strings.Contains(shScript, "pass an explicit env file") {
		t.Fatal("expected shell startup script to stop silently falling back to platform.example.env")
	}
}

func TestBootstrapLocalPlatformConfigScriptsCopyExampleFilesIntoLiveFiles(t *testing.T) {
	psScript := readRepoDoc(t, "scripts", "bootstrap-local-platform-config.ps1")
	shScript := readRepoDoc(t, "scripts", "bootstrap-local-platform-config.sh")

	for _, script := range []string{psScript, shScript} {
		if !strings.Contains(script, "platform.example.env") || !strings.Contains(script, "runtime-config.example.json") {
			t.Fatal("expected bootstrap scripts to read the example platform/runtime config templates")
		}
		if !strings.Contains(script, "platform.env") || !strings.Contains(script, "runtime-config.json") {
			t.Fatal("expected bootstrap scripts to materialize live config filenames")
		}
		if !strings.Contains(script, "replace-with-your-upstream-api-key") {
			t.Fatal("expected bootstrap scripts to remind operators to replace placeholder upstream credentials")
		}
	}
	if !strings.Contains(psScript, "-Force") || !strings.Contains(shScript, "--force") {
		t.Fatal("expected bootstrap scripts to expose explicit overwrite switches")
	}
}

func TestBuildDocsReferenceBootstrapAndOfficialModelSmokeFlow(t *testing.T) {
	buildDoc := readRepoDoc(t, "docs", "build-and-release.md")
	smokeDoc := readRepoDoc(t, "docs", "official-model-local-smoke-test.md")
	runbook := readRepoDoc(t, "docs", "release-macos-runbook.md")

	if !strings.Contains(buildDoc, "bootstrap-local-platform-config.sh") || !strings.Contains(buildDoc, "bootstrap-local-platform-config.ps1") {
		t.Fatal("expected build docs to mention the bootstrap scripts for live platform config")
	}
	if !strings.Contains(buildDoc, "official-model-local-smoke-test.md") {
		t.Fatal("expected build docs to link the official model smoke-test runbook")
	}
	if !strings.Contains(smokeDoc, "GET /wallet/orders/{id}") || !strings.Contains(smokeDoc, "POST /admin/orders/{id}/reconcile") {
		t.Fatal("expected official-model smoke doc to cover wallet order details and reconciliation APIs")
	}
	if !strings.Contains(smokeDoc, "replace-with-your-upstream-api-key") {
		t.Fatal("expected official-model smoke doc to call out upstream API key placeholder replacement")
	}
	if !strings.Contains(runbook, "bootstrap-local-platform-config.sh") || !strings.Contains(runbook, "official-model-local-smoke-test.md") {
		t.Fatal("expected macOS runbook to reference bootstrap config setup and official-model smoke testing")
	}
	if !strings.Contains(runbook, "scripts/notarize-macos.sh") || !strings.Contains(runbook, "scripts/package-macos-dmg.sh") {
		t.Fatal("expected macOS runbook to reference notarization and DMG automation scripts")
	}
}

func TestMacAutomationScriptsCoverNotarizationAndDmgFlow(t *testing.T) {
	notarizeScript := readRepoDoc(t, "scripts", "notarize-macos.sh")
	dmgScript := readRepoDoc(t, "scripts", "package-macos-dmg.sh")

	if !strings.Contains(notarizeScript, "xcrun notarytool submit") || !strings.Contains(notarizeScript, "xcrun stapler staple") {
		t.Fatal("expected notarize script to submit and staple macOS app bundles")
	}
	if !strings.Contains(notarizeScript, "codesign --verify") || !strings.Contains(notarizeScript, "spctl --assess") {
		t.Fatal("expected notarize script to verify signatures and Gatekeeper status")
	}
	if !strings.Contains(notarizeScript, "MAC_NOTARYTOOL_PROFILE") {
		t.Fatal("expected notarize script to support notarytool profile via environment variable")
	}
	if !strings.Contains(dmgScript, "hdiutil create") || !strings.Contains(dmgScript, "hdiutil verify") {
		t.Fatal("expected DMG script to create and verify macOS disk images")
	}
	if !strings.Contains(dmgScript, "--overwrite") {
		t.Fatal("expected DMG script to require an explicit overwrite flag")
	}
}

func TestReleaseScriptsDoNotRunGoGenerateDuringPackaging(t *testing.T) {
	windowsScript := readRepoDoc(t, "scripts", "build-release.ps1")
	macScript := readRepoDoc(t, "scripts", "build-release.sh")

	if strings.Contains(windowsScript, `generate ./...`) {
		t.Fatal("expected windows release packaging script to avoid go generate because it mutates tracked onboard workspace templates")
	}
	if strings.Contains(macScript, `generate ./...`) {
		t.Fatal("expected mac release packaging script to avoid go generate because it mutates tracked onboard workspace templates")
	}
}

func TestMacReleaseScriptHasValidShellSyntax(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	scriptPath := filepath.Join("..", "..", "..", "scripts", "build-release.sh")
	cmd := exec.Command(bashPath, "-n", scriptPath)
	cmd.Dir = "."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash -n %s failed: %v\n%s", scriptPath, err, output)
	}
}

func TestMacAutomationScriptsHaveValidShellSyntax(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}

	for _, scriptPath := range []string{
		filepath.Join("..", "..", "..", "scripts", "notarize-macos.sh"),
		filepath.Join("..", "..", "..", "scripts", "package-macos-dmg.sh"),
	} {
		cmd := exec.Command(bashPath, "-n", scriptPath)
		cmd.Dir = "."
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bash -n %s failed: %v\n%s", scriptPath, err, output)
		}
	}
}
