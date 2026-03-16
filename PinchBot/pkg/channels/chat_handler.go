// Chat handler for Launcher: POST /api/chat with {"message":"..."} and optional
// {"attachments":["path1", ...]} forwards to the agent and returns {"response":"..."}.
// When mediaStore is set, local file paths in attachments are registered and
// sent as media:// refs so the agent/LLM can resolve them.

package channels

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sipeed/pinchbot/pkg/bus"
	"github.com/sipeed/pinchbot/pkg/logger"
	"github.com/sipeed/pinchbot/pkg/media"
	"github.com/sipeed/pinchbot/pkg/platformapi"
)

const chatAPITimeout = 120 * time.Second

// ChatAPIHandler handles POST /api/chat for the Launcher chat window.
type ChatAPIHandler struct {
	bus        *bus.MessageBus
	launcher   *LauncherChannel
	mediaStore media.MediaStore
	validator  SessionValidator
	counter    uint64
}

type SessionValidator interface {
	ValidateAccessToken(ctx context.Context, accessToken string) error
}

type PlatformSessionValidator struct {
	client *platformapi.Client
}

func NewPlatformSessionValidator(baseURL string) *PlatformSessionValidator {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	return &PlatformSessionValidator{client: platformapi.NewClient(baseURL)}
}

func (v *PlatformSessionValidator) ValidateAccessToken(ctx context.Context, accessToken string) error {
	if v == nil || v.client == nil {
		return nil
	}
	if strings.TrimSpace(accessToken) == "" {
		return &platformapi.APIError{StatusCode: http.StatusUnauthorized, Message: "missing bearer token"}
	}
	_, err := v.client.GetMe(ctx, accessToken)
	return err
}

// NewChatAPIHandler returns an HTTP handler that accepts JSON {"message":"...", "attachments":["path", ...]}
// and returns JSON {"response":"..."} by publishing to the bus and waiting for the launcher channel.
// If mediaStore is non-nil, attachment paths are registered in the store and media refs are sent to the agent.
func NewChatAPIHandler(messageBus *bus.MessageBus, launcherCh *LauncherChannel, mediaStore media.MediaStore, validator SessionValidator) *ChatAPIHandler {
	return &ChatAPIHandler{bus: messageBus, launcher: launcherCh, mediaStore: mediaStore, validator: validator, counter: 0}
}

func (h *ChatAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message     string   `json:"message"`
		Attachments []string `json:"attachments,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Message == "" && len(req.Attachments) == 0 {
		http.Error(w, "message or attachments required", http.StatusBadRequest)
		return
	}

	chatID := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.FormatUint(atomic.AddUint64(&h.counter, 1), 10)
	respCh := make(chan string, 1)
	h.launcher.RegisterWaiter(chatID, respCh)
	defer h.launcher.UnregisterWaiter(chatID)

	accessToken := ""
	metadata := map[string]string{}
	if authHeader := strings.TrimSpace(r.Header.Get("Authorization")); authHeader != "" {
		lower := strings.ToLower(authHeader)
		if strings.HasPrefix(lower, "bearer ") {
			accessToken = strings.TrimSpace(authHeader[7:])
		}
	}
	if h.validator != nil {
		if strings.TrimSpace(accessToken) == "" {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		if err := h.validator.ValidateAccessToken(r.Context(), accessToken); err != nil {
			if platformapi.IsStatusCode(err, http.StatusUnauthorized) {
				http.Error(w, "invalid bearer token", http.StatusUnauthorized)
				return
			}
			http.Error(w, "platform session validation unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	if accessToken != "" {
		metadata["platform_access_token"] = accessToken
	}

	mediaRefs := req.Attachments
	if h.mediaStore != nil && len(req.Attachments) > 0 {
		scope := "launcher:" + chatID
		mediaRefs = make([]string, 0, len(req.Attachments))
		for _, p := range req.Attachments {
			if strings.HasPrefix(p, "media://") {
				mediaRefs = append(mediaRefs, p)
				continue
			}
			ref, err := h.mediaStore.Store(p, media.MediaMeta{
				Filename: filepath.Base(p),
				Source:   "launcher",
			}, scope)
			if err != nil {
				logger.WarnCF("launcher", "Failed to register attachment", map[string]any{"path": p, "error": err.Error()})
				continue
			}
			mediaRefs = append(mediaRefs, ref)
		}
	}

	ctx := r.Context()
	if err := h.bus.PublishInbound(ctx, bus.InboundMessage{
		Channel:    "launcher",
		ChatID:     chatID,
		Content:    req.Message,
		Media:      mediaRefs,
		SessionKey: "launcher:" + chatID,
		Peer:       bus.Peer{Kind: "direct", ID: "launcher"},
		Metadata:   metadata,
	}); err != nil {
		http.Error(w, "bus unavailable", http.StatusServiceUnavailable)
		return
	}

	select {
	case response := <-respCh:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"response": response})
	case <-time.After(chatAPITimeout):
		http.Error(w, "timeout waiting for agent", http.StatusGatewayTimeout)
	case <-ctx.Done():
		http.Error(w, "request cancelled", http.StatusBadRequest)
	}
}
