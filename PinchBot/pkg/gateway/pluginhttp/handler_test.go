package pluginhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/plugins"
)

func TestHandler_ServeHTTP_ServiceUnavailable(t *testing.T) {
	h := &Handler{
		Cfg:  &config.Config{},
		Host: nil,
		Route: plugins.PluginHTTPRoute{
			PluginID: "p",
			Method:   "GET",
			Path:     "/x",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rec.Code)
	}
}
