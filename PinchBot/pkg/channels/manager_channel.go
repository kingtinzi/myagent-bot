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
	enabled     func(ch *config.ChannelsConfig) bool
	snapshot    func(ch *config.ChannelsConfig) any
}

var channelDefinitions = []channelDefinition{
	{
		name:        "telegram",
		displayName: "Telegram",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.Telegram.Enabled && ch.Telegram.Token != ""
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.Telegram },
	},
	{
		name:        "whatsapp_native",
		displayName: "WhatsApp Native",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.WhatsApp.Enabled && ch.WhatsApp.UseNative
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.WhatsApp },
	},
	{
		name:        "whatsapp",
		displayName: "WhatsApp",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.WhatsApp.Enabled && !ch.WhatsApp.UseNative && ch.WhatsApp.BridgeURL != ""
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.WhatsApp },
	},
	{
		name:        "feishu",
		displayName: "Feishu",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.Feishu.Enabled
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.Feishu },
	},
	{
		name:        "discord",
		displayName: "Discord",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.Discord.Enabled && ch.Discord.Token != ""
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.Discord },
	},
	{
		name:        "maixcam",
		displayName: "MaixCam",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.MaixCam.Enabled
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.MaixCam },
	},
	{
		name:        "qq",
		displayName: "QQ",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.QQ.Enabled
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.QQ },
	},
	{
		name:        "dingtalk",
		displayName: "DingTalk",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.DingTalk.Enabled && ch.DingTalk.ClientID != ""
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.DingTalk },
	},
	{
		name:        "slack",
		displayName: "Slack",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.Slack.Enabled && ch.Slack.BotToken != ""
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.Slack },
	},
	{
		name:        "matrix",
		displayName: "Matrix",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.Matrix.Enabled &&
				ch.Matrix.Homeserver != "" &&
				ch.Matrix.UserID != "" &&
				ch.Matrix.AccessToken != ""
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.Matrix },
	},
	{
		name:        "line",
		displayName: "LINE",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.LINE.Enabled && ch.LINE.ChannelAccessToken != ""
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.LINE },
	},
	{
		name:        "onebot",
		displayName: "OneBot",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.OneBot.Enabled && ch.OneBot.WSUrl != ""
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.OneBot },
	},
	{
		name:        "wecom",
		displayName: "WeCom",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.WeCom.Enabled && ch.WeCom.Token != ""
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.WeCom },
	},
	{
		name:        "wecom_aibot",
		displayName: "WeCom AI Bot",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.WeComAIBot.Enabled &&
				((ch.WeComAIBot.BotID != "" && ch.WeComAIBot.Secret != "") ||
					ch.WeComAIBot.Token != "")
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.WeComAIBot },
	},
	{
		name:        "wecom_app",
		displayName: "WeCom App",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.WeComApp.Enabled && ch.WeComApp.CorpID != ""
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.WeComApp },
	},
	{
		name:        "pico",
		displayName: "Pico",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.Pico.Enabled && ch.Pico.Token != ""
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.Pico },
	},
	{
		name:        "irc",
		displayName: "IRC",
		enabled: func(ch *config.ChannelsConfig) bool {
			return ch.IRC.Enabled && ch.IRC.Server != ""
		},
		snapshot: func(ch *config.ChannelsConfig) any { return ch.IRC },
	},
}

func configuredChannelDefinitions(channels *config.ChannelsConfig) []channelDefinition {
	if channels == nil {
		return nil
	}

	result := make([]channelDefinition, 0, len(channelDefinitions))
	for _, def := range channelDefinitions {
		if def.enabled(channels) {
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

	for _, def := range configuredChannelDefinitions(&cfg.Channels) {
		payload, err := json.Marshal(def.snapshot(&cfg.Channels))
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
