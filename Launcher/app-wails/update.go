// Package main: 自动更新 — 检查清单、下载 ZIP、下次启动前应用
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Version 由构建时注入：-ldflags "-X main.Version=xxx"
var Version = "dev"

// 更新清单 API 返回的 JSON 结构（见 docs/auto-update.md）
type UpdateManifest struct {
	Version     string `json:"version"`
	URL         string `json:"url"`
	ZipFolder   string `json:"zip_folder"`
	ReleaseDate string `json:"release_date,omitempty"`
	Notes       string `json:"notes,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

// DefaultManifestURL 更新清单地址；发布时改为你的实际 URL 或通过配置覆盖
const DefaultManifestURL = "https://example.com/pinchbot/update-manifest.json"

const (
	manifestURLEnv       = "PINCHBOT_UPDATE_MANIFEST_URL"
	legacyManifestURLEnv = "OPENCLAW_UPDATE_MANIFEST_URL"
)

func getManifestURL() string {
	if u := os.Getenv(manifestURLEnv); u != "" {
		return u
	}
	if u := os.Getenv(legacyManifestURLEnv); u != "" {
		return u
	}
	return DefaultManifestURL
}

// FetchManifest 拉取更新清单
func FetchManifest(ctx context.Context) (*UpdateManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, getManifestURL(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest 返回 %d", resp.StatusCode)
	}
	var m UpdateManifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	if m.Version == "" || m.URL == "" || m.ZipFolder == "" {
		return nil, fmt.Errorf("manifest 缺少 version/url/zip_folder")
	}
	return &m, nil
}

// versionLess 比较 a < b（用于判断是否有新版本）。不支持则按字符串比较。
func versionLess(a, b string) bool {
	if a == "" || a == "dev" {
		return true
	}
	if b == "" || b == "dev" {
		return false
	}
	partsA := strings.Split(strings.TrimPrefix(a, "v"), ".")
	partsB := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(partsA) || i < len(partsB); i++ {
		var na, nb int
		if i < len(partsA) {
			fmt.Sscanf(partsA[i], "%d", &na)
		}
		if i < len(partsB) {
			fmt.Sscanf(partsB[i], "%d", &nb)
		}
		if na < nb {
			return true
		}
		if na > nb {
			return false
		}
	}
	return false
}

const (
	pendingDirEnv         = "PINCHBOT_PENDING_DIR"
	legacyPendingDirEnv   = "OPENCLAW_PENDING_DIR"
	currentPendingDirName = "PinchBot"
	legacyPendingDirName  = "OpenClaw"
	pendingExtractDirName = "PinchBot-update-extract"
)

func getPendingDir() (string, error) {
	if d := os.Getenv(pendingDirEnv); d != "" {
		return ensureDirectory(d)
	}
	if d := os.Getenv(legacyPendingDirEnv); d != "" {
		return ensureDirectory(d)
	}
	// %LOCALAPPDATA%\PinchBot\pending（若已存在旧版 OpenClaw pending，则继续复用以避免丢失待应用更新）
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData = os.Getenv("TEMP")
	}
	if localAppData == "" {
		return "", fmt.Errorf("无法确定 LOCALAPPDATA/TEMP")
	}
	currentDir := filepath.Join(localAppData, currentPendingDirName, "pending")
	legacyDir := filepath.Join(localAppData, legacyPendingDirName, "pending")
	if directoryExists(currentDir) {
		return currentDir, nil
	}
	if directoryExists(legacyDir) {
		return legacyDir, nil
	}
	return ensureDirectory(currentDir)
}

const pendingMetaFile = "pending_update.json"

type pendingMeta struct {
	ZipPath    string `json:"zip_path"`
	ZipFolder  string `json:"zip_folder"`
	InstallDir string `json:"install_dir"`
}

// DownloadUpdate 将 m.URL 下载到 pending 目录并写入待应用元数据
func DownloadUpdate(ctx context.Context, m *UpdateManifest) (zipPath string, err error) {
	dir, err := getPendingDir()
	if err != nil {
		return "", err
	}
	zipPath = filepath.Join(dir, filepath.Base(m.URL))
	if zipPath == "." {
		zipPath = filepath.Join(dir, "update.zip")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载返回 %d", resp.StatusCode)
	}

	f, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(zipPath)
		return "", err
	}

	installDir, err := getInstallDir()
	if err != nil {
		os.Remove(zipPath)
		return "", err
	}
	meta := pendingMeta{ZipPath: zipPath, ZipFolder: m.ZipFolder, InstallDir: installDir}
	metaPath := filepath.Join(dir, pendingMetaFile)
	metaBytes, _ := json.Marshal(meta)
	if err := os.WriteFile(metaPath, metaBytes, 0644); err != nil {
		os.Remove(zipPath)
		return "", err
	}
	return zipPath, nil
}

func getInstallDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

// HasPendingUpdate 是否存在已下载待应用的更新
func HasPendingUpdate() bool {
	dir, err := getPendingDir()
	if err != nil {
		return false
	}
	metaPath := filepath.Join(dir, pendingMetaFile)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return false
	}
	var meta pendingMeta
	if json.Unmarshal(data, &meta) != nil {
		return false
	}
	if meta.ZipPath == "" || meta.InstallDir == "" || meta.ZipFolder == "" {
		return false
	}
	if _, err := os.Stat(meta.ZipPath); err != nil {
		return false
	}
	return true
}

// RunApplyScriptAndExit 生成并执行「延迟更新」脚本后退出；脚本会在进程退出后解压覆盖并重启
func RunApplyScriptAndExit() {
	dir, err := getPendingDir()
	if err != nil {
		log.Printf("[update] 无法获取 pending 目录: %v", err)
		return
	}
	metaPath := filepath.Join(dir, pendingMetaFile)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		log.Printf("[update] 读取 pending 元数据失败: %v", err)
		return
	}
	var meta pendingMeta
	if json.Unmarshal(data, &meta) != nil {
		log.Printf("[update] 解析 pending 元数据失败")
		return
	}
	installDir := meta.InstallDir
	zipPath := meta.ZipPath
	zipFolder := meta.ZipFolder
	exeName := "launcher-chat.exe"
	if runtime.GOOS != "windows" {
		exeName = "launcher-chat"
	}
	launcherExe := filepath.Join(installDir, exeName)

	// 使用 PowerShell：等待进程退出 → 解压 → 复制 zip_folder 内容到安装目录 → 启动（路径用单引号避免转义）
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
Start-Sleep -Seconds 4
$zip = '%s'
$dst = '%s'
$inner = '%s'
$launcher = '%s'
$tempExtract = Join-Path $env:TEMP "%s"
if (Test-Path $tempExtract) { Remove-Item -Recurse -Force $tempExtract }
Expand-Archive -Path $zip -DestinationPath $tempExtract -Force
$innerPath = Join-Path $tempExtract $inner
if (-not (Test-Path $innerPath)) { exit 1 }
Copy-Item -Path (Join-Path $innerPath "*") -Destination $dst -Recurse -Force
Remove-Item -Path $zip -Force -ErrorAction SilentlyContinue
Remove-Item -Path (Join-Path (Split-Path $zip) "pending_update.json") -Force -ErrorAction SilentlyContinue
Remove-Item -Path $tempExtract -Recurse -Force -ErrorAction SilentlyContinue
Start-Process -FilePath $launcher -WorkingDirectory $dst
`, zipPath, installDir, zipFolder, launcherExe, pendingExtractDirName)

	scriptPath := filepath.Join(dir, "apply_update.ps1")
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		log.Printf("[update] 写入脚本失败: %v", err)
		return
	}

	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		log.Printf("[update] 启动应用脚本失败: %v", err)
		return
	}
	_ = cmd.Process.Release()
	log.Printf("[update] 已安排退出后应用更新并重启")
}

func ensureDirectory(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func directoryExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
