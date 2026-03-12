package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sipeed/pinchbot/pkg/bus"
)

func TestChatAPIHandlerPassesBearerTokenMetadata(t *testing.T) {
	messageBus := bus.NewMessageBus()
	launcher := NewLauncherChannel()
	if err := launcher.Start(context.Background()); err != nil {
		t.Fatalf("launcher.Start() error = %v", err)
	}
	handler := NewChatAPIHandler(messageBus, launcher, nil)

	body, _ := json.Marshal(map[string]any{"message": "hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer session-token")
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	inbound, ok := messageBus.ConsumeInbound(context.Background())
	if !ok {
		t.Fatal("expected inbound message")
	}
	if got := inbound.Metadata["platform_access_token"]; got != "session-token" {
		t.Fatalf("platform_access_token = %q, want %q", got, "session-token")
	}
	if err := launcher.Send(context.Background(), bus.OutboundMessage{
		Channel: "launcher",
		ChatID:  inbound.ChatID,
		Content: "ok",
	}); err != nil {
		t.Fatalf("launcher.Send() error = %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not finish")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestChatAPIHandlerCleansUpWaiterOnCancelledRequest(t *testing.T) {
	messageBus := bus.NewMessageBus()
	launcher := NewLauncherChannel()
	if err := launcher.Start(context.Background()); err != nil {
		t.Fatalf("launcher.Start() error = %v", err)
	}
	handler := NewChatAPIHandler(messageBus, launcher, nil)

	body, _ := json.Marshal(map[string]any{"message": "hello"})
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	inbound, ok := messageBus.ConsumeInbound(context.Background())
	if !ok {
		t.Fatal("expected inbound message")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not finish after cancellation")
	}

	if _, ok := launcher.responses.Load(inbound.ChatID); ok {
		t.Fatal("expected waiter to be removed after request cancellation")
	}
}
