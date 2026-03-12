package platformapi

import (
	"testing"
)

func TestFileSessionStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSessionStore(dir)
	want := Session{
		AccessToken:  "token-1",
		RefreshToken: "refresh-1",
		UserID:       "user-1",
		Email:        "user@example.com",
	}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.AccessToken != want.AccessToken || got.UserID != want.UserID || got.Email != want.Email {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestFileSessionStoreClear(t *testing.T) {
	dir := t.TempDir()
	store := NewFileSessionStore(dir)
	if err := store.Save(Session{AccessToken: "token-1", UserID: "user-1"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("Load() after Clear() expected error")
	}
}
