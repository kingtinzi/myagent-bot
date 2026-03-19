package gatewayservice

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/pinchbot/pkg/config"
)

type stubRuntime struct {
	startCalls int
	stopCalls  int
	reloadFn   func() error
}

func (s *stubRuntime) Start(context.Context) error {
	s.startCalls++
	return nil
}

func (s *stubRuntime) Stop(context.Context) error {
	s.stopCalls++
	return nil
}

func (s *stubRuntime) SetReloadFunc(fn func() error) {
	s.reloadFn = fn
}

type channelReloadRuntime struct {
	stubRuntime
	channelReloadCalls int
	channelReloadErr   error
}

func (r *channelReloadRuntime) ReloadChannels(_ context.Context, _ *config.Config) error {
	r.channelReloadCalls++
	return r.channelReloadErr
}

func writeGatewayServiceConfig(t *testing.T, cfg *config.Config) string {
	t.Helper()
	homeDir := t.TempDir()
	t.Setenv("PINCHBOT_HOME", homeDir)
	cfgPath := filepath.Join(homeDir, "config.json")
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	return cfgPath
}

func TestNewUsesConfigGatewayAddressForHealthURLs(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "0.0.0.0"
	cfg.Gateway.Port = 28790
	cfgPath := writeGatewayServiceConfig(t, cfg)

	svc, err := New(Options{ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if got, want := svc.ConfigPath(), cfgPath; got != want {
		t.Fatalf("ConfigPath() = %q, want %q", got, want)
	}
	if got, want := svc.BaseURL(), "http://127.0.0.1:28790"; got != want {
		t.Fatalf("BaseURL() = %q, want %q", got, want)
	}
	if got, want := svc.HealthURL(), "http://127.0.0.1:28790/health"; got != want {
		t.Fatalf("HealthURL() = %q, want %q", got, want)
	}
	if got, want := svc.ReadyURL(), "http://127.0.0.1:28790/ready"; got != want {
		t.Fatalf("ReadyURL() = %q, want %q", got, want)
	}
}

func TestStartBootstrapsWorkspaceAndReusesSingleRuntime(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 28791
	cfgPath := writeGatewayServiceConfig(t, cfg)
	workspacePath := cfg.WorkspacePath()

	originalBootstrap := workspaceBootstrapper
	originalFactory := runtimeFactory
	t.Cleanup(func() {
		workspaceBootstrapper = originalBootstrap
		runtimeFactory = originalFactory
	})

	var bootstrapped []string
	workspaceBootstrapper = func(path string) error {
		bootstrapped = append(bootstrapped, path)
		return os.MkdirAll(path, 0o755)
	}

	runtime := &stubRuntime{}
	factoryCalls := 0
	runtimeFactory = func(cfg *config.Config, opts Options) (runtimeController, error) {
		factoryCalls++
		return runtime, nil
	}

	svc, err := New(Options{ConfigPath: cfgPath, ShutdownTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if err := svc.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if factoryCalls != 1 {
		t.Fatalf("runtimeFactory calls = %d, want 1", factoryCalls)
	}
	if runtime.startCalls != 1 {
		t.Fatalf("runtime startCalls = %d, want 1", runtime.startCalls)
	}
	if runtime.stopCalls != 1 {
		t.Fatalf("runtime stopCalls = %d, want 1", runtime.stopCalls)
	}
	if len(bootstrapped) != 1 || bootstrapped[0] != workspacePath {
		t.Fatalf("workspace bootstrap paths = %#v, want [%q]", bootstrapped, workspacePath)
	}
}

func TestRealServiceStartServesHealthReadyAndChatAPI(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = port
	cfgPath := writeGatewayServiceConfig(t, cfg)

	svc, err := New(Options{ConfigPath: cfgPath, ShutdownTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		_ = svc.Stop(context.Background())
	})

	waitDeadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(svc.ReadyURL())
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(waitDeadline) {
			t.Fatalf("service did not become ready at %s before timeout", svc.ReadyURL())
		}
		time.Sleep(100 * time.Millisecond)
	}

	for _, url := range []string{svc.HealthURL(), svc.ReadyURL()} {
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("GET %s error = %v", url, err)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			t.Fatalf("GET %s status = %d, want %d", url, resp.StatusCode, http.StatusOK)
		}
		_ = resp.Body.Close()
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, svc.BaseURL()+"/api/chat", strings.NewReader(`{"message":"hello"}`))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST /api/chat error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST /api/chat status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestTriggerReload_FailsWhenServiceNotRunning(t *testing.T) {
	cfg := config.DefaultConfig()
	cfgPath := writeGatewayServiceConfig(t, cfg)
	svc, err := New(Options{ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := svc.TriggerReload(); err == nil {
		t.Fatal("expected TriggerReload() to fail when service is not running")
	}
}

func TestTriggerReload_RebuildsRuntime(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 28792
	cfgPath := writeGatewayServiceConfig(t, cfg)

	originalBootstrap := workspaceBootstrapper
	originalFactory := runtimeFactory
	t.Cleanup(func() {
		workspaceBootstrapper = originalBootstrap
		runtimeFactory = originalFactory
	})

	workspaceBootstrapper = func(path string) error {
		return os.MkdirAll(path, 0o755)
	}

	runtime1 := &stubRuntime{}
	runtime2 := &stubRuntime{}
	factoryCalls := 0
	runtimeFactory = func(_ *config.Config, _ Options) (runtimeController, error) {
		factoryCalls++
		if factoryCalls == 1 {
			return runtime1, nil
		}
		return runtime2, nil
	}

	svc, err := New(Options{ConfigPath: cfgPath, ShutdownTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		_ = svc.Stop(context.Background())
	})

	if runtime1.reloadFn == nil {
		t.Fatal("expected reload hook to be attached on initial runtime")
	}
	if err := svc.TriggerReload(); err != nil {
		t.Fatalf("TriggerReload() error = %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if runtime1.stopCalls == 1 && runtime2.startCalls == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if runtime1.stopCalls != 1 {
		t.Fatalf("runtime1 stopCalls = %d, want 1", runtime1.stopCalls)
	}
	if runtime2.startCalls != 1 {
		t.Fatalf("runtime2 startCalls = %d, want 1", runtime2.startCalls)
	}
	if runtime2.reloadFn == nil {
		t.Fatal("expected reload hook to be attached on reloaded runtime")
	}
}

func TestTriggerReload_OnlyChannelChangesUseHotReload(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 28793
	cfgPath := writeGatewayServiceConfig(t, cfg)

	originalBootstrap := workspaceBootstrapper
	originalFactory := runtimeFactory
	t.Cleanup(func() {
		workspaceBootstrapper = originalBootstrap
		runtimeFactory = originalFactory
	})

	workspaceBootstrapper = func(path string) error {
		return os.MkdirAll(path, 0o755)
	}

	runtime := &channelReloadRuntime{}
	factoryCalls := 0
	runtimeFactory = func(_ *config.Config, _ Options) (runtimeController, error) {
		factoryCalls++
		return runtime, nil
	}

	svc, err := New(Options{ConfigPath: cfgPath, ShutdownTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		_ = svc.Stop(context.Background())
	})

	updated := config.DefaultConfig()
	updated.Gateway.Host = cfg.Gateway.Host
	updated.Gateway.Port = cfg.Gateway.Port
	updated.Channels.Telegram.Enabled = true
	updated.Channels.Telegram.Token = "reload-token"
	if err := config.SaveConfig(cfgPath, updated); err != nil {
		t.Fatalf("SaveConfig(updated) error = %v", err)
	}

	if err := svc.TriggerReload(); err != nil {
		t.Fatalf("TriggerReload() error = %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.channelReloadCalls == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if runtime.channelReloadCalls != 1 {
		t.Fatalf("channelReloadCalls = %d, want 1", runtime.channelReloadCalls)
	}
	if runtime.stopCalls != 0 {
		t.Fatalf("stopCalls = %d, want 0", runtime.stopCalls)
	}
	if factoryCalls != 1 {
		t.Fatalf("runtimeFactory calls = %d, want 1", factoryCalls)
	}
}
