package upstream

import (
	"context"
	"fmt"
	"sync"

	"github.com/sipeed/pinchbot/pkg/config"
	"github.com/sipeed/pinchbot/pkg/platformapi"
	"github.com/sipeed/pinchbot/pkg/providers"
	"github.com/sipeed/pinchbot/pkg/providers/protocoltypes"
)

type OfficialRoute struct {
	PublicModelID string             `json:"public_model_id"`
	ModelConfig   config.ModelConfig `json:"model_config"`
}

type Router struct {
	mu     sync.RWMutex
	routes map[string]config.ModelConfig
}

func NewRouter(routes []OfficialRoute) *Router {
	router := &Router{}
	router.UpdateRoutes(routes)
	return router
}

func (r *Router) UpdateRoutes(routes []OfficialRoute) {
	items := make(map[string]config.ModelConfig, len(routes))
	for _, route := range routes {
		items[route.PublicModelID] = route.ModelConfig
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = items
}

func (r *Router) Routes() []OfficialRoute {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]OfficialRoute, 0, len(r.routes))
	for publicModelID, modelConfig := range r.routes {
		items = append(items, OfficialRoute{
			PublicModelID: publicModelID,
			ModelConfig:   modelConfig,
		})
	}
	return items
}

func (r *Router) ProxyChat(
	ctx context.Context,
	userID string,
	request platformapi.ChatProxyRequest,
) (platformapi.ChatProxyResponse, error) {
	r.mu.RLock()
	modelCfg, ok := r.routes[request.ModelID]
	r.mu.RUnlock()
	if !ok {
		return platformapi.ChatProxyResponse{}, fmt.Errorf("official model %q not configured", request.ModelID)
	}
	provider, modelID, err := providers.CreateProviderFromConfig(&modelCfg)
	if err != nil {
		return platformapi.ChatProxyResponse{}, err
	}
	response, err := provider.Chat(ctx, toProviderMessages(request.Messages), toProviderTools(request.Tools), modelID, request.Options)
	if err != nil {
		return platformapi.ChatProxyResponse{}, err
	}
	return platformapi.ChatProxyResponse{
		Response: protocoltypes.LLMResponse(*response),
	}, nil
}

func toProviderMessages(items []protocoltypes.Message) []providers.Message {
	out := make([]providers.Message, 0, len(items))
	for _, item := range items {
		out = append(out, providers.Message(item))
	}
	return out
}

func toProviderTools(items []protocoltypes.ToolDefinition) []providers.ToolDefinition {
	out := make([]providers.ToolDefinition, 0, len(items))
	for _, item := range items {
		out = append(out, providers.ToolDefinition(item))
	}
	return out
}
