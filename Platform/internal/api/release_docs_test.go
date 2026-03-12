package api

import (
	"os"
	"path/filepath"
	"regexp"
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
	if !regexp.MustCompile(`maybe_codesign\r?\nwrite_readme`).MatchString(script) {
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
