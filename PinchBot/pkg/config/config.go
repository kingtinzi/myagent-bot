package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/caarlos0/env/v11"

	"github.com/sipeed/pinchbot/pkg/fileutil"
)

// rrCounter is a global counter for round-robin load balancing across models.
var rrCounter atomic.Uint64

// FlexibleStringSlice is a []string that also accepts JSON numbers,
// so allow_from can contain both "123" and 123.
type FlexibleStringSlice []string

func (f *FlexibleStringSlice) UnmarshalJSON(data []byte) error {
	// Try []string first
	var ss []string
	if err := json.Unmarshal(data, &ss); err == nil {
		*f = ss
		return nil
	}

	// Try []interface{} to handle mixed types
	var raw []any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	result := make([]string, 0, len(raw))
	for _, v := range raw {
		switch val := v.(type) {
		case string:
			result = append(result, val)
		case float64:
			result = append(result, fmt.Sprintf("%.0f", val))
		default:
			result = append(result, fmt.Sprintf("%v", val))
		}
	}
	*f = result
	return nil
}

type Config struct {
	Agents      AgentsConfig      `json:"agents"`
	Bindings    []AgentBinding    `json:"bindings,omitempty"`
	Session     SessionConfig     `json:"session,omitempty"`
	Plugins     PluginsConfig     `json:"plugins,omitempty"`
	Channels    ChannelsConfig    `json:"channels"`
	Providers   ProvidersConfig   `json:"providers,omitempty"`
	ModelList   []ModelConfig     `json:"model_list"` // New model-centric provider configuration
	PlatformAPI PlatformAPIConfig `json:"platform_api,omitempty"`
	Gateway     GatewayConfig     `json:"gateway"`
	Tools       ToolsConfig       `json:"tools"`
	Heartbeat   HeartbeatConfig   `json:"heartbeat"`
	Devices     DevicesConfig     `json:"devices"`

	// GraphMemory is loaded from config.graph-memory.json (see LoadConfig); not part of config.json.
	GraphMemory *GraphMemoryFileConfig `json:"-"`
}

// MarshalJSON implements custom JSON marshaling for Config
// to omit providers section when empty and session when empty
func (c Config) MarshalJSON() ([]byte, error) {
	type Alias Config
	aux := &struct {
		Providers *ProvidersConfig `json:"providers,omitempty"`
		Session   *SessionConfig   `json:"session,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(&c),
	}

	// Only include providers if not empty
	if !c.Providers.IsEmpty() {
		aux.Providers = &c.Providers
	}

	// Only include session if not empty
	if c.Session.DMScope != "" || len(c.Session.IdentityLinks) > 0 {
		aux.Session = &c.Session
	}

	return json.Marshal(aux)
}

type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults"`
	List     []AgentConfig `json:"list,omitempty"`
}

// AgentModelConfig supports both string and structured model config.
// String format: "gpt-4" (just primary, no fallbacks)
// Object format: {"primary": "gpt-4", "fallbacks": ["claude-haiku"]}
type AgentModelConfig struct {
	Primary   string   `json:"primary,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty"`
}

func (m *AgentModelConfig) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		m.Primary = s
		m.Fallbacks = nil
		return nil
	}
	type raw struct {
		Primary   string   `json:"primary"`
		Fallbacks []string `json:"fallbacks"`
	}
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	m.Primary = r.Primary
	m.Fallbacks = r.Fallbacks
	return nil
}

func (m AgentModelConfig) MarshalJSON() ([]byte, error) {
	if len(m.Fallbacks) == 0 && m.Primary != "" {
		return json.Marshal(m.Primary)
	}
	type raw struct {
		Primary   string   `json:"primary,omitempty"`
		Fallbacks []string `json:"fallbacks,omitempty"`
	}
	return json.Marshal(raw{Primary: m.Primary, Fallbacks: m.Fallbacks})
}

// AgentToolsProfile filters tool names for an agent (OpenClaw-style agents.defaults.tools / agents.list[].tools).
// Used by POST /tools/invoke (after gateway.tools) and by the agent LLM tool list. Optional profile field is reserved for future presets.
type AgentToolsProfile struct {
	Profile   *string  `json:"profile,omitempty"`
	Allow     []string `json:"allow,omitempty"`
	Deny      []string `json:"deny,omitempty"`
	AlsoAllow []string `json:"alsoAllow,omitempty"`
}

// MergeAgentToolsProfile combines defaults with a per-agent entry. Deny lists are unioned (case-insensitive keys).
// When the agent entry sets any allow or alsoAllow, those lists replace the defaults' allow/alsoAllow; otherwise
// defaults' allow/alsoAllow are inherited.
func MergeAgentToolsProfile(def *AgentToolsProfile, agent *AgentToolsProfile) *AgentToolsProfile {
	if def == nil && agent == nil {
		return nil
	}
	if agent == nil {
		return cloneAgentToolsProfile(def)
	}
	if def == nil {
		return cloneAgentToolsProfile(agent)
	}
	out := &AgentToolsProfile{}
	if agent.Profile != nil {
		out.Profile = agent.Profile
	} else if def.Profile != nil {
		out.Profile = def.Profile
	}
	deny := map[string]struct{}{}
	for _, d := range def.Deny {
		s := strings.ToLower(strings.TrimSpace(d))
		if s != "" {
			deny[s] = struct{}{}
		}
	}
	for _, d := range agent.Deny {
		s := strings.ToLower(strings.TrimSpace(d))
		if s != "" {
			deny[s] = struct{}{}
		}
	}
	for d := range deny {
		out.Deny = append(out.Deny, d)
	}
	sort.Strings(out.Deny)

	if len(agent.Allow) > 0 || len(agent.AlsoAllow) > 0 {
		out.Allow = append([]string(nil), agent.Allow...)
		out.AlsoAllow = append([]string(nil), agent.AlsoAllow...)
	} else {
		out.Allow = append([]string(nil), def.Allow...)
		out.AlsoAllow = append([]string(nil), def.AlsoAllow...)
	}
	return out
}

func cloneAgentToolsProfile(p *AgentToolsProfile) *AgentToolsProfile {
	if p == nil {
		return nil
	}
	out := &AgentToolsProfile{
		Allow:     append([]string(nil), p.Allow...),
		Deny:      append([]string(nil), p.Deny...),
		AlsoAllow: append([]string(nil), p.AlsoAllow...),
	}
	if p.Profile != nil {
		s := *p.Profile
		out.Profile = &s
	}
	return out
}

// DeniedByAgentToolsProfile applies OpenClaw-style per-agent allow/deny/alsoAllow on a merged profile.
// Deny wins. If allow or alsoAllow is non-empty, the tool must be listed (whitelist); empty allow/alsoAllow means no whitelist.
func DeniedByAgentToolsProfile(p *AgentToolsProfile, toolName string) bool {
	if p == nil {
		return false
	}
	n := strings.ToLower(strings.TrimSpace(toolName))
	if n == "" {
		return true
	}
	for _, d := range p.Deny {
		if strings.ToLower(strings.TrimSpace(d)) == n {
			return true
		}
	}
	allow := map[string]struct{}{}
	for _, a := range p.Allow {
		s := strings.ToLower(strings.TrimSpace(a))
		if s != "" {
			allow[s] = struct{}{}
		}
	}
	for _, a := range p.AlsoAllow {
		s := strings.ToLower(strings.TrimSpace(a))
		if s != "" {
			allow[s] = struct{}{}
		}
	}
	if len(allow) == 0 {
		return false
	}
	_, ok := allow[n]
	return !ok
}

type AgentConfig struct {
	ID        string             `json:"id"`
	Default   bool               `json:"default,omitempty"`
	Name      string             `json:"name,omitempty"`
	Workspace string             `json:"workspace,omitempty"`
	Model     *AgentModelConfig  `json:"model,omitempty"`
	Skills    []string           `json:"skills,omitempty"`
	Subagents *SubagentsConfig   `json:"subagents,omitempty"`
	Tools     *AgentToolsProfile `json:"tools,omitempty"`
}

type SubagentsConfig struct {
	AllowAgents []string          `json:"allow_agents,omitempty"`
	Model       *AgentModelConfig `json:"model,omitempty"`
}

type PeerMatch struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type BindingMatch struct {
	Channel   string     `json:"channel"`
	AccountID string     `json:"account_id,omitempty"`
	Peer      *PeerMatch `json:"peer,omitempty"`
	GuildID   string     `json:"guild_id,omitempty"`
	TeamID    string     `json:"team_id,omitempty"`
}

type AgentBinding struct {
	AgentID string       `json:"agent_id"`
	Match   BindingMatch `json:"match"`
}

type SessionConfig struct {
	DMScope       string              `json:"dm_scope,omitempty"`
	IdentityLinks map[string][]string `json:"identity_links,omitempty"`
}

type PluginsConfig struct {
	Enabled       []string `json:"enabled,omitempty"`
	ExtensionsDir string   `json:"extensions_dir,omitempty"`
	// GraphMemoryGoNative is legacy JSON: graph-memory is always Go-only (pkg/graphmemory).
	// The field is ignored at runtime; keep false or omit.
	GraphMemoryGoNative bool `json:"graph_memory_go_native,omitempty"`
	// NodeHost runs OpenClaw-style TS extensions (openclaw.plugin.json + index.ts) via Node + jiti.
	NodeHost   bool   `json:"node_host,omitempty"`
	NodeBinary string `json:"node_binary,omitempty"`
	// HostDir is the directory containing run.mjs and node_modules (default: PinchBot pkg/plugins/assets).
	HostDir string `json:"host_dir,omitempty"`
	// NodeHostStartRetries is spawn+init attempts on startup (default 3 when 0).
	NodeHostStartRetries int `json:"node_host_start_retries,omitempty"`
	// NodeHostMaxRecoveries is extra Execute attempts after IPC/process failure (default 2 when 0).
	NodeHostMaxRecoveries int `json:"node_host_max_recoveries,omitempty"`
	// NodeHostRestartDelayMs is backoff before restarting the Node process (default 500 when 0).
	NodeHostRestartDelayMs int `json:"node_host_restart_delay_ms,omitempty"`
	// LlmTask configures the native Go llm-task tool when plugins.enabled contains "llm-task".
	LlmTask *LlmTaskPluginConfig `json:"llm_task,omitempty"`
	// Slots mirrors OpenClaw-style plugin slots (e.g. contextEngine). PinchBot wires graph-memory
	// from Go directly; this field is kept for config parity and tooling.
	Slots map[string]string `json:"slots,omitempty"`
	// PluginSettings maps extension manifest id (case-insensitive) to a JSON object passed to
	// Node register(api).pluginConfig for that extension.
	PluginSettings map[string]map[string]any `json:"plugin_settings,omitempty"`
}

// LlmTaskPluginConfig holds optional defaults for the built-in llm-task tool (JSON-only sub-call).
type LlmTaskPluginConfig struct {
	DefaultProvider      string   `json:"default_provider,omitempty"`
	DefaultModel         string   `json:"default_model,omitempty"` // model_list model_name override
	DefaultAuthProfileID string   `json:"default_auth_profile_id,omitempty"`
	AllowedModels        []string `json:"allowed_models,omitempty"` // entries like "openai/gpt-4o"
	MaxTokens            int      `json:"max_tokens,omitempty"`
	TimeoutMs            int      `json:"timeout_ms,omitempty"`
}

func (p PluginsConfig) IsPluginEnabled(id string) bool {
	target := strings.TrimSpace(id)
	if target == "" {
		return false
	}
	for _, candidate := range p.Enabled {
		if strings.EqualFold(strings.TrimSpace(candidate), target) {
			return true
		}
	}
	return false
}

// RoutingConfig controls the intelligent model routing feature.
// When enabled, each incoming message is scored against structural features
// (message length, code blocks, tool call history, conversation depth, attachments).
// Messages scoring below Threshold are sent to LightModel; all others use the
// agent's primary model. This reduces cost and latency for simple tasks without
// requiring any keyword matching — all scoring is language-agnostic.
type RoutingConfig struct {
	Enabled    bool    `json:"enabled"`
	LightModel string  `json:"light_model"` // model_name from model_list to use for simple tasks
	Threshold  float64 `json:"threshold"`   // complexity score in [0,1]; score >= threshold → primary model
}

// ToolFeedbackConfig controls whether tool invocation details are sent to
// the active chat channel as runtime feedback.
type ToolFeedbackConfig struct {
	Enabled       bool `json:"enabled"         env:"PinchBot_AGENTS_DEFAULTS_TOOL_FEEDBACK_ENABLED"`
	MaxArgsLength int  `json:"max_args_length" env:"PinchBot_AGENTS_DEFAULTS_TOOL_FEEDBACK_MAX_ARGS_LENGTH"`
}

type AgentDefaults struct {
	Workspace                 string             `json:"workspace"                       env:"PinchBot_AGENTS_DEFAULTS_WORKSPACE"`
	RestrictToWorkspace       bool               `json:"restrict_to_workspace"           env:"PinchBot_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE"`
	AllowReadOutsideWorkspace bool               `json:"allow_read_outside_workspace"    env:"PinchBot_AGENTS_DEFAULTS_ALLOW_READ_OUTSIDE_WORKSPACE"`
	Provider                  string             `json:"provider"                        env:"PinchBot_AGENTS_DEFAULTS_PROVIDER"`
	ModelName                 string             `json:"model_name,omitempty"            env:"PinchBot_AGENTS_DEFAULTS_MODEL_NAME"`
	Model                     string             `json:"model"                           env:"PinchBot_AGENTS_DEFAULTS_MODEL"` // Deprecated: use model_name instead
	ModelFallbacks            []string           `json:"model_fallbacks,omitempty"`
	ImageModel                string             `json:"image_model,omitempty"           env:"PinchBot_AGENTS_DEFAULTS_IMAGE_MODEL"`
	ImageModelFallbacks       []string           `json:"image_model_fallbacks,omitempty"`
	MaxTokens                 int                `json:"max_tokens"                      env:"PinchBot_AGENTS_DEFAULTS_MAX_TOKENS"`
	Temperature               *float64           `json:"temperature,omitempty"           env:"PinchBot_AGENTS_DEFAULTS_TEMPERATURE"`
	MaxToolIterations         int                `json:"max_tool_iterations"             env:"PinchBot_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS"`
	SummarizeMessageThreshold int                `json:"summarize_message_threshold"     env:"PinchBot_AGENTS_DEFAULTS_SUMMARIZE_MESSAGE_THRESHOLD"`
	SummarizeTokenPercent     int                `json:"summarize_token_percent"         env:"PinchBot_AGENTS_DEFAULTS_SUMMARIZE_TOKEN_PERCENT"`
	MaxMediaSize              int                `json:"max_media_size,omitempty"        env:"PinchBot_AGENTS_DEFAULTS_MAX_MEDIA_SIZE"`
	Routing                   *RoutingConfig     `json:"routing,omitempty"`
	ToolFeedback              ToolFeedbackConfig `json:"tool_feedback,omitempty"`
	// ToolModel is the model_name (from model_list) used when the agent has tools
	// and will send a tool list to the LLM. Use a model with reliable tool-calling
	// (e.g. Qwen) if the primary/light models do not return tool_calls.
	ToolModel string `json:"tool_model,omitempty" env:"PinchBot_AGENTS_DEFAULTS_TOOL_MODEL"`
	// Tools optional filter for tool names (merged with each agents.list[].tools).
	Tools *AgentToolsProfile `json:"tools,omitempty"`
}

const (
	DefaultMaxMediaSize                = 20 * 1024 * 1024 // 20 MB
	DefaultWeComAIBotProcessingMessage = "⏳ Processing, please wait. The results will be sent shortly."
)

func (d *AgentDefaults) GetMaxMediaSize() int {
	if d.MaxMediaSize > 0 {
		return d.MaxMediaSize
	}
	return DefaultMaxMediaSize
}

// GetToolFeedbackMaxArgsLength returns the max number of characters included
// in tool arguments when tool feedback is published to channels.
func (d *AgentDefaults) GetToolFeedbackMaxArgsLength() int {
	if d.ToolFeedback.MaxArgsLength > 0 {
		return d.ToolFeedback.MaxArgsLength
	}
	return 300
}

// IsToolFeedbackEnabled returns true when tool feedback messages should be sent to chat.
func (d *AgentDefaults) IsToolFeedbackEnabled() bool {
	return d.ToolFeedback.Enabled
}

// GetModelName returns the effective model name for the agent defaults.
// It prefers the new "model_name" field but falls back to "model" for backward compatibility.
func (d *AgentDefaults) GetModelName() string {
	if d.ModelName != "" {
		return d.ModelName
	}
	return d.Model
}

type ChannelsConfig struct {
	WhatsApp   WhatsAppConfig   `json:"whatsapp"`
	Telegram   TelegramConfig   `json:"telegram"`
	Feishu     FeishuConfig     `json:"feishu"`
	Discord    DiscordConfig    `json:"discord"`
	MaixCam    MaixCamConfig    `json:"maixcam"`
	QQ         QQConfig         `json:"qq"`
	DingTalk   DingTalkConfig   `json:"dingtalk"`
	Slack      SlackConfig      `json:"slack"`
	Matrix     MatrixConfig     `json:"matrix"`
	LINE       LINEConfig       `json:"line"`
	OneBot     OneBotConfig     `json:"onebot"`
	WeCom      WeComConfig      `json:"wecom"`
	WeComApp   WeComAppConfig   `json:"wecom_app"`
	WeComAIBot WeComAIBotConfig `json:"wecom_aibot"`
	Pico       PicoConfig       `json:"pico"`
	IRC        IRCConfig        `json:"irc"`
}

// GroupTriggerConfig controls when the bot responds in group chats.
type GroupTriggerConfig struct {
	MentionOnly bool     `json:"mention_only,omitempty"`
	Prefixes    []string `json:"prefixes,omitempty"`
}

// TypingConfig controls typing indicator behavior (Phase 10).
type TypingConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

// PlaceholderConfig controls placeholder message behavior (Phase 10).
type PlaceholderConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	Text    string `json:"text,omitempty"`
}

type WhatsAppConfig struct {
	Enabled            bool                `json:"enabled"              env:"PinchBot_CHANNELS_WHATSAPP_ENABLED"`
	BridgeURL          string              `json:"bridge_url"           env:"PinchBot_CHANNELS_WHATSAPP_BRIDGE_URL"`
	UseNative          bool                `json:"use_native"           env:"PinchBot_CHANNELS_WHATSAPP_USE_NATIVE"`
	SessionStorePath   string              `json:"session_store_path"   env:"PinchBot_CHANNELS_WHATSAPP_SESSION_STORE_PATH"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"           env:"PinchBot_CHANNELS_WHATSAPP_ALLOW_FROM"`
	ReasoningChannelID string              `json:"reasoning_channel_id" env:"PinchBot_CHANNELS_WHATSAPP_REASONING_CHANNEL_ID"`
}

type TelegramConfig struct {
	Enabled            bool                `json:"enabled"                 env:"PinchBot_CHANNELS_TELEGRAM_ENABLED"`
	Token              string              `json:"token"                   env:"PinchBot_CHANNELS_TELEGRAM_TOKEN"`
	BaseURL            string              `json:"base_url"                env:"PinchBot_CHANNELS_TELEGRAM_BASE_URL"`
	Proxy              string              `json:"proxy"                   env:"PinchBot_CHANNELS_TELEGRAM_PROXY"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"              env:"PinchBot_CHANNELS_TELEGRAM_ALLOW_FROM"`
	UseMarkdownV2      bool                `json:"use_markdown_v2"         env:"PinchBot_CHANNELS_TELEGRAM_USE_MARKDOWN_V2"`
	GroupTrigger       GroupTriggerConfig  `json:"group_trigger,omitempty"`
	Typing             TypingConfig        `json:"typing,omitempty"`
	Placeholder        PlaceholderConfig   `json:"placeholder,omitempty"`
	ReasoningChannelID string              `json:"reasoning_channel_id"    env:"PinchBot_CHANNELS_TELEGRAM_REASONING_CHANNEL_ID"`
}

type FeishuConfig struct {
	Enabled             bool                `json:"enabled"                 env:"PinchBot_CHANNELS_FEISHU_ENABLED"`
	AppID               string              `json:"app_id"                  env:"PinchBot_CHANNELS_FEISHU_APP_ID"`
	AppSecret           string              `json:"app_secret"              env:"PinchBot_CHANNELS_FEISHU_APP_SECRET"`
	EncryptKey          string              `json:"encrypt_key"             env:"PinchBot_CHANNELS_FEISHU_ENCRYPT_KEY"`
	VerificationToken   string              `json:"verification_token"      env:"PinchBot_CHANNELS_FEISHU_VERIFICATION_TOKEN"`
	AllowFrom           FlexibleStringSlice `json:"allow_from"              env:"PinchBot_CHANNELS_FEISHU_ALLOW_FROM"`
	GroupTrigger        GroupTriggerConfig  `json:"group_trigger,omitempty"`
	Placeholder         PlaceholderConfig   `json:"placeholder,omitempty"`
	ReasoningChannelID  string              `json:"reasoning_channel_id"    env:"PinchBot_CHANNELS_FEISHU_REASONING_CHANNEL_ID"`
	RandomReactionEmoji FlexibleStringSlice `json:"random_reaction_emoji"   env:"PinchBot_CHANNELS_FEISHU_RANDOM_REACTION_EMOJI"`
	IsLark              bool                `json:"is_lark"                 env:"PinchBot_CHANNELS_FEISHU_IS_LARK"`
}

type DiscordConfig struct {
	Enabled            bool                `json:"enabled"                 env:"PinchBot_CHANNELS_DISCORD_ENABLED"`
	Token              string              `json:"token"                   env:"PinchBot_CHANNELS_DISCORD_TOKEN"`
	Proxy              string              `json:"proxy"                   env:"PinchBot_CHANNELS_DISCORD_PROXY"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"              env:"PinchBot_CHANNELS_DISCORD_ALLOW_FROM"`
	MentionOnly        bool                `json:"mention_only"            env:"PinchBot_CHANNELS_DISCORD_MENTION_ONLY"`
	GroupTrigger       GroupTriggerConfig  `json:"group_trigger,omitempty"`
	Typing             TypingConfig        `json:"typing,omitempty"`
	Placeholder        PlaceholderConfig   `json:"placeholder,omitempty"`
	ReasoningChannelID string              `json:"reasoning_channel_id"    env:"PinchBot_CHANNELS_DISCORD_REASONING_CHANNEL_ID"`
}

type MaixCamConfig struct {
	Enabled            bool                `json:"enabled"              env:"PinchBot_CHANNELS_MAIXCAM_ENABLED"`
	Host               string              `json:"host"                 env:"PinchBot_CHANNELS_MAIXCAM_HOST"`
	Port               int                 `json:"port"                 env:"PinchBot_CHANNELS_MAIXCAM_PORT"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"           env:"PinchBot_CHANNELS_MAIXCAM_ALLOW_FROM"`
	ReasoningChannelID string              `json:"reasoning_channel_id" env:"PinchBot_CHANNELS_MAIXCAM_REASONING_CHANNEL_ID"`
}

type QQConfig struct {
	Enabled              bool                `json:"enabled"                  env:"PinchBot_CHANNELS_QQ_ENABLED"`
	AppID                string              `json:"app_id"                   env:"PinchBot_CHANNELS_QQ_APP_ID"`
	AppSecret            string              `json:"app_secret"               env:"PinchBot_CHANNELS_QQ_APP_SECRET"`
	AllowFrom            FlexibleStringSlice `json:"allow_from"               env:"PinchBot_CHANNELS_QQ_ALLOW_FROM"`
	GroupTrigger         GroupTriggerConfig  `json:"group_trigger,omitempty"`
	MaxMessageLength     int                 `json:"max_message_length"       env:"PinchBot_CHANNELS_QQ_MAX_MESSAGE_LENGTH"`
	MaxBase64FileSizeMiB int64               `json:"max_base64_file_size_mib" env:"PinchBot_CHANNELS_QQ_MAX_BASE64_FILE_SIZE_MIB"`
	SendMarkdown         bool                `json:"send_markdown"            env:"PinchBot_CHANNELS_QQ_SEND_MARKDOWN"`
	ReasoningChannelID   string              `json:"reasoning_channel_id"     env:"PinchBot_CHANNELS_QQ_REASONING_CHANNEL_ID"`
}

type DingTalkConfig struct {
	Enabled            bool                `json:"enabled"                 env:"PinchBot_CHANNELS_DINGTALK_ENABLED"`
	ClientID           string              `json:"client_id"               env:"PinchBot_CHANNELS_DINGTALK_CLIENT_ID"`
	ClientSecret       string              `json:"client_secret"           env:"PinchBot_CHANNELS_DINGTALK_CLIENT_SECRET"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"              env:"PinchBot_CHANNELS_DINGTALK_ALLOW_FROM"`
	GroupTrigger       GroupTriggerConfig  `json:"group_trigger,omitempty"`
	ReasoningChannelID string              `json:"reasoning_channel_id"    env:"PinchBot_CHANNELS_DINGTALK_REASONING_CHANNEL_ID"`
}

type SlackConfig struct {
	Enabled            bool                `json:"enabled"                 env:"PinchBot_CHANNELS_SLACK_ENABLED"`
	BotToken           string              `json:"bot_token"               env:"PinchBot_CHANNELS_SLACK_BOT_TOKEN"`
	AppToken           string              `json:"app_token"               env:"PinchBot_CHANNELS_SLACK_APP_TOKEN"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"              env:"PinchBot_CHANNELS_SLACK_ALLOW_FROM"`
	GroupTrigger       GroupTriggerConfig  `json:"group_trigger,omitempty"`
	Typing             TypingConfig        `json:"typing,omitempty"`
	Placeholder        PlaceholderConfig   `json:"placeholder,omitempty"`
	ReasoningChannelID string              `json:"reasoning_channel_id"    env:"PinchBot_CHANNELS_SLACK_REASONING_CHANNEL_ID"`
}

type MatrixConfig struct {
	Enabled            bool                `json:"enabled"                 env:"PinchBot_CHANNELS_MATRIX_ENABLED"`
	Homeserver         string              `json:"homeserver"              env:"PinchBot_CHANNELS_MATRIX_HOMESERVER"`
	UserID             string              `json:"user_id"                 env:"PinchBot_CHANNELS_MATRIX_USER_ID"`
	AccessToken        string              `json:"access_token"            env:"PinchBot_CHANNELS_MATRIX_ACCESS_TOKEN"`
	DeviceID           string              `json:"device_id,omitempty"     env:"PinchBot_CHANNELS_MATRIX_DEVICE_ID"`
	JoinOnInvite       bool                `json:"join_on_invite"          env:"PinchBot_CHANNELS_MATRIX_JOIN_ON_INVITE"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"              env:"PinchBot_CHANNELS_MATRIX_ALLOW_FROM"`
	GroupTrigger       GroupTriggerConfig  `json:"group_trigger,omitempty"`
	Placeholder        PlaceholderConfig   `json:"placeholder,omitempty"`
	ReasoningChannelID string              `json:"reasoning_channel_id"    env:"PinchBot_CHANNELS_MATRIX_REASONING_CHANNEL_ID"`
}

type LINEConfig struct {
	Enabled            bool                `json:"enabled"                 env:"PinchBot_CHANNELS_LINE_ENABLED"`
	ChannelSecret      string              `json:"channel_secret"          env:"PinchBot_CHANNELS_LINE_CHANNEL_SECRET"`
	ChannelAccessToken string              `json:"channel_access_token"    env:"PinchBot_CHANNELS_LINE_CHANNEL_ACCESS_TOKEN"`
	WebhookHost        string              `json:"webhook_host"            env:"PinchBot_CHANNELS_LINE_WEBHOOK_HOST"`
	WebhookPort        int                 `json:"webhook_port"            env:"PinchBot_CHANNELS_LINE_WEBHOOK_PORT"`
	WebhookPath        string              `json:"webhook_path"            env:"PinchBot_CHANNELS_LINE_WEBHOOK_PATH"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"              env:"PinchBot_CHANNELS_LINE_ALLOW_FROM"`
	GroupTrigger       GroupTriggerConfig  `json:"group_trigger,omitempty"`
	Typing             TypingConfig        `json:"typing,omitempty"`
	Placeholder        PlaceholderConfig   `json:"placeholder,omitempty"`
	ReasoningChannelID string              `json:"reasoning_channel_id"    env:"PinchBot_CHANNELS_LINE_REASONING_CHANNEL_ID"`
}

type OneBotConfig struct {
	Enabled            bool                `json:"enabled"                 env:"PinchBot_CHANNELS_ONEBOT_ENABLED"`
	WSUrl              string              `json:"ws_url"                  env:"PinchBot_CHANNELS_ONEBOT_WS_URL"`
	AccessToken        string              `json:"access_token"            env:"PinchBot_CHANNELS_ONEBOT_ACCESS_TOKEN"`
	ReconnectInterval  int                 `json:"reconnect_interval"      env:"PinchBot_CHANNELS_ONEBOT_RECONNECT_INTERVAL"`
	GroupTriggerPrefix []string            `json:"group_trigger_prefix"    env:"PinchBot_CHANNELS_ONEBOT_GROUP_TRIGGER_PREFIX"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"              env:"PinchBot_CHANNELS_ONEBOT_ALLOW_FROM"`
	GroupTrigger       GroupTriggerConfig  `json:"group_trigger,omitempty"`
	Typing             TypingConfig        `json:"typing,omitempty"`
	Placeholder        PlaceholderConfig   `json:"placeholder,omitempty"`
	ReasoningChannelID string              `json:"reasoning_channel_id"    env:"PinchBot_CHANNELS_ONEBOT_REASONING_CHANNEL_ID"`
}

type WeComConfig struct {
	Enabled            bool                `json:"enabled"                 env:"PinchBot_CHANNELS_WECOM_ENABLED"`
	Token              string              `json:"token"                   env:"PinchBot_CHANNELS_WECOM_TOKEN"`
	EncodingAESKey     string              `json:"encoding_aes_key"        env:"PinchBot_CHANNELS_WECOM_ENCODING_AES_KEY"`
	WebhookURL         string              `json:"webhook_url"             env:"PinchBot_CHANNELS_WECOM_WEBHOOK_URL"`
	WebhookHost        string              `json:"webhook_host"            env:"PinchBot_CHANNELS_WECOM_WEBHOOK_HOST"`
	WebhookPort        int                 `json:"webhook_port"            env:"PinchBot_CHANNELS_WECOM_WEBHOOK_PORT"`
	WebhookPath        string              `json:"webhook_path"            env:"PinchBot_CHANNELS_WECOM_WEBHOOK_PATH"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"              env:"PinchBot_CHANNELS_WECOM_ALLOW_FROM"`
	ReplyTimeout       int                 `json:"reply_timeout"           env:"PinchBot_CHANNELS_WECOM_REPLY_TIMEOUT"`
	GroupTrigger       GroupTriggerConfig  `json:"group_trigger,omitempty"`
	ReasoningChannelID string              `json:"reasoning_channel_id"    env:"PinchBot_CHANNELS_WECOM_REASONING_CHANNEL_ID"`
}

type WeComAppConfig struct {
	Enabled            bool                `json:"enabled"                 env:"PinchBot_CHANNELS_WECOM_APP_ENABLED"`
	CorpID             string              `json:"corp_id"                 env:"PinchBot_CHANNELS_WECOM_APP_CORP_ID"`
	CorpSecret         string              `json:"corp_secret"             env:"PinchBot_CHANNELS_WECOM_APP_CORP_SECRET"`
	AgentID            int64               `json:"agent_id"                env:"PinchBot_CHANNELS_WECOM_APP_AGENT_ID"`
	Token              string              `json:"token"                   env:"PinchBot_CHANNELS_WECOM_APP_TOKEN"`
	EncodingAESKey     string              `json:"encoding_aes_key"        env:"PinchBot_CHANNELS_WECOM_APP_ENCODING_AES_KEY"`
	WebhookHost        string              `json:"webhook_host"            env:"PinchBot_CHANNELS_WECOM_APP_WEBHOOK_HOST"`
	WebhookPort        int                 `json:"webhook_port"            env:"PinchBot_CHANNELS_WECOM_APP_WEBHOOK_PORT"`
	WebhookPath        string              `json:"webhook_path"            env:"PinchBot_CHANNELS_WECOM_APP_WEBHOOK_PATH"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"              env:"PinchBot_CHANNELS_WECOM_APP_ALLOW_FROM"`
	ReplyTimeout       int                 `json:"reply_timeout"           env:"PinchBot_CHANNELS_WECOM_APP_REPLY_TIMEOUT"`
	GroupTrigger       GroupTriggerConfig  `json:"group_trigger,omitempty"`
	ReasoningChannelID string              `json:"reasoning_channel_id"    env:"PinchBot_CHANNELS_WECOM_APP_REASONING_CHANNEL_ID"`
}

type WeComAIBotConfig struct {
	Enabled            bool                `json:"enabled"                      env:"PinchBot_CHANNELS_WECOM_AIBOT_ENABLED"`
	BotID              string              `json:"bot_id,omitempty"             env:"PinchBot_CHANNELS_WECOM_AIBOT_BOT_ID"`
	Secret             string              `json:"secret,omitempty"             env:"PinchBot_CHANNELS_WECOM_AIBOT_SECRET"`
	Token              string              `json:"token,omitempty"              env:"PinchBot_CHANNELS_WECOM_AIBOT_TOKEN"`
	EncodingAESKey     string              `json:"encoding_aes_key,omitempty"   env:"PinchBot_CHANNELS_WECOM_AIBOT_ENCODING_AES_KEY"`
	WebhookPath        string              `json:"webhook_path,omitempty"       env:"PinchBot_CHANNELS_WECOM_AIBOT_WEBHOOK_PATH"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"                   env:"PinchBot_CHANNELS_WECOM_AIBOT_ALLOW_FROM"`
	ReplyTimeout       int                 `json:"reply_timeout"                env:"PinchBot_CHANNELS_WECOM_AIBOT_REPLY_TIMEOUT"`
	MaxSteps           int                 `json:"max_steps"                    env:"PinchBot_CHANNELS_WECOM_AIBOT_MAX_STEPS"`       // Maximum streaming steps
	WelcomeMessage     string              `json:"welcome_message"              env:"PinchBot_CHANNELS_WECOM_AIBOT_WELCOME_MESSAGE"` // Sent on enter_chat event; empty = no welcome
	ProcessingMessage  string              `json:"processing_message,omitempty" env:"PinchBot_CHANNELS_WECOM_AIBOT_PROCESSING_MESSAGE"`
	ReasoningChannelID string              `json:"reasoning_channel_id"         env:"PinchBot_CHANNELS_WECOM_AIBOT_REASONING_CHANNEL_ID"`
}

type PicoConfig struct {
	Enabled         bool                `json:"enabled"                     env:"PinchBot_CHANNELS_PICO_ENABLED"`
	Token           string              `json:"token"                       env:"PinchBot_CHANNELS_PICO_TOKEN"`
	AllowTokenQuery bool                `json:"allow_token_query,omitempty"`
	AllowOrigins    []string            `json:"allow_origins,omitempty"`
	PingInterval    int                 `json:"ping_interval,omitempty"`
	ReadTimeout     int                 `json:"read_timeout,omitempty"`
	WriteTimeout    int                 `json:"write_timeout,omitempty"`
	MaxConnections  int                 `json:"max_connections,omitempty"`
	AllowFrom       FlexibleStringSlice `json:"allow_from"                  env:"PinchBot_CHANNELS_PICO_ALLOW_FROM"`
	Placeholder     PlaceholderConfig   `json:"placeholder,omitempty"`
}

type IRCConfig struct {
	Enabled            bool                `json:"enabled"                 env:"PinchBot_CHANNELS_IRC_ENABLED"`
	Server             string              `json:"server"                  env:"PinchBot_CHANNELS_IRC_SERVER"`
	TLS                bool                `json:"tls"                     env:"PinchBot_CHANNELS_IRC_TLS"`
	Nick               string              `json:"nick"                    env:"PinchBot_CHANNELS_IRC_NICK"`
	User               string              `json:"user,omitempty"          env:"PinchBot_CHANNELS_IRC_USER"`
	RealName           string              `json:"real_name,omitempty"     env:"PinchBot_CHANNELS_IRC_REAL_NAME"`
	Password           string              `json:"password"                env:"PinchBot_CHANNELS_IRC_PASSWORD"`
	NickServPassword   string              `json:"nickserv_password"       env:"PinchBot_CHANNELS_IRC_NICKSERV_PASSWORD"`
	SASLUser           string              `json:"sasl_user"               env:"PinchBot_CHANNELS_IRC_SASL_USER"`
	SASLPassword       string              `json:"sasl_password"           env:"PinchBot_CHANNELS_IRC_SASL_PASSWORD"`
	Channels           FlexibleStringSlice `json:"channels"                env:"PinchBot_CHANNELS_IRC_CHANNELS"`
	RequestCaps        FlexibleStringSlice `json:"request_caps,omitempty"  env:"PinchBot_CHANNELS_IRC_REQUEST_CAPS"`
	AllowFrom          FlexibleStringSlice `json:"allow_from"              env:"PinchBot_CHANNELS_IRC_ALLOW_FROM"`
	GroupTrigger       GroupTriggerConfig  `json:"group_trigger,omitempty"`
	Typing             TypingConfig        `json:"typing,omitempty"`
	ReasoningChannelID string              `json:"reasoning_channel_id"    env:"PinchBot_CHANNELS_IRC_REASONING_CHANNEL_ID"`
}

type HeartbeatConfig struct {
	Enabled  bool `json:"enabled"  env:"PinchBot_HEARTBEAT_ENABLED"`
	Interval int  `json:"interval" env:"PinchBot_HEARTBEAT_INTERVAL"` // minutes, min 5
}

type DevicesConfig struct {
	Enabled    bool `json:"enabled"     env:"PinchBot_DEVICES_ENABLED"`
	MonitorUSB bool `json:"monitor_usb" env:"PinchBot_DEVICES_MONITOR_USB"`
}

type ProvidersConfig struct {
	Anthropic     ProviderConfig       `json:"anthropic"`
	OpenAI        OpenAIProviderConfig `json:"openai"`
	LiteLLM       ProviderConfig       `json:"litellm"`
	OpenRouter    ProviderConfig       `json:"openrouter"`
	Groq          ProviderConfig       `json:"groq"`
	Zhipu         ProviderConfig       `json:"zhipu"`
	VLLM          ProviderConfig       `json:"vllm"`
	Gemini        ProviderConfig       `json:"gemini"`
	Nvidia        ProviderConfig       `json:"nvidia"`
	Ollama        ProviderConfig       `json:"ollama"`
	Moonshot      ProviderConfig       `json:"moonshot"`
	ShengSuanYun  ProviderConfig       `json:"shengsuanyun"`
	DeepSeek      ProviderConfig       `json:"deepseek"`
	Cerebras      ProviderConfig       `json:"cerebras"`
	Vivgrid       ProviderConfig       `json:"vivgrid"`
	VolcEngine    ProviderConfig       `json:"volcengine"`
	GitHubCopilot ProviderConfig       `json:"github_copilot"`
	Antigravity   ProviderConfig       `json:"antigravity"`
	Qwen          ProviderConfig       `json:"qwen"`
	Mistral       ProviderConfig       `json:"mistral"`
	Avian         ProviderConfig       `json:"avian"`
}

// IsEmpty checks if all provider configs are empty (no API keys or API bases set)
// Note: WebSearch is an optimization option and doesn't count as "non-empty"
func (p ProvidersConfig) IsEmpty() bool {
	return p.Anthropic.APIKey == "" && p.Anthropic.APIBase == "" &&
		p.OpenAI.APIKey == "" && p.OpenAI.APIBase == "" &&
		p.LiteLLM.APIKey == "" && p.LiteLLM.APIBase == "" &&
		p.OpenRouter.APIKey == "" && p.OpenRouter.APIBase == "" &&
		p.Groq.APIKey == "" && p.Groq.APIBase == "" &&
		p.Zhipu.APIKey == "" && p.Zhipu.APIBase == "" &&
		p.VLLM.APIKey == "" && p.VLLM.APIBase == "" &&
		p.Gemini.APIKey == "" && p.Gemini.APIBase == "" &&
		p.Nvidia.APIKey == "" && p.Nvidia.APIBase == "" &&
		p.Ollama.APIKey == "" && p.Ollama.APIBase == "" &&
		p.Moonshot.APIKey == "" && p.Moonshot.APIBase == "" &&
		p.ShengSuanYun.APIKey == "" && p.ShengSuanYun.APIBase == "" &&
		p.DeepSeek.APIKey == "" && p.DeepSeek.APIBase == "" &&
		p.Cerebras.APIKey == "" && p.Cerebras.APIBase == "" &&
		p.Vivgrid.APIKey == "" && p.Vivgrid.APIBase == "" &&
		p.VolcEngine.APIKey == "" && p.VolcEngine.APIBase == "" &&
		p.GitHubCopilot.APIKey == "" && p.GitHubCopilot.APIBase == "" &&
		p.Antigravity.APIKey == "" && p.Antigravity.APIBase == "" &&
		p.Qwen.APIKey == "" && p.Qwen.APIBase == "" &&
		p.Mistral.APIKey == "" && p.Mistral.APIBase == "" &&
		p.Avian.APIKey == "" && p.Avian.APIBase == ""
}

// MarshalJSON implements custom JSON marshaling for ProvidersConfig
// to omit the entire section when empty
func (p ProvidersConfig) MarshalJSON() ([]byte, error) {
	if p.IsEmpty() {
		return []byte("null"), nil
	}
	type Alias ProvidersConfig
	return json.Marshal((*Alias)(&p))
}

type ProviderConfig struct {
	APIKey         string `json:"api_key"                   env:"PinchBot_PROVIDERS_{{.Name}}_API_KEY"`
	APIBase        string `json:"api_base"                  env:"PinchBot_PROVIDERS_{{.Name}}_API_BASE"`
	Proxy          string `json:"proxy,omitempty"           env:"PinchBot_PROVIDERS_{{.Name}}_PROXY"`
	RequestTimeout int    `json:"request_timeout,omitempty" env:"PinchBot_PROVIDERS_{{.Name}}_REQUEST_TIMEOUT"`
	AuthMethod     string `json:"auth_method,omitempty"     env:"PinchBot_PROVIDERS_{{.Name}}_AUTH_METHOD"`
	ConnectMode    string `json:"connect_mode,omitempty"    env:"PinchBot_PROVIDERS_{{.Name}}_CONNECT_MODE"` // only for Github Copilot, `stdio` or `grpc`
}

type OpenAIProviderConfig struct {
	ProviderConfig
	WebSearch bool `json:"web_search" env:"PinchBot_PROVIDERS_OPENAI_WEB_SEARCH"`
}

// ModelConfig represents a model-centric provider configuration.
// It allows adding new providers (especially OpenAI-compatible ones) via configuration only.
// The model field uses protocol prefix format: [protocol/]model-identifier
// Supported protocols: openai, anthropic, antigravity, claude-cli, codex-cli, github-copilot
// Default protocol is "openai" if no prefix is specified.
type ModelConfig struct {
	// Required fields
	ModelName string `json:"model_name"` // User-facing alias for the model
	Model     string `json:"model"`      // Protocol/model-identifier (e.g., "openai/gpt-4o", "anthropic/claude-sonnet-4.6")

	// HTTP-based providers
	APIBase   string   `json:"api_base,omitempty"`  // API endpoint URL
	APIKey    string   `json:"api_key"`             // API authentication key (single key)
	APIKeys   []string `json:"api_keys,omitempty"`  // API authentication keys (multiple keys for failover)
	Proxy     string   `json:"proxy,omitempty"`     // HTTP proxy URL
	Fallbacks []string `json:"fallbacks,omitempty"` // Fallback model names for failover

	// Special providers (CLI-based, OAuth, etc.)
	AuthMethod  string `json:"auth_method,omitempty"`  // Authentication method: oauth, token
	ConnectMode string `json:"connect_mode,omitempty"` // Connection mode: stdio, grpc
	Workspace   string `json:"workspace,omitempty"`    // Workspace path for CLI-based providers

	// Optional optimizations
	RPM            int    `json:"rpm,omitempty"`              // Requests per minute limit
	MaxTokensField string `json:"max_tokens_field,omitempty"` // Field name for max tokens (e.g., "max_completion_tokens")
	RequestTimeout int    `json:"request_timeout,omitempty"`
	ThinkingLevel  string `json:"thinking_level,omitempty"` // Extended thinking: off|low|medium|high|xhigh|adaptive
}

// Validate checks if the ModelConfig has all required fields.
func (c *ModelConfig) Validate() error {
	if c.ModelName == "" {
		return fmt.Errorf("model_name is required")
	}
	if c.Model == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}

// GatewayHTTPAuthConfig gates Gateway HTTP surfaces such as POST /tools/invoke.
// Mode: "none" (default when omitted), "token", or "password" — compare Bearer secret to Token or Password.
type GatewayHTTPAuthConfig struct {
	Mode     string `json:"mode,omitempty"`
	Token    string `json:"token,omitempty"`
	Password string `json:"password,omitempty"`
}

// GatewayHTTPToolsConfig adjusts the default HTTP tool deny list (OpenClaw-compatible keys).
type GatewayHTTPToolsConfig struct {
	Deny  []string `json:"deny,omitempty"`
	Allow []string `json:"allow,omitempty"`
}

// GatewayRateLimitConfig optional fixed window (per minute) for Gateway HTTP surfaces.
// When RequestsPerMinute > 0, limits apply per client: Bearer credential hash when Authorization is present, else client IP.
type GatewayRateLimitConfig struct {
	RequestsPerMinute int `json:"requests_per_minute,omitempty"`
}

type GatewayConfig struct {
	Host      string                    `json:"host" env:"PinchBot_GATEWAY_HOST"`
	Port      int                       `json:"port" env:"PinchBot_GATEWAY_PORT"`
	Auth      *GatewayHTTPAuthConfig    `json:"auth,omitempty"`
	Tools     *GatewayHTTPToolsConfig   `json:"tools,omitempty"`
	RateLimit *GatewayRateLimitConfig   `json:"rate_limit,omitempty"`
}

type PlatformAPIConfig struct {
	BaseURL        string `json:"base_url,omitempty" env:"PICOCLAW_PLATFORM_API_BASE_URL"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" env:"PICOCLAW_PLATFORM_API_TIMEOUT_SECONDS"`
}

type ToolConfig struct {
	Enabled bool `json:"enabled" env:"ENABLED"`
}

type BraveConfig struct {
	Enabled    bool   `json:"enabled"     env:"PinchBot_TOOLS_WEB_BRAVE_ENABLED"`
	APIKey     string `json:"api_key"     env:"PinchBot_TOOLS_WEB_BRAVE_API_KEY"`
	MaxResults int    `json:"max_results" env:"PinchBot_TOOLS_WEB_BRAVE_MAX_RESULTS"`
}

type TavilyConfig struct {
	Enabled    bool   `json:"enabled"     env:"PinchBot_TOOLS_WEB_TAVILY_ENABLED"`
	APIKey     string `json:"api_key"     env:"PinchBot_TOOLS_WEB_TAVILY_API_KEY"`
	BaseURL    string `json:"base_url"    env:"PinchBot_TOOLS_WEB_TAVILY_BASE_URL"`
	MaxResults int    `json:"max_results" env:"PinchBot_TOOLS_WEB_TAVILY_MAX_RESULTS"`
}

type DuckDuckGoConfig struct {
	Enabled    bool `json:"enabled"     env:"PinchBot_TOOLS_WEB_DUCKDUCKGO_ENABLED"`
	MaxResults int  `json:"max_results" env:"PinchBot_TOOLS_WEB_DUCKDUCKGO_MAX_RESULTS"`
}

type PerplexityConfig struct {
	Enabled    bool   `json:"enabled"     env:"PinchBot_TOOLS_WEB_PERPLEXITY_ENABLED"`
	APIKey     string `json:"api_key"     env:"PinchBot_TOOLS_WEB_PERPLEXITY_API_KEY"`
	MaxResults int    `json:"max_results" env:"PinchBot_TOOLS_WEB_PERPLEXITY_MAX_RESULTS"`
}

type SearXNGConfig struct {
	Enabled    bool   `json:"enabled"     env:"PinchBot_TOOLS_WEB_SEARXNG_ENABLED"`
	BaseURL    string `json:"base_url"    env:"PinchBot_TOOLS_WEB_SEARXNG_BASE_URL"`
	MaxResults int    `json:"max_results" env:"PinchBot_TOOLS_WEB_SEARXNG_MAX_RESULTS"`
}

type GLMSearchConfig struct {
	Enabled bool   `json:"enabled"  env:"PinchBot_TOOLS_WEB_GLM_ENABLED"`
	APIKey  string `json:"api_key"  env:"PinchBot_TOOLS_WEB_GLM_API_KEY"`
	BaseURL string `json:"base_url" env:"PinchBot_TOOLS_WEB_GLM_BASE_URL"`
	// SearchEngine specifies the search backend: "search_std" (default),
	// "search_pro", "search_pro_sogou", or "search_pro_quark".
	SearchEngine string `json:"search_engine" env:"PinchBot_TOOLS_WEB_GLM_SEARCH_ENGINE"`
	MaxResults   int    `json:"max_results"   env:"PinchBot_TOOLS_WEB_GLM_MAX_RESULTS"`
}

type WebToolsConfig struct {
	ToolConfig   `                 envPrefix:"PinchBot_TOOLS_WEB_"`
	Brave        BraveConfig      `                                json:"brave"`
	Tavily       TavilyConfig     `                                json:"tavily"`
	DuckDuckGo   DuckDuckGoConfig `                                json:"duckduckgo"`
	Perplexity   PerplexityConfig `                                json:"perplexity"`
	SearXNG      SearXNGConfig    `                                json:"searxng"`
	GLMSearch    GLMSearchConfig  `                                json:"glm_search"`
	PreferNative bool             `                                json:"prefer_native"                 env:"PinchBot_TOOLS_WEB_PREFER_NATIVE"`
	// Proxy is an optional proxy URL for web tools (http/https/socks5/socks5h).
	// For authenticated proxies, prefer HTTP_PROXY/HTTPS_PROXY env vars instead of embedding credentials in config.
	Proxy                string              `json:"proxy,omitempty"                  env:"PinchBot_TOOLS_WEB_PROXY"`
	FetchLimitBytes      int64               `json:"fetch_limit_bytes,omitempty"      env:"PinchBot_TOOLS_WEB_FETCH_LIMIT_BYTES"`
	PrivateHostWhitelist FlexibleStringSlice `json:"private_host_whitelist,omitempty" env:"PinchBot_TOOLS_WEB_PRIVATE_HOST_WHITELIST"`
}

type CronToolsConfig struct {
	ToolConfig         `     envPrefix:"PinchBot_TOOLS_CRON_"`
	ExecTimeoutMinutes int  `                                 env:"PinchBot_TOOLS_CRON_EXEC_TIMEOUT_MINUTES" json:"exec_timeout_minutes"` // 0 means no timeout
	AllowCommand       bool `                                 env:"PinchBot_TOOLS_CRON_ALLOW_COMMAND"        json:"allow_command"`
}

type ExecConfig struct {
	ToolConfig          `         envPrefix:"PinchBot_TOOLS_EXEC_"`
	EnableDenyPatterns  bool     `                                 env:"PinchBot_TOOLS_EXEC_ENABLE_DENY_PATTERNS"  json:"enable_deny_patterns"`
	AllowRemote         bool     `                                 env:"PinchBot_TOOLS_EXEC_ALLOW_REMOTE"          json:"allow_remote"`
	CustomDenyPatterns  []string `                                 env:"PinchBot_TOOLS_EXEC_CUSTOM_DENY_PATTERNS"  json:"custom_deny_patterns"`
	CustomAllowPatterns []string `                                 env:"PinchBot_TOOLS_EXEC_CUSTOM_ALLOW_PATTERNS" json:"custom_allow_patterns"`
	TimeoutSeconds      int      `                                 env:"PinchBot_TOOLS_EXEC_TIMEOUT_SECONDS"       json:"timeout_seconds"` // 0 means use default (60s)
}

type SkillsToolsConfig struct {
	ToolConfig            `                       envPrefix:"PinchBot_TOOLS_SKILLS_"`
	Registries            SkillsRegistriesConfig `                                   json:"registries"`
	MaxConcurrentSearches int                    `                                   json:"max_concurrent_searches" env:"PinchBot_TOOLS_SKILLS_MAX_CONCURRENT_SEARCHES"`
	SearchCache           SearchCacheConfig      `                                   json:"search_cache"`
}

type MediaCleanupConfig struct {
	ToolConfig `    envPrefix:"PinchBot_MEDIA_CLEANUP_"`
	MaxAge     int `                                    env:"PinchBot_MEDIA_CLEANUP_MAX_AGE"  json:"max_age_minutes"`
	Interval   int `                                    env:"PinchBot_MEDIA_CLEANUP_INTERVAL" json:"interval_minutes"`
}

type ToolsConfig struct {
	AllowReadPaths  []string           `json:"allow_read_paths"  env:"PinchBot_TOOLS_ALLOW_READ_PATHS"`
	AllowWritePaths []string           `json:"allow_write_paths" env:"PinchBot_TOOLS_ALLOW_WRITE_PATHS"`
	Web             WebToolsConfig     `json:"web"`
	Cron            CronToolsConfig    `json:"cron"`
	Exec            ExecConfig         `json:"exec"`
	Skills          SkillsToolsConfig  `json:"skills"`
	MediaCleanup    MediaCleanupConfig `json:"media_cleanup"`
	MCP             MCPConfig          `json:"mcp"`
	AppendFile      ToolConfig         `json:"append_file"                                              envPrefix:"PinchBot_TOOLS_APPEND_FILE_"`
	EditFile        ToolConfig         `json:"edit_file"                                                envPrefix:"PinchBot_TOOLS_EDIT_FILE_"`
	FindSkills      ToolConfig         `json:"find_skills"                                              envPrefix:"PinchBot_TOOLS_FIND_SKILLS_"`
	I2C             ToolConfig         `json:"i2c"                                                      envPrefix:"PinchBot_TOOLS_I2C_"`
	InstallSkill    ToolConfig         `json:"install_skill"                                            envPrefix:"PinchBot_TOOLS_INSTALL_SKILL_"`
	ListDir         ToolConfig         `json:"list_dir"                                                 envPrefix:"PinchBot_TOOLS_LIST_DIR_"`
	Message         ToolConfig         `json:"message"                                                  envPrefix:"PinchBot_TOOLS_MESSAGE_"`
	ReadFile        ToolConfig         `json:"read_file"                                                envPrefix:"PinchBot_TOOLS_READ_FILE_"`
	SendFile        ToolConfig         `json:"send_file"                                                envPrefix:"PinchBot_TOOLS_SEND_FILE_"`
	Spawn           ToolConfig         `json:"spawn"                                                    envPrefix:"PinchBot_TOOLS_SPAWN_"`
	SPI             ToolConfig         `json:"spi"                                                      envPrefix:"PinchBot_TOOLS_SPI_"`
	Subagent        ToolConfig         `json:"subagent"                                                 envPrefix:"PinchBot_TOOLS_SUBAGENT_"`
	WebFetch        ToolConfig         `json:"web_fetch"                                                envPrefix:"PinchBot_TOOLS_WEB_FETCH_"`
	WriteFile       ToolConfig         `json:"write_file"                                               envPrefix:"PinchBot_TOOLS_WRITE_FILE_"`
}

type SearchCacheConfig struct {
	MaxSize    int `json:"max_size"    env:"PinchBot_SKILLS_SEARCH_CACHE_MAX_SIZE"`
	TTLSeconds int `json:"ttl_seconds" env:"PinchBot_SKILLS_SEARCH_CACHE_TTL_SECONDS"`
}

type SkillsRegistriesConfig struct {
	ClawHub ClawHubRegistryConfig `json:"clawhub"`
}

type ClawHubRegistryConfig struct {
	Enabled         bool   `json:"enabled"           env:"PinchBot_SKILLS_REGISTRIES_CLAWHUB_ENABLED"`
	BaseURL         string `json:"base_url"          env:"PinchBot_SKILLS_REGISTRIES_CLAWHUB_BASE_URL"`
	AuthToken       string `json:"auth_token"        env:"PinchBot_SKILLS_REGISTRIES_CLAWHUB_AUTH_TOKEN"`
	SearchPath      string `json:"search_path"       env:"PinchBot_SKILLS_REGISTRIES_CLAWHUB_SEARCH_PATH"`
	SkillsPath      string `json:"skills_path"       env:"PinchBot_SKILLS_REGISTRIES_CLAWHUB_SKILLS_PATH"`
	DownloadPath    string `json:"download_path"     env:"PinchBot_SKILLS_REGISTRIES_CLAWHUB_DOWNLOAD_PATH"`
	Timeout         int    `json:"timeout"           env:"PinchBot_SKILLS_REGISTRIES_CLAWHUB_TIMEOUT"`
	MaxZipSize      int    `json:"max_zip_size"      env:"PinchBot_SKILLS_REGISTRIES_CLAWHUB_MAX_ZIP_SIZE"`
	MaxResponseSize int    `json:"max_response_size" env:"PinchBot_SKILLS_REGISTRIES_CLAWHUB_MAX_RESPONSE_SIZE"`
}

// MCPServerConfig defines configuration for a single MCP server
type MCPServerConfig struct {
	// Enabled indicates whether this MCP server is active
	Enabled bool `json:"enabled"`
	// Deferred controls whether this server's tools are registered as hidden.
	// Hidden tools can still be executed by exact name, but are omitted from
	// provider tool definitions by default.
	Deferred *bool `json:"deferred,omitempty"`
	// Command is the executable to run (e.g., "npx", "python", "/path/to/server")
	Command string `json:"command"`
	// Args are the arguments to pass to the command
	Args []string `json:"args,omitempty"`
	// Env are environment variables to set for the server process (stdio only)
	Env map[string]string `json:"env,omitempty"`
	// EnvFile is the path to a file containing environment variables (stdio only)
	EnvFile string `json:"env_file,omitempty"`
	// Type is "stdio", "sse", or "http" (default: stdio if command is set, sse if url is set)
	Type string `json:"type,omitempty"`
	// URL is used for SSE/HTTP transport
	URL string `json:"url,omitempty"`
	// Headers are HTTP headers to send with requests (sse/http only)
	Headers map[string]string `json:"headers,omitempty"`
}

// MCPConfig defines configuration for all MCP servers
type MCPConfig struct {
	ToolConfig `envPrefix:"PinchBot_TOOLS_MCP_"`
	// Servers is a map of server name to server configuration
	Servers map[string]MCPServerConfig `json:"servers,omitempty"`
}

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			gm, gerr := LoadGraphMemorySidecar(path)
			if gerr != nil {
				return nil, gerr
			}
			cfg.GraphMemory = gm
			return cfg, nil
		}
		return nil, err
	}

	// Pre-scan the JSON to check how many model_list entries the user provided.
	// Go's JSON decoder reuses existing slice backing-array elements rather than
	// zero-initializing them, so fields absent from the user's JSON (e.g. api_base)
	// would silently inherit values from the DefaultConfig template at the same
	// index position. We only reset cfg.ModelList when the user actually provides
	// entries; when count is 0 we keep DefaultConfig's built-in list as fallback.
	var tmp Config
	if err := json.Unmarshal(data, &tmp); err != nil {
		return nil, err
	}
	if len(tmp.ModelList) > 0 {
		cfg.ModelList = nil
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	// Migrate legacy channel config fields to new unified structures
	cfg.migrateChannelConfigs()

	// Auto-migrate: if only legacy providers config exists, convert to model_list
	if len(cfg.ModelList) == 0 && cfg.HasProvidersConfig() {
		cfg.ModelList = ConvertProvidersToModelList(cfg)
	}

	// When providers and model_list coexist, inherit missing credentials from
	// matching provider protocol sections to reduce duplicate config.
	if cfg.HasProvidersConfig() {
		InheritProviderCredentials(cfg.ModelList, cfg.Providers)
	}

	// Expand multi-key model configs into separate entries for key-level failover.
	cfg.ModelList = ExpandMultiKeyModels(cfg.ModelList)

	// Validate model_list for uniqueness and required fields
	if err := cfg.ValidateModelList(); err != nil {
		return nil, err
	}

	gm, err := LoadGraphMemorySidecar(path)
	if err != nil {
		return nil, err
	}
	cfg.GraphMemory = gm

	return cfg, nil
}

func (c *Config) migrateChannelConfigs() {
	// Discord: mention_only -> group_trigger.mention_only
	if c.Channels.Discord.MentionOnly && !c.Channels.Discord.GroupTrigger.MentionOnly {
		c.Channels.Discord.GroupTrigger.MentionOnly = true
	}

	// OneBot: group_trigger_prefix -> group_trigger.prefixes
	if len(c.Channels.OneBot.GroupTriggerPrefix) > 0 &&
		len(c.Channels.OneBot.GroupTrigger.Prefixes) == 0 {
		c.Channels.OneBot.GroupTrigger.Prefixes = c.Channels.OneBot.GroupTriggerPrefix
	}
}

func SaveConfig(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	// Use unified atomic write utility with explicit sync for flash storage reliability.
	return fileutil.WriteFileAtomic(path, data, 0o600)
}

// EnsureConfigAt creates the parent directory and writes a default config with workspace "workspace"
// (relative to the config's parent, i.e. <parent>/workspace). Idempotent: does nothing if path exists.
func EnsureConfigAt(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cfg := DefaultConfig()
	cfg.Agents.Defaults.Workspace = "workspace"
	return SaveConfig(path, cfg)
}

func (c *Config) WorkspacePath() string {
	return ResolveWorkspacePath(c.Agents.Defaults.Workspace)
}

func (c *Config) GetAPIKey() string {
	if c.Providers.OpenRouter.APIKey != "" {
		return c.Providers.OpenRouter.APIKey
	}
	if c.Providers.Anthropic.APIKey != "" {
		return c.Providers.Anthropic.APIKey
	}
	if c.Providers.OpenAI.APIKey != "" {
		return c.Providers.OpenAI.APIKey
	}
	if c.Providers.Gemini.APIKey != "" {
		return c.Providers.Gemini.APIKey
	}
	if c.Providers.Zhipu.APIKey != "" {
		return c.Providers.Zhipu.APIKey
	}
	if c.Providers.Groq.APIKey != "" {
		return c.Providers.Groq.APIKey
	}
	if c.Providers.VLLM.APIKey != "" {
		return c.Providers.VLLM.APIKey
	}
	if c.Providers.ShengSuanYun.APIKey != "" {
		return c.Providers.ShengSuanYun.APIKey
	}
	if c.Providers.Cerebras.APIKey != "" {
		return c.Providers.Cerebras.APIKey
	}
	return ""
}

func (c *Config) GetAPIBase() string {
	if c.Providers.OpenRouter.APIKey != "" {
		if c.Providers.OpenRouter.APIBase != "" {
			return c.Providers.OpenRouter.APIBase
		}
		return "https://openrouter.ai/api/v1"
	}
	if c.Providers.Zhipu.APIKey != "" {
		return c.Providers.Zhipu.APIBase
	}
	if c.Providers.VLLM.APIKey != "" && c.Providers.VLLM.APIBase != "" {
		return c.Providers.VLLM.APIBase
	}
	return ""
}

func expandHome(path string) string {
	if path == "" {
		return path
	}
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		if len(path) > 1 && (path[1] == '/' || path[1] == '\\') {
			return filepath.Join(home, path[2:])
		}
		return home
	}
	return path
}

// GetModelConfig returns the ModelConfig for the given model name.
// If multiple configs exist with the same model_name, it uses round-robin
// selection for load balancing. Returns an error if the model is not found.
func (c *Config) GetModelConfig(modelName string) (*ModelConfig, error) {
	matches := c.findMatches(modelName)
	if len(matches) == 0 {
		return nil, fmt.Errorf("model %q not found in model_list or providers", modelName)
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}

	// Multiple configs - use round-robin for load balancing
	idx := (rrCounter.Add(1) - 1) % uint64(len(matches))
	return &matches[idx], nil
}

// findMatches finds all ModelConfig entries with the given model_name.
func (c *Config) findMatches(modelName string) []ModelConfig {
	var matches []ModelConfig
	for i := range c.ModelList {
		if c.ModelList[i].ModelName == modelName {
			matches = append(matches, c.ModelList[i])
		}
	}
	return matches
}

// HasProvidersConfig checks if any provider in the old providers config has configuration.
func (c *Config) HasProvidersConfig() bool {
	return !c.Providers.IsEmpty()
}

// ValidateModelList validates all ModelConfig entries in the model_list.
// It checks that each model config is valid.
// Note: Multiple entries with the same model_name are allowed for load balancing.
func (c *Config) ValidateModelList() error {
	for i := range c.ModelList {
		if err := c.ModelList[i].Validate(); err != nil {
			return fmt.Errorf("model_list[%d]: %w", i, err)
		}
	}
	return nil
}

// MergeAPIKeys merges api_key and api_keys into one ordered, deduplicated list.
// Empty and whitespace-only keys are ignored.
func MergeAPIKeys(apiKey string, apiKeys []string) []string {
	seen := make(map[string]struct{}, len(apiKeys)+1)
	merged := make([]string, 0, len(apiKeys)+1)

	appendKey := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, exists := seen[v]; exists {
			return
		}
		seen[v] = struct{}{}
		merged = append(merged, v)
	}

	appendKey(apiKey)
	for _, key := range apiKeys {
		appendKey(key)
	}

	return merged
}

// ExpandMultiKeyModels expands model entries with multiple API keys into
// separate model_name entries for key-level failover.
//
// Example:
//   - input:  {"model_name":"glm-4.7","api_keys":["k1","k2","k3"]}
//   - output: {"model_name":"glm-4.7","api_key":"k1","fallbacks":["glm-4.7__key_1","glm-4.7__key_2"]}
//     {"model_name":"glm-4.7__key_1","api_key":"k2"}
//     {"model_name":"glm-4.7__key_2","api_key":"k3"}
func ExpandMultiKeyModels(models []ModelConfig) []ModelConfig {
	if len(models) == 0 {
		return models
	}

	expanded := make([]ModelConfig, 0, len(models))

	for _, model := range models {
		keys := MergeAPIKeys(model.APIKey, model.APIKeys)
		if len(keys) <= 1 {
			if model.APIKey == "" && len(keys) == 1 {
				model.APIKey = keys[0]
			}
			model.APIKeys = nil
			expanded = append(expanded, model)
			continue
		}

		primary := model
		primary.APIKey = keys[0]
		primary.APIKeys = nil

		keyFallbacks := make([]string, 0, len(keys)-1)
		for i := 1; i < len(keys); i++ {
			keyFallbacks = append(keyFallbacks, fmt.Sprintf("%s__key_%d", model.ModelName, i))
		}
		if len(keyFallbacks) > 0 {
			primary.Fallbacks = append(keyFallbacks, model.Fallbacks...)
		}
		expanded = append(expanded, primary)

		for i := 1; i < len(keys); i++ {
			item := model
			item.ModelName = fmt.Sprintf("%s__key_%d", model.ModelName, i)
			item.APIKey = keys[i]
			item.APIKeys = nil
			item.Fallbacks = nil
			expanded = append(expanded, item)
		}
	}

	return expanded
}

func (t *ToolsConfig) IsToolEnabled(name string) bool {
	switch name {
	case "web":
		return t.Web.Enabled
	case "cron":
		return t.Cron.Enabled
	case "exec":
		return t.Exec.Enabled
	case "skills":
		return t.Skills.Enabled
	case "media_cleanup":
		return t.MediaCleanup.Enabled
	case "append_file":
		return t.AppendFile.Enabled
	case "edit_file":
		return t.EditFile.Enabled
	case "find_skills":
		return t.FindSkills.Enabled
	case "i2c":
		return t.I2C.Enabled
	case "install_skill":
		return t.InstallSkill.Enabled
	case "list_dir":
		return t.ListDir.Enabled
	case "message":
		return t.Message.Enabled
	case "read_file":
		return t.ReadFile.Enabled
	case "spawn":
		return t.Spawn.Enabled
	case "spi":
		return t.SPI.Enabled
	case "subagent":
		return t.Subagent.Enabled
	case "web_fetch":
		return t.WebFetch.Enabled
	case "send_file":
		return t.SendFile.Enabled
	case "write_file":
		return t.WriteFile.Enabled
	case "mcp":
		return t.MCP.Enabled
	default:
		return true
	}
}
