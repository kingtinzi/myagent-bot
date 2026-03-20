package gatewayservice

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/sipeed/pinchbot/pkg/agent"
	"github.com/sipeed/pinchbot/pkg/bus"
	"github.com/sipeed/pinchbot/pkg/channels"
	"github.com/sipeed/pinchbot/pkg/channels/dingtalk"
	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/cron"
	"github.com/sipeed/pinchbot/pkg/devices"
	"github.com/sipeed/pinchbot/pkg/health"
	"github.com/sipeed/pinchbot/pkg/heartbeat"
	"github.com/sipeed/pinchbot/pkg/logger"
	"github.com/sipeed/pinchbot/pkg/media"
	"github.com/sipeed/pinchbot/pkg/platformapi"
	"github.com/sipeed/pinchbot/pkg/providers"
	"github.com/sipeed/pinchbot/pkg/state"
	"github.com/sipeed/pinchbot/pkg/tools"
	"github.com/sipeed/pinchbot/pkg/usage"
	"github.com/sipeed/pinchbot/pkg/voice"
)

type realRuntime struct {
	cfg              *config.Config
	opts             Options
	provider         providers.LLMProvider
	msgBus           *bus.MessageBus
	agentLoop        *agent.AgentLoop
	cronService      *cron.CronService
	heartbeatService *heartbeat.HeartbeatService
	deviceService    *devices.Service
	mediaStore       *media.FileMediaStore
	channelManager   *channels.Manager
	healthServer     *health.Server
	usageLogger      *usage.Logger
	cancel           context.CancelFunc
}

func buildRuntime(cfg *config.Config, opts Options) (runtimeController, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if opts.Debug {
		logger.SetLevel(logger.DEBUG)
	}

	provider, modelID, err := providers.CreateProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("error creating provider: %w", err)
	}
	if modelID != "" {
		cfg.Agents.Defaults.ModelName = modelID
	}

	msgBus := bus.NewMessageBus()
	agentLoop := agent.NewAgentLoop(cfg, msgBus, provider)
	sessionStore := platformapi.NewFileSessionStore(config.GetPinchBotHome())
	agentLoop.SetPlatformAccessTokenFallback(func(ctx context.Context) string {
		_ = ctx
		sess, err := sessionStore.Load()
		if err != nil {
			return ""
		}
		if sess.IsExpired(time.Now()) {
			return ""
		}
		return sess.AccessToken
	})

	execTimeout := time.Duration(cfg.Tools.Cron.ExecTimeoutMinutes) * time.Minute
	cronService, err := setupCronTool(agentLoop, msgBus, cfg.WorkspacePath(), cfg.Agents.Defaults.RestrictToWorkspace, execTimeout, cfg)
	if err != nil {
		return nil, err
	}

	heartbeatService := heartbeat.NewHeartbeatService(
		cfg.WorkspacePath(),
		cfg.Heartbeat.Interval,
		cfg.Heartbeat.Enabled,
	)
	heartbeatService.SetBus(msgBus)
	heartbeatService.SetHandler(func(prompt, channel, chatID string) *tools.ToolResult {
		if channel == "" || chatID == "" {
			channel, chatID = "cli", "direct"
		}
		response, err := agentLoop.ProcessHeartbeat(context.Background(), prompt, channel, chatID)
		if err != nil {
			return tools.ErrorResult(fmt.Sprintf("Heartbeat error: %v", err))
		}
		if response == "HEARTBEAT_OK" {
			return tools.SilentResult("Heartbeat OK")
		}
		return tools.SilentResult(response)
	})

	mediaStore := media.NewFileMediaStoreWithCleanup(media.MediaCleanerConfig{
		Enabled:  cfg.Tools.MediaCleanup.Enabled,
		MaxAge:   time.Duration(cfg.Tools.MediaCleanup.MaxAge) * time.Minute,
		Interval: time.Duration(cfg.Tools.MediaCleanup.Interval) * time.Minute,
	})

	channelManager, err := channels.NewManager(cfg, msgBus, mediaStore)
	if err != nil {
		return nil, fmt.Errorf("error creating channel manager: %w", err)
	}

	agentLoop.SetChannelManager(channelManager)
	agentLoop.SetMediaStore(mediaStore)
	usageLogger := usage.NewLogger(cfg.WorkspacePath())
	agentLoop.SetUsageLogger(usageLogger)

	if transcriber := voice.DetectTranscriber(cfg); transcriber != nil {
		agentLoop.SetTranscriber(transcriber)
	}
	if ch, ok := channelManager.GetChannel("dingtalk"); ok {
		if dt, ok := ch.(*dingtalk.DingTalkChannel); ok {
			if reader := dt.GetUploadedFileReader(); reader != nil {
				agentLoop.RegisterTool(tools.NewReadUploadedFileTool(reader))
			}
		}
	}

	stateManager := state.NewManager(cfg.WorkspacePath())
	deviceService := devices.NewService(devices.Config{
		Enabled:    cfg.Devices.Enabled,
		MonitorUSB: cfg.Devices.MonitorUSB,
	}, stateManager)
	deviceService.SetBus(msgBus)

	healthServer := health.NewServer(cfg.Gateway.Host, cfg.Gateway.Port)
	addr := fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.Port)
	channelManager.SetupHTTPServer(addr, healthServer)
	usageHandler := usage.NewHandler(cfg.WorkspacePath())
	channelManager.RegisterHandler("/usage", usageHandler)
	channelManager.RegisterHandler("/dashboard", usageHandler)
	launcherCh := channels.NewLauncherChannel()
	channelManager.AddChannel("launcher", launcherCh)
	channelManager.RegisterHandler("/api/chat", channels.NewChatAPIHandler(
		msgBus,
		launcherCh,
		mediaStore,
		channels.NewPlatformSessionValidator(cfg.PlatformAPI.BaseURL),
	))

	return &realRuntime{
		cfg:              cfg,
		opts:             opts,
		provider:         provider,
		msgBus:           msgBus,
		agentLoop:        agentLoop,
		cronService:      cronService,
		heartbeatService: heartbeatService,
		deviceService:    deviceService,
		mediaStore:       mediaStore,
		channelManager:   channelManager,
		healthServer:     healthServer,
		usageLogger:      usageLogger,
	}, nil
}

func (r *realRuntime) Start(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("runtime is nil")
	}
	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	r.mediaStore.Start()

	if err := r.cronService.Start(); err != nil {
		if r.opts.OnLog != nil {
			r.opts.OnLog(fmt.Sprintf("Error starting cron service: %v", err))
		}
	}
	if err := r.heartbeatService.Start(); err != nil {
		if r.opts.OnLog != nil {
			r.opts.OnLog(fmt.Sprintf("Error starting heartbeat service: %v", err))
		}
	}
	if err := r.deviceService.Start(runCtx); err != nil {
		if r.opts.OnLog != nil {
			r.opts.OnLog(fmt.Sprintf("Error starting device service: %v", err))
		}
	}
	if err := r.channelManager.StartAll(runCtx); err != nil {
		r.mediaStore.Stop()
		cancel()
		return err
	}

	go r.agentLoop.Run(runCtx)

	baseURL := gatewayBaseURL(r.cfg)
	if r.opts.OnLog != nil {
		r.opts.OnLog(fmt.Sprintf("✓ Gateway started on %s", baseURL))
		r.opts.OnLog(fmt.Sprintf("✓ Health endpoints available at %s/health and %s/ready", baseURL, baseURL))
		r.opts.OnLog(fmt.Sprintf("✓ Launcher chat API: POST %s/api/chat", baseURL))
	}
	return nil
}

func (r *realRuntime) Stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if r.cancel != nil {
		r.cancel()
	}
	if r.msgBus != nil {
		r.msgBus.Close()
	}
	if r.channelManager != nil {
		_ = r.channelManager.StopAll(ctx)
	}
	if r.deviceService != nil {
		r.deviceService.Stop()
	}
	if r.heartbeatService != nil {
		r.heartbeatService.Stop()
	}
	if r.cronService != nil {
		r.cronService.Stop()
	}
	if r.mediaStore != nil {
		r.mediaStore.Stop()
	}
	if r.agentLoop != nil {
		r.agentLoop.Stop()
	}
	if r.usageLogger != nil {
		_ = r.usageLogger.Close()
	}
	if cp, ok := r.provider.(providers.StatefulProvider); ok {
		cp.Close()
	}
	return nil
}

func (r *realRuntime) SetReloadFunc(fn func() error) {
	if r == nil {
		return
	}
	if r.agentLoop != nil {
		r.agentLoop.SetReloadFunc(fn)
	}
	if r.healthServer != nil {
		r.healthServer.SetReloadFunc(fn)
	}
}

func (r *realRuntime) ReloadChannels(ctx context.Context, cfg *config.Config) error {
	if r == nil {
		return fmt.Errorf("runtime is nil")
	}
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if r.channelManager == nil {
		return fmt.Errorf("channel manager is nil")
	}
	if err := r.channelManager.Reload(ctx, cfg); err != nil {
		return err
	}
	r.cfg = cfg
	return nil
}

func setupCronTool(
	agentLoop *agent.AgentLoop,
	msgBus *bus.MessageBus,
	workspace string,
	restrict bool,
	execTimeout time.Duration,
	cfg *config.Config,
) (*cron.CronService, error) {
	cronStorePath := filepath.Join(workspace, "cron", "jobs.json")
	cronService := cron.NewCronService(cronStorePath, nil)

	var cronTool *tools.CronTool
	if cfg.Tools.IsToolEnabled("cron") {
		var err error
		cronTool, err = tools.NewCronTool(cronService, agentLoop, msgBus, workspace, restrict, execTimeout, cfg)
		if err != nil {
			return nil, fmt.Errorf("critical error during CronTool initialization: %w", err)
		}
		agentLoop.RegisterTool(cronTool)
	}

	if cronTool != nil {
		cronService.SetOnJob(func(job *cron.CronJob) (string, error) {
			result := cronTool.ExecuteJob(context.Background(), job)
			return result, nil
		})
	}

	return cronService, nil
}
