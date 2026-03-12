package server

import (
	"net"
	"os"
	"path/filepath"
)

// DefaultConfigPath returns the path to config.json when no CLI arg is given.
// Uses <exe_dir>/.pinchbot/config.json so data lives next to the program; fallback ~/.pinchbot/config.json.
func DefaultConfigPath() string {
	return filepath.Join(GetPinchBotHome(), "config.json")
}

// GetPinchBotHome 返回数据目录（与程序同目录的 .pinchbot），与 PinchBot 网关的 GetPinchBotHome 一致。
func GetPinchBotHome() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), ".pinchbot")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pinchbot")
}

func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return ""
}
