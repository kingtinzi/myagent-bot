// PicoClaw - Ultra-lightweight personal AI agent
// DingTalk channel implementation using Stream Mode

package dingtalk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/identity"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/media"
	"github.com/sipeed/picoclaw/pkg/utils"
)

// DingTalkChannel implements the Channel interface for DingTalk (钉钉)
// It uses WebSocket for receiving messages via stream mode and API for sending
type DingTalkChannel struct {
	*channels.BaseChannel
	config       config.DingTalkConfig
	clientID     string
	clientSecret string
	streamClient *client.StreamClient
	ctx          context.Context
	cancel       context.CancelFunc
	// Map to store session webhooks for each chat
	sessionWebhooks sync.Map // chatID -> sessionWebhook
	// Uploaded file records (PDF, Excel, etc.) for lazy read via tool
	fileStore   UploadedFileStore
	downloader  *DingTalkDownloader
	fileCacheDir string
}

// NewDingTalkChannel creates a new DingTalk channel instance
func NewDingTalkChannel(cfg config.DingTalkConfig, messageBus *bus.MessageBus) (*DingTalkChannel, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("dingtalk client_id and client_secret are required")
	}

	base := channels.NewBaseChannel("dingtalk", cfg, messageBus, cfg.AllowFrom,
		channels.WithMaxMessageLength(20000),
		channels.WithGroupTrigger(cfg.GroupTrigger),
		channels.WithReasoningChannelID(cfg.ReasoningChannelID),
	)

	return &DingTalkChannel{
		BaseChannel:   base,
		config:        cfg,
		clientID:      cfg.ClientID,
		clientSecret:  cfg.ClientSecret,
		fileStore:     NewUploadedFileStore(),
		downloader:    NewDingTalkDownloader(cfg.ClientID, cfg.ClientSecret),
		fileCacheDir:  "", // set in Start if needed
	}, nil
}

// Start initializes the DingTalk channel with Stream Mode
func (c *DingTalkChannel) Start(ctx context.Context) error {
	logger.InfoC("dingtalk", "Starting DingTalk channel (Stream Mode)...")

	if c.fileCacheDir == "" {
		c.fileCacheDir = filepath.Join(os.TempDir(), "picoclaw_dingtalk_cache")
	}
	c.ctx, c.cancel = context.WithCancel(ctx)

	// Create credential config
	cred := client.NewAppCredentialConfig(c.clientID, c.clientSecret)

	// Create the stream client with options
	c.streamClient = client.NewStreamClient(
		client.WithAppCredential(cred),
		client.WithAutoReconnect(true),
	)

	// Register chatbot callback handler (IChatBotMessageHandler is a function type)
	c.streamClient.RegisterChatBotCallbackRouter(c.onChatBotMessageReceived)

	// Start the stream client
	if err := c.streamClient.Start(c.ctx); err != nil {
		return fmt.Errorf("failed to start stream client: %w", err)
	}

	c.SetRunning(true)
	logger.InfoC("dingtalk", "DingTalk channel started (Stream Mode)")
	return nil
}

// Stop gracefully stops the DingTalk channel
func (c *DingTalkChannel) Stop(ctx context.Context) error {
	logger.InfoC("dingtalk", "Stopping DingTalk channel...")

	if c.cancel != nil {
		c.cancel()
	}

	if c.streamClient != nil {
		c.streamClient.Close()
	}

	c.SetRunning(false)
	logger.InfoC("dingtalk", "DingTalk channel stopped")
	return nil
}

// Send sends a message to DingTalk via the chatbot reply API
func (c *DingTalkChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return channels.ErrNotRunning
	}

	// Get session webhook from storage
	sessionWebhookRaw, ok := c.sessionWebhooks.Load(msg.ChatID)
	if !ok {
		return fmt.Errorf("no session_webhook found for chat %s, cannot send message", msg.ChatID)
	}

	sessionWebhook, ok := sessionWebhookRaw.(string)
	if !ok {
		return fmt.Errorf("invalid session_webhook type for chat %s", msg.ChatID)
	}

	logger.DebugCF("dingtalk", "Sending message", map[string]any{
		"chat_id": msg.ChatID,
		"preview": utils.Truncate(msg.Content, 100),
	})

	// Use the session webhook to send the reply
	return c.SendDirectReply(ctx, sessionWebhook, msg.Content)
}

// onChatBotMessageReceived implements the IChatBotMessageHandler function signature
// This is called by the Stream SDK when a new message arrives
// IChatBotMessageHandler is: func(c context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error)
func (c *DingTalkChannel) onChatBotMessageReceived(
	ctx context.Context,
	data *chatbot.BotCallbackDataModel,
) ([]byte, error) {
	msgtype := strings.TrimSpace(strings.ToLower(data.Msgtype))
	contentMap, _ := data.Content.(map[string]any)

	chatID := data.SenderStaffId
	if data.ConversationType != "1" {
		chatID = data.ConversationId
	}
	scope := channels.BuildMediaScope("dingtalk", chatID, data.MsgId)

	// Build text content and optional media refs / file record
	content := data.Text.Content
	if content == "" && contentMap != nil {
		if t, ok := contentMap["content"].(string); ok {
			content = strings.TrimSpace(t)
		}
	}

	var mediaRefs []string

	switch msgtype {
	case "picture", "image":
		downloadCode := c.extractDownloadCode(contentMap, "downloadCode", "pictureDownloadCode")
		if downloadCode != "" && c.GetMediaStore() != nil {
			ref := c.downloadMediaToStore(ctx, downloadCode, chatID, scope, "image", ".jpg")
			if ref != "" {
				mediaRefs = append(mediaRefs, ref)
			}
			if content == "" {
				content = "[图片]"
			}
		}
	case "voice", "audio":
		downloadCode := c.extractDownloadCode(contentMap, "downloadCode")
		if downloadCode != "" && c.GetMediaStore() != nil {
			ref := c.downloadMediaToStore(ctx, downloadCode, chatID, scope, "audio", ".amr")
			if ref != "" {
				mediaRefs = append(mediaRefs, ref)
			}
			if content == "" {
				content = "[语音]"
			}
		}
	case "file":
		downloadCode := c.extractDownloadCode(contentMap, "downloadCode")
		filename := c.extractString(contentMap, "fileName", "file_name", "filename")
		if filename == "" {
			filename = "file"
		}
		if downloadCode != "" && c.fileStore != nil {
			c.fileStore.Add(chatID, downloadCode, filename, "application/octet-stream")
			if content == "" {
				content = "[用户上传了文件：" + filename + "，可通过 read_uploaded_file 工具按需读取]"
			} else {
				content = content + "\n[用户上传了文件：" + filename + "，可通过 read_uploaded_file 工具按需读取]"
			}
		}
	default:
		// text or unknown: keep content as-is
		if msgtype != "text" && msgtype != "" && content == "" && len(mediaRefs) == 0 {
			return nil, nil
		}
	}

	if content == "" && len(mediaRefs) == 0 {
		return nil, nil
	}

	senderID := data.SenderStaffId
	senderNick := data.SenderNick

	// Store the session webhook for this chat so we can reply later
	c.sessionWebhooks.Store(chatID, data.SessionWebhook)

	metadata := map[string]string{
		"sender_name":       senderNick,
		"conversation_id":   data.ConversationId,
		"conversation_type": data.ConversationType,
		"platform":          "dingtalk",
		"session_webhook":   data.SessionWebhook,
	}

	var peer bus.Peer
	if data.ConversationType == "1" {
		peer = bus.Peer{Kind: "direct", ID: senderID}
	} else {
		peer = bus.Peer{Kind: "group", ID: data.ConversationId}
		respond, cleaned := c.ShouldRespondInGroup(false, content)
		if !respond {
			return nil, nil
		}
		content = cleaned
	}

	logger.DebugCF("dingtalk", "Received message", map[string]any{
		"sender_nick": senderNick,
		"sender_id":   senderID,
		"msgtype":     msgtype,
		"preview":     utils.Truncate(content, 50),
	})

	sender := bus.SenderInfo{
		Platform:    "dingtalk",
		PlatformID:  senderID,
		CanonicalID: identity.BuildCanonicalID("dingtalk", senderID),
		DisplayName: senderNick,
	}

	if !c.IsAllowedSender(sender) {
		return nil, nil
	}

	c.HandleMessage(ctx, peer, data.MsgId, senderID, chatID, content, mediaRefs, metadata, sender)
	return nil, nil
}

func (c *DingTalkChannel) extractDownloadCode(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func (c *DingTalkChannel) extractString(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k].(string); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// downloadMediaToStore downloads by downloadCode, stores in MediaStore, returns media ref or "".
func (c *DingTalkChannel) downloadMediaToStore(ctx context.Context, downloadCode, chatID, scope, kind, ext string) string {
	store := c.GetMediaStore()
	if store == nil {
		return ""
	}
	cacheDir := filepath.Join(c.fileCacheDir, chatID)
	localPath, err := c.downloader.DownloadToTemp(ctx, downloadCode, cacheDir, kind+ext)
	if err != nil {
		return ""
	}
	filename := filepath.Base(localPath)
	contentType := "image/jpeg"
	if kind == "audio" {
		contentType = "audio/amr"
	}
	ref, err := store.Store(localPath, media.MediaMeta{
		Filename:    filename,
		ContentType: contentType,
		Source:      "dingtalk",
	}, scope)
	if err != nil {
		logger.WarnCF("dingtalk", "MediaStore.Store failed", map[string]any{"error": err.Error()})
		return ""
	}
	return ref
}

// SendDirectReply sends a direct reply using the session webhook
func (c *DingTalkChannel) SendDirectReply(ctx context.Context, sessionWebhook, content string) error {
	replier := chatbot.NewChatbotReplier()

	// Convert string content to []byte for the API
	contentBytes := []byte(content)
	titleBytes := []byte("PicoClaw")

	// Send markdown formatted reply
	err := replier.SimpleReplyMarkdown(
		ctx,
		sessionWebhook,
		titleBytes,
		contentBytes,
	)
	if err != nil {
		return fmt.Errorf("dingtalk send: %w", channels.ErrTemporary)
	}

	return nil
}
