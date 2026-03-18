package gatewayservice

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/pinchbot/internal/workspacetpl"
	"github.com/sipeed/pinchbot/pkg/config"
)

type Options struct {
	ConfigPath      string
	Debug           bool
	ShutdownTimeout time.Duration
	OnLog           func(string)
}

type runtimeController interface {
	Start(context.Context) error
	Stop(context.Context) error
}

var (
	workspaceBootstrapper = EnsureWorkspaceBootstrap
	runtimeFactory        = buildRuntime
)

type Service struct {
	mu      sync.Mutex
	opts    Options
	cfgPath string
	cfg     *config.Config
	runtime runtimeController
}

func New(opts Options) (*Service, error) {
	cfgPath := strings.TrimSpace(opts.ConfigPath)
	if cfgPath == "" {
		cfgPath = config.GetConfigPath()
	}
	cfg, err := config.LoadOrInitConfig(cfgPath)
	if err != nil {
		return nil, err
	}
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 15 * time.Second
	}
	return &Service{
		opts:    opts,
		cfgPath: cfgPath,
		cfg:     cfg,
	}, nil
}

func (s *Service) ConfigPath() string {
	return s.cfgPath
}

func (s *Service) BaseURL() string {
	return gatewayBaseURL(s.cfg)
}

func (s *Service) HealthURL() string {
	return s.BaseURL() + "/health"
}

func (s *Service) ReadyURL() string {
	return s.BaseURL() + "/ready"
}

func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.runtime != nil {
		return nil
	}
	if err := workspaceBootstrapper(s.cfg.WorkspacePath()); err != nil {
		return err
	}
	rt, err := runtimeFactory(s.cfg, s.opts)
	if err != nil {
		return err
	}
	if err := rt.Start(ctx); err != nil {
		return err
	}
	s.runtime = rt
	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	rt := s.runtime
	s.runtime = nil
	s.mu.Unlock()

	if rt == nil {
		return nil
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && s.opts.ShutdownTimeout > 0 {
		shutdownCtx, cancel := context.WithTimeout(ctx, s.opts.ShutdownTimeout)
		defer cancel()
		ctx = shutdownCtx
	}
	return rt.Stop(ctx)
}

func (s *Service) logf(format string, args ...any) {
	if s.opts.OnLog != nil {
		s.opts.OnLog(fmt.Sprintf(format, args...))
	}
}

func gatewayBaseURL(cfg *config.Config) string {
	host := "127.0.0.1"
	port := 18790
	if cfg != nil {
		if trimmed := strings.TrimSpace(cfg.Gateway.Host); trimmed != "" && trimmed != "0.0.0.0" {
			host = trimmed
		}
		if cfg.Gateway.Port > 0 {
			port = cfg.Gateway.Port
		}
	}
	return "http://" + net.JoinHostPort(host, fmt.Sprintf("%d", port))
}

// EnsureWorkspaceBootstrap creates the workspace directory and copies embedded
// template files into it when the directory is missing or empty.
func EnsureWorkspaceBootstrap(workspace string) error {
	info, err := os.Stat(workspace)
	if os.IsNotExist(err) {
		return copyEmbeddedToTarget(workspace)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace path is not a directory: %s", workspace)
	}

	entries, err := os.ReadDir(workspace)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return copyEmbeddedToTarget(workspace)
	}
	return nil
}

func copyEmbeddedToTarget(targetDir string) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	return fs.WalkDir(workspacetpl.Files, workspacetpl.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := workspacetpl.Files.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", path, err)
		}
		relPath, err := filepath.Rel(workspacetpl.Root, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}
		targetPath := filepath.Join(targetDir, relPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", filepath.Dir(targetPath), err)
		}
		if err := os.WriteFile(targetPath, data, targetFileMode(path, data)); err != nil {
			return fmt.Errorf("failed to write file %s: %w", targetPath, err)
		}
		return nil
	})
}

func targetFileMode(path string, data []byte) fs.FileMode {
	mode := fs.FileMode(0o644)
	if shouldBeExecutable(path, data) {
		mode |= 0o111
	}
	return mode
}

func shouldBeExecutable(path string, data []byte) bool {
	if strings.EqualFold(filepath.Ext(path), ".sh") {
		return true
	}
	trimmed := bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	return bytes.HasPrefix(trimmed, []byte("#!"))
}
