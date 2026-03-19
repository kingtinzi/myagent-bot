package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadPlatformEnvFromPath loads KEY=VALUE lines from a .env-style file into the process
// environment. Missing files are ignored. Existing non-empty environment variables are
// not overridden (same semantics as Platform/internal/config/envfile).
func LoadPlatformEnvFromPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
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
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}
