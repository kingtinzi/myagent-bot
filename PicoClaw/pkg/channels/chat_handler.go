// Chat handler for Launcher: POST /api/chat with {"message":"..."} and optional
// {"attachments":["path1", ...]} forwards to the agent and returns {"response":"..."}.
// When mediaStore is set, local file paths in attachments are registered and
// sent as media:// refs so the agent/LLM can resolve them.

package channels

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/media"
)

const chatAPITimeout = 120 * time.Second

// ChatAPIHandler handles POST /api/chat for the Launcher chat window.
type ChatAPIHandler struct {
	bus        *bus.MessageBus
	launcher   *LauncherChannel
	mediaStore media.MediaStore
	counter    uint64
}

// NewChatAPIHandler returns an HTTP handler that accepts JSON {"message":"...", "attachments":["path", ...]}
// and returns JSON {"response":"..."} by publishing to the bus and waiting for the launcher channel.
// If mediaStore is non-nil, attachment paths are registered in the store and media refs are sent to the agent.
func NewChatAPIHandler(messageBus *bus.MessageBus, launcherCh *LauncherChannel, mediaStore media.MediaStore) *ChatAPIHandler {
	return &ChatAPIHandler{bus: messageBus, launcher: launcherCh, mediaStore: mediaStore, counter: 0}
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
