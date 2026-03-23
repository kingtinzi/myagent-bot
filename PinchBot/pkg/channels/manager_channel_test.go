package channels

import (
	"context"
	"reflect"
	"testing"

	"github.com/sipeed/pinchbot/pkg/bus"
	"github.com/sipeed/pinchbot/pkg/config"
)

type reloadTestChannel struct {
	BaseChannel
	startCalls int
	stopCalls  int
}

func (c *reloadTestChannel) Start(context.Context) error {
	c.startCalls++
	c.SetRunning(true)
	return nil
}

func (c *reloadTestChannel) Stop(context.Context) error {
	c.stopCalls++
	c.SetRunning(false)
	return nil
}

func (c *reloadTestChannel) Send(context.Context, bus.OutboundMessage) error {
	return nil
}

func closedSignal() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func TestToChannelHashes_DefaultHasNoChannels(t *testing.T) {
	cfg := config.DefaultConfig()
	hashes := toChannelHashes(cfg)
	if len(hashes) != 0 {
		t.Fatalf("expected no enabled channels, got %d", len(hashes))
	}
}

func TestToChannelHashes_FeishuOpenclawPluginSkipsBuiltin(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels.Feishu.Enabled = true
	cfg.Channels.Feishu.AppID = "cli_x"
	cfg.Channels.Feishu.AppSecret = "sec"
	cfg.Plugins.Enabled = append(cfg.Plugins.Enabled, config.OpenclawLarkPluginID)

	hashes := toChannelHashes(cfg)
	if _, ok := hashes["feishu"]; ok {
		t.Fatalf("expected feishu built-in channel omitted when openclaw-lark enabled, got hashes=%v", hashes)
	}
}

func TestToChannelHashes_FeishuEnabledWithoutPlugin(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Channels.Feishu.Enabled = true
	cfg.Channels.Feishu.AppID = "cli_x"
	cfg.Channels.Feishu.AppSecret = "sec"

	hashes := toChannelHashes(cfg)
	if _, ok := hashes["feishu"]; !ok {
		t.Fatalf("expected feishu hash when plugin not used, got %v", hashes)
	}
}

func TestCompareChannels_AddedRemovedChanged(t *testing.T) {
	oldHashes := map[string]string{
		"telegram": "a",
		"wecom":    "b",
	}
	newHashes := map[string]string{
		"telegram": "c", // changed
		"discord":  "d", // added
	}

	added, removed := compareChannels(oldHashes, newHashes)

	if want := []string{"discord", "telegram"}; !reflect.DeepEqual(added, want) {
		t.Fatalf("added = %#v, want %#v", added, want)
	}
	if want := []string{"telegram", "wecom"}; !reflect.DeepEqual(removed, want) {
		t.Fatalf("removed = %#v, want %#v", removed, want)
	}
}

func TestManagerReload_RemovesDisabledChannel(t *testing.T) {
	oldCfg := config.DefaultConfig()
	oldCfg.Channels.Telegram.Enabled = true
	oldCfg.Channels.Telegram.Token = "token-old"

	newCfg := config.DefaultConfig()

	telegram := &reloadTestChannel{}
	manager := &Manager{
		channels: map[string]Channel{
			"telegram": telegram,
		},
		workers: map[string]*channelWorker{
			"telegram": {
				queue:      make(chan bus.OutboundMessage),
				done:       closedSignal(),
				mediaQueue: make(chan bus.OutboundMediaMessage),
				mediaDone:  closedSignal(),
			},
		},
		config:        oldCfg,
		channelHashes: toChannelHashes(oldCfg),
	}

	if err := manager.Reload(context.Background(), newCfg); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	if telegram.stopCalls != 1 {
		t.Fatalf("stopCalls = %d, want 1", telegram.stopCalls)
	}
	if _, exists := manager.channels["telegram"]; exists {
		t.Fatal("expected telegram channel removed after reload")
	}
	if _, exists := manager.workers["telegram"]; exists {
		t.Fatal("expected telegram worker removed after reload")
	}
	if len(manager.channelHashes) != 0 {
		t.Fatalf("channelHashes = %#v, want empty", manager.channelHashes)
	}
}

func TestManagerReload_AddsAndStartsChannelWhenRunning(t *testing.T) {
	oldFactory, hadOldFactory := getFactory("telegram")
	t.Cleanup(func() {
		if hadOldFactory {
			RegisterFactory("telegram", oldFactory)
		}
	})

	var created *reloadTestChannel
	RegisterFactory("telegram", func(cfg *config.Config, _ *bus.MessageBus) (Channel, error) {
		created = &reloadTestChannel{}
		return created, nil
	})

	oldCfg := config.DefaultConfig()
	newCfg := config.DefaultConfig()
	newCfg.Channels.Telegram.Enabled = true
	newCfg.Channels.Telegram.Token = "token-new"

	mb := bus.NewMessageBus()
	defer mb.Close()

	manager := &Manager{
		channels:      make(map[string]Channel),
		workers:       make(map[string]*channelWorker),
		bus:           mb,
		config:        oldCfg,
		channelHashes: toChannelHashes(oldCfg),
		dispatchTask:  &asyncTask{cancel: func() {}},
	}

	if err := manager.Reload(context.Background(), newCfg); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	if created == nil {
		t.Fatal("expected telegram channel to be created")
	}
	if created.startCalls != 1 {
		t.Fatalf("startCalls = %d, want 1", created.startCalls)
	}
	if _, exists := manager.channels["telegram"]; !exists {
		t.Fatal("expected telegram channel to be registered")
	}
	if _, exists := manager.workers["telegram"]; !exists {
		t.Fatal("expected telegram worker to be created")
	}

	manager.UnregisterChannel("telegram")
}
