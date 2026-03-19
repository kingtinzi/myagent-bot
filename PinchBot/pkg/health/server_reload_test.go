package health

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newHealthMuxForTest(t *testing.T, s *Server) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	s.RegisterOnMux(mux)
	return mux
}

func TestReloadHandler_MethodNotAllowed(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	mux := newHealthMuxForTest(t, s)

	req := httptest.NewRequest(http.MethodGet, "/reload", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestReloadHandler_NotConfigured(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	mux := newHealthMuxForTest(t, s)

	req := httptest.NewRequest(http.MethodPost, "/reload", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestReloadHandler_Success(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	called := false
	s.SetReloadFunc(func() error {
		called = true
		return nil
	})
	mux := newHealthMuxForTest(t, s)

	req := httptest.NewRequest(http.MethodPost, "/reload", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !called {
		t.Fatal("expected reload callback to be called")
	}
}

func TestReloadHandler_Error(t *testing.T) {
	s := NewServer("127.0.0.1", 0)
	s.SetReloadFunc(func() error {
		return errors.New("boom")
	})
	mux := newHealthMuxForTest(t, s)

	req := httptest.NewRequest(http.MethodPost, "/reload", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
