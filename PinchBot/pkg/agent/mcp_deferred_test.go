package agent

import (
	"testing"

	"github.com/sipeed/pinchbot/pkg/config"
)

func TestServerIsDeferred(t *testing.T) {
	trueValue := true
	falseValue := false

	tests := []struct {
		name string
		cfg  config.MCPServerConfig
		want bool
	}{
		{
			name: "nil deferred defaults to false",
			cfg: config.MCPServerConfig{
				Enabled: true,
			},
			want: false,
		},
		{
			name: "explicit true deferred",
			cfg: config.MCPServerConfig{
				Enabled:  true,
				Deferred: &trueValue,
			},
			want: true,
		},
		{
			name: "explicit false deferred",
			cfg: config.MCPServerConfig{
				Enabled:  true,
				Deferred: &falseValue,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serverIsDeferred(tt.cfg)
			if got != tt.want {
				t.Fatalf("serverIsDeferred() = %v, want %v", got, tt.want)
			}
		})
	}
}
