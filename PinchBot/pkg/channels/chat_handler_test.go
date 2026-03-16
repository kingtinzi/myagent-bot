package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sipeed/pinchbot/pkg/bus"
	"github.com/sipeed/pinchbot/pkg/platformapi"
)

type stubChatSessionValidator struct {
	validateErr error
	tokens      []string
}

func (s *stubChatSessionValidator) ValidateAccessToken(_ context.Context, accessToken string) error {
	s.tokens = append(s.tokens, accessToken)
	return s.validateErr
}

func TestChatAPIHandlerPassesBearerTokenMetadata(t *testing.T) {
	messageBus := bus.NewMessageBus()
	launcher := NewLauncherChannel()
	if err := launcher.Start(context.Background()); err != nil {
		t.Fatalf("launcher.Start() error = %v", err)
	}
	handler := NewChatAPIHandler(messageBus, launcher, nil, nil)

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
	handler := NewChatAPIHandler(messageBus, launcher, nil, nil)

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

func TestChatAPIHandlerRejectsMissingBearerToken(t *testing.T) {
	messageBus := bus.NewMessageBus()
	launcher := NewLauncherChannel()
	if err := launcher.Start(context.Background()); err != nil {
		t.Fatalf("launcher.Start() error = %v", err)
	}
	validator := &stubChatSessionValidator{}
	handler := NewChatAPIHandler(messageBus, launcher, nil, validator)

	body, _ := json.Marshal(map[string]any{"message": "hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if len(validator.tokens) != 0 {
		t.Fatalf("validator tokens = %#v, want no validation call for missing token", validator.tokens)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if inbound, ok := messageBus.ConsumeInbound(ctx); ok {
		t.Fatalf("unexpected inbound message %#v", inbound)
	}
}

func TestChatAPIHandlerRejectsInvalidBearerToken(t *testing.T) {
	messageBus := bus.NewMessageBus()
	launcher := NewLauncherChannel()
	if err := launcher.Start(context.Background()); err != nil {
		t.Fatalf("launcher.Start() error = %v", err)
	}
	validator := &stubChatSessionValidator{
		validateErr: &platformapi.APIError{StatusCode: http.StatusUnauthorized, Message: "invalid bearer token"},
	}
	handler := NewChatAPIHandler(messageBus, launcher, nil, validator)

	body, _ := json.Marshal(map[string]any{"message": "hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if len(validator.tokens) != 1 || validator.tokens[0] != "invalid-token" {
		t.Fatalf("validator tokens = %#v, want invalid-token", validator.tokens)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if inbound, ok := messageBus.ConsumeInbound(ctx); ok {
		t.Fatalf("unexpected inbound message %#v", inbound)
	}
}

func TestChatAPIHandlerReturnsServiceUnavailableWhenSessionValidationFailsOpen(t *testing.T) {
	messageBus := bus.NewMessageBus()
	launcher := NewLauncherChannel()
	if err := launcher.Start(context.Background()); err != nil {
		t.Fatalf("launcher.Start() error = %v", err)
	}
	validator := &stubChatSessionValidator{
		validateErr: errors.New("platform unavailable"),
	}
	handler := NewChatAPIHandler(messageBus, launcher, nil, validator)

	body, _ := json.Marshal(map[string]any{"message": "hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer session-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
