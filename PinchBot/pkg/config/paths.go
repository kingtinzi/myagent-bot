package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	PinchBotHomeEnv   = "PINCHBOT_HOME"
	PinchBotConfigEnv = "PINCHBOT_CONFIG"
	LegacyHomeEnv     = "PICOCLAW_HOME"
	LegacyConfigEnv   = "PICOCLAW_CONFIG"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func executableDir() string {
	if exePath, err := os.Executable(); err == nil && strings.TrimSpace(exePath) != "" {
		return filepath.Dir(exePath)
	}
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		return cwd
	}
	return "."
}

// isMacOSAppBundleExecutable 判断路径是否为 *.app/Contents/MacOS/<exe>（从访达双击启动的 GUI 主程序）。
func isMacOSAppBundleExecutable(exePath string) bool {
	exePath = filepath.Clean(strings.TrimSpace(exePath))
	if exePath == "" || exePath == "." {
		return false
	}
	dir := filepath.Dir(exePath)
	if !strings.EqualFold(filepath.Base(dir), "MacOS") {
		return false
	}
	contentsDir := filepath.Dir(dir)
	if !strings.EqualFold(filepath.Base(contentsDir), "Contents") {
		return false
	}
	appDir := filepath.Dir(contentsDir)
	return strings.HasSuffix(strings.ToLower(appDir), ".app")
}

// pinchBotHomeBaseFor 返回未设置 PINCHBOT_HOME 时，「.pinchbot」的父目录（由主程序路径推导）。
// 非 .app 包：使用可执行文件所在目录（与旧行为一致，便于 go run / 命令行单文件）。
func pinchBotHomeBaseFor(exePath string) string {
	exePath = filepath.Clean(strings.TrimSpace(exePath))
	if exePath == "" || exePath == "." {
		return "."
	}
	return filepath.Dir(exePath)
}

func defaultPinchBotHomeBase() string {
	exePath, err := os.Executable()
	if err != nil || strings.TrimSpace(exePath) == "" {
		if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
			return cwd
		}
		return "."
	}
	return pinchBotHomeBaseFor(exePath)
}

func GetPinchBotHome() string {
	if home := firstNonEmpty(os.Getenv(PinchBotHomeEnv), os.Getenv(LegacyHomeEnv)); home != "" {
		return normalizeConfiguredPath(home, executableDir())
	}
	// macOS 从访达双击 .app 时，系统可能 App Translocation：整个包挂在只读临时卷，
	// 在「.app 同级」或包内写 .pinchbot 会失败 → 进程秒退。固定写到用户可写目录。
	if runtime.GOOS == "darwin" {
		if exePath, err := os.Executable(); err == nil && isMacOSAppBundleExecutable(exePath) {
			if uh, err := os.UserHomeDir(); err == nil && strings.TrimSpace(uh) != "" {
				return filepath.Join(uh, "Library", "Application Support", "PinchBot")
			}
		}
	}
	return filepath.Join(defaultPinchBotHomeBase(), ".pinchbot")
}

func GetConfigPath() string {
	if configPath := firstNonEmpty(os.Getenv(PinchBotConfigEnv), os.Getenv(LegacyConfigEnv)); configPath != "" {
		return normalizeConfiguredConfigPath(configPath)
	}
	return filepath.Join(GetPinchBotHome(), "config.json")
}

func ResolveWorkspacePath(path string) string {
	path = expandHome(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(GetPinchBotHome(), path)
}

func LoadOrInitConfig(path string) (*Config, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		if err := SaveConfig(path, cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func normalizeConfiguredPath(path, baseDir string) string {
	path = expandHome(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(baseDir, path))
}

func normalizeConfiguredConfigPath(path string) string {
	path = expandHome(strings.TrimSpace(path))
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(GetPinchBotHome(), path))
}
