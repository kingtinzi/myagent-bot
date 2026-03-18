package launcherui

import (
	"net"

	"github.com/sipeed/pinchbot/pkg/config"
)

func DefaultConfigPath() string {
	return config.GetConfigPath()
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

