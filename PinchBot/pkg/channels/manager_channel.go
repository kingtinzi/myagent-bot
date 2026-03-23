package channels

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/sipeed/pinchbot/pkg/config"
)

type channelDefinition struct {
	name        string
	displayName string
	enabled     func(cfg *config.Config) bool
	snapshot    func(cfg *config.Config) any
}

var channelDefinitions = []channelDefinition{
	{
		name:        "telegram",
		displayName: "Telegram",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.Telegram.Enabled && ch.Telegram.Token != ""
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.Telegram },
	},
	{
		name:        "whatsapp_native",
		displayName: "WhatsApp Native",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.WhatsApp.Enabled && ch.WhatsApp.UseNative
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.WhatsApp },
	},
	{
		name:        "whatsapp",
		displayName: "WhatsApp",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.WhatsApp.Enabled && !ch.WhatsApp.UseNative && ch.WhatsApp.BridgeURL != ""
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.WhatsApp },
	},
	{
		name:        "feishu",
		displayName: "Feishu",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			return cfg.FeishuUsesBuiltinGoChannel()
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.Feishu },
	},
	{
		name:        "discord",
		displayName: "Discord",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.Discord.Enabled && ch.Discord.Token != ""
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.Discord },
	},
	{
		name:        "maixcam",
		displayName: "MaixCam",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			return cfg.Channels.MaixCam.Enabled
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.MaixCam },
	},
	{
		name:        "qq",
		displayName: "QQ",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			return cfg.Channels.QQ.Enabled
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.QQ },
	},
	{
		name:        "dingtalk",
		displayName: "DingTalk",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.DingTalk.Enabled && ch.DingTalk.ClientID != ""
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.DingTalk },
	},
	{
		name:        "slack",
		displayName: "Slack",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.Slack.Enabled && ch.Slack.BotToken != ""
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.Slack },
	},
	{
		name:        "matrix",
		displayName: "Matrix",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.Matrix.Enabled &&
				ch.Matrix.Homeserver != "" &&
				ch.Matrix.UserID != "" &&
				ch.Matrix.AccessToken != ""
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.Matrix },
	},
	{
		name:        "line",
		displayName: "LINE",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.LINE.Enabled && ch.LINE.ChannelAccessToken != ""
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.LINE },
	},
	{
		name:        "onebot",
		displayName: "OneBot",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.OneBot.Enabled && ch.OneBot.WSUrl != ""
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.OneBot },
	},
	{
		name:        "wecom",
		displayName: "WeCom",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.WeCom.Enabled && ch.WeCom.Token != ""
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.WeCom },
	},
	{
		name:        "wecom_aibot",
		displayName: "WeCom AI Bot",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.WeComAIBot.Enabled &&
				((ch.WeComAIBot.BotID != "" && ch.WeComAIBot.Secret != "") ||
					ch.WeComAIBot.Token != "")
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.WeComAIBot },
	},
	{
		name:        "wecom_app",
		displayName: "WeCom App",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.WeComApp.Enabled && ch.WeComApp.CorpID != ""
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.WeComApp },
	},
	{
		name:        "pico",
		displayName: "Pico",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.Pico.Enabled && ch.Pico.Token != ""
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.Pico },
	},
	{
		name:        "irc",
		displayName: "IRC",
		enabled: func(cfg *config.Config) bool {
			if cfg == nil {
				return false
			}
			ch := &cfg.Channels
			return ch.IRC.Enabled && ch.IRC.Server != ""
		},
		snapshot: func(cfg *config.Config) any { return cfg.Channels.IRC },
	},
}

func configuredChannelDefinitions(cfg *config.Config) []channelDefinition {
	if cfg == nil {
		return nil
	}

	result := make([]channelDefinition, 0, len(channelDefinitions))
	for _, def := range channelDefinitions {
		if def.enabled(cfg) {
			result = append(result, def)
		}
	}
	return result
}

func channelDefinitionForName(name string) (channelDefinition, bool) {
	for _, def := range channelDefinitions {
		if def.name == name {
			return def, true
		}
	}
	return channelDefinition{}, false
}

func toChannelHashes(cfg *config.Config) map[string]string {
	result := make(map[string]string)
	if cfg == nil {
		return result
	}

	for _, def := range configuredChannelDefinitions(cfg) {
		payload, err := json.Marshal(def.snapshot(cfg))
		if err != nil {
			continue
		}
		sum := md5.Sum(payload)
		result[def.name] = hex.EncodeToString(sum[:])
	}
	return result
}

func compareChannels(old, news map[string]string) (added, removed []string) {
	for key, newHash := range news {
		oldHash, exists := old[key]
		if !exists {
			added = append(added, key)
			continue
		}
		if oldHash != newHash {
			added = append(added, key)
			removed = append(removed, key)
		}
	}

	for key := range old {
		if _, exists := news[key]; !exists {
			removed = append(removed, key)
		}
	}

	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}
