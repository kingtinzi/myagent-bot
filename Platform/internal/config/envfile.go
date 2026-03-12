package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const platformEnvPath = "config/platform.env"

func LoadPlatformEnv() error {
	return LoadEnvFiles(platformEnvPath)
}

func LoadEnvFiles(paths ...string) error {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if err := loadEnvFile(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: invalid env line", path, lineNo)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return fmt.Errorf("%s:%d: env key is required", path, lineNo)
		}
		if current, exists := os.LookupEnv(key); exists && strings.TrimSpace(current) != "" {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}
