package platformapi

import (
	"testing"
	"time"
)

func TestSessionIsExpired(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	if (Session{}).IsExpired(now) {
		t.Fatal("expected zero-value session to be treated as not expired")
	}
	if !(Session{ExpiresAt: now.Add(-time.Second).Unix()}).IsExpired(now) {
		t.Fatal("expected past expires_at to be treated as expired")
	}
	if !(Session{ExpiresAt: now.Unix()}).IsExpired(now) {
		t.Fatal("expected current expires_at boundary to be treated as expired")
	}
	if (Session{ExpiresAt: now.Add(time.Second).Unix()}).IsExpired(now) {
		t.Fatal("expected future expires_at to be treated as active")
	}
}
