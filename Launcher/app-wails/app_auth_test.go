package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sipeed/pinchbot/pkg/platformapi"
)

func TestGetAuthStateClearsUnauthorizedSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid bearer token", http.StatusUnauthorized)
	}))
	defer server.Close()

	baseDir := t.TempDir()
	store := platformapi.NewFileSessionStore(baseDir)
	if err := store.Save(platformapi.Session{
		AccessToken: "expired-token",
		UserID:      "user-1",
		Email:       "user@example.com",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	app := &App{
		platformClient: platformapi.NewClient(server.URL),
		sessionStore:   store,
	}

	state := app.GetAuthState()
	if state.Authenticated {
		t.Fatal("expected unauthorized session to be treated as signed out")
	}
	if _, err := os.Stat(filepath.Join(baseDir, "platform-session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected session file to be removed, err = %v", err)
	}
}

func TestChatClearsUnauthorizedSession(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid bearer token", http.StatusUnauthorized)
	}))
	defer gateway.Close()

	baseDir := t.TempDir()
	store := platformapi.NewFileSessionStore(baseDir)
	if err := store.Save(platformapi.Session{
		AccessToken: "expired-token",
		UserID:      "user-1",
		Email:       "user@example.com",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	app := &App{
		ctx:          context.Background(),
		gatewayURL:   gateway.URL,
		sessionStore: store,
	}

	_, err := app.Chat("hello", nil)
	if err == nil {
		t.Fatal("expected unauthorized chat session to fail")
	}
	if got := err.Error(); got != authRequiredErrorPrefix+"登录状态已过期，请重新登录" {
		t.Fatalf("Chat() error = %q, want auth-required prefix", got)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "platform-session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected session file to be removed, err = %v", err)
	}
}

func TestGetAuthStateClearsLocallyExpiredSessionWithoutUpstreamCall(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"user_id":"user-1","balance_fen":0,"currency":"CNY","updated_unix":1}`))
	}))
	defer server.Close()

	baseDir := t.TempDir()
	store := platformapi.NewFileSessionStore(baseDir)
	if err := store.Save(platformapi.Session{
		AccessToken: "expired-token",
		UserID:      "user-1",
		Email:       "user@example.com",
		ExpiresAt:   time.Now().Add(-time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	app := &App{
		platformClient: platformapi.NewClient(server.URL),
		sessionStore:   store,
	}

	state := app.GetAuthState()
	if state.Authenticated {
		t.Fatal("expected expired local session to be treated as signed out")
	}
	if called {
		t.Fatal("expected expired session to be rejected locally before upstream wallet call")
	}
	if _, err := os.Stat(filepath.Join(baseDir, "platform-session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected session file to be removed, err = %v", err)
	}
}

func TestChatClearsLocallyExpiredSessionWithoutGatewayCall(t *testing.T) {
	called := false
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"response":"ok"}`))
	}))
	defer gateway.Close()

	baseDir := t.TempDir()
	store := platformapi.NewFileSessionStore(baseDir)
	if err := store.Save(platformapi.Session{
		AccessToken: "expired-token",
		UserID:      "user-1",
		Email:       "user@example.com",
		ExpiresAt:   time.Now().Add(-time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	app := &App{
		ctx:          context.Background(),
		gatewayURL:   gateway.URL,
		sessionStore: store,
	}

	_, err := app.Chat("hello", nil)
	if err == nil {
		t.Fatal("expected locally expired chat session to fail")
	}
	if got := err.Error(); got != authRequiredErrorPrefix+"登录状态已过期，请重新登录" {
		t.Fatalf("Chat() error = %q, want auth-required expired-session error", got)
	}
	if called {
		t.Fatal("expected expired session to be rejected locally before gateway call")
	}
	if _, err := os.Stat(filepath.Join(baseDir, "platform-session.json")); !os.IsNotExist(err) {
		t.Fatalf("expected session file to be removed, err = %v", err)
	}
}
