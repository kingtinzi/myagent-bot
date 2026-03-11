// Chat handler for Launcher: POST /api/chat with {"message":"..."} forwards to the agent
// and returns {"response":"..."}.

package channels

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
)

const chatAPITimeout = 120 * time.Second

// ChatAPIHandler handles POST /api/chat for the Launcher chat window.
type ChatAPIHandler struct {
	bus     *bus.MessageBus
	launcher *LauncherChannel
	counter uint64
}

// NewChatAPIHandler returns an HTTP handler that accepts JSON {"message":"..."}
// and returns JSON {"response":"..."} by publishing to the bus and waiting for the launcher channel.
func NewChatAPIHandler(messageBus *bus.MessageBus, launcherCh *LauncherChannel) *ChatAPIHandler {
	return &ChatAPIHandler{bus: messageBus, launcher: launcherCh, counter: 0}
}

func (h *ChatAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		http.Error(w, "message required", http.StatusBadRequest)
		return
	}

	chatID := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.FormatUint(atomic.AddUint64(&h.counter, 1), 10)
	respCh := make(chan string, 1)
	h.launcher.RegisterWaiter(chatID, respCh)

	ctx := r.Context()
	if err := h.bus.PublishInbound(ctx, bus.InboundMessage{
		Channel:    "launcher",
		ChatID:     chatID,
		Content:    req.Message,
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
