package configstore

import (
	"errors"
	"os"
	"path/filepath"

	pinchbotconfig "github.com/sipeed/pinchbot/pkg/config"
)

const (
	configDirName  = ".PinchBot"
	configFileName = "config.json"
)

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName), nil
}

func Load() (*pinchbotconfig.Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	return pinchbotconfig.LoadConfig(path)
}

func Save(cfg *pinchbotconfig.Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return pinchbotconfig.SaveConfig(path, cfg)
}
