package plugins

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplyPluginSettings(t *testing.T) {
	d := []DiscoveredPlugin{
		{ID: "Alpha", Root: "/a"},
		{ID: "beta", Root: "/b"},
	}
	settings := map[string]map[string]any{
		"alpha": {"x": 1},
		"BETA":  {"y": "z"},
	}
	out := ApplyPluginSettings(d, settings)
	if len(out) != 2 {
		t.Fatal(len(out))
	}
	if out[0].PluginConfig["x"] != 1 {
		t.Fatalf("alpha: %+v", out[0].PluginConfig)
	}
	if out[1].PluginConfig["y"] != "z" {
		t.Fatalf("beta: %+v", out[1].PluginConfig)
	}
	// Original slice unchanged
	if d[0].PluginConfig != nil {
		t.Fatal("expected original slice untouched")
	}
}

func TestBuildPluginInitStatus(t *testing.T) {
	cat := []CatalogTool{
		{PluginID: "p1", Name: "t1"},
		{PluginID: "p1", Name: "t2"},
		{PluginID: "p2", Name: "x"},
	}
	diags := []PluginInitDiagnostic{
		{PluginID: "p1", OK: true},
		{PluginID: "p2", OK: false, Error: "e"},
	}
	httpRoutes := []PluginHTTPRoute{
		{PluginID: "p1", Method: "GET", Path: "/a"},
		{PluginID: "p1", Method: "POST", Path: "/b"},
	}
	cliCommands := []PluginCLICommand{
		{PluginID: "p1", Name: "cmd-a", Description: "d1"},
		{PluginID: "p1", Name: "cmd-b"},
	}
	gatewayMethods := []PluginGatewayMethod{
		{PluginID: "p1", Method: "demo.ping"},
		{PluginID: "p1", Method: "demo.status"},
	}
	registeredServices := []PluginRegisteredService{
		{PluginID: "p1", ServiceID: "svc-a"},
	}
	registerCli := []PluginRegisterCli{
		{PluginID: "p1", Commands: []string{"rc-a", "rc-b"}},
		{PluginID: "p1", Commands: nil},
	}
	providerSnap := PluginProviderSnapshots{
		Text:               []PluginRegisteredProvider{{PluginID: "p1", ID: "prov-x", Label: "LX"}},
		Speech:             []PluginRegisteredProvider{{PluginID: "p1", ID: "sp1", Label: "SpL"}},
		MediaUnderstanding: []PluginRegisteredProvider{{PluginID: "p1", ID: "mu1"}},
		ImageGeneration:    []PluginRegisteredProvider{{PluginID: "p1", ID: "ig1", Label: "IG"}},
		WebSearch:          []PluginRegisteredProvider{{PluginID: "p1", ID: "ws1", Label: "WS"}},
	}
	extras := PluginInitExtras{
		Hooks: []PluginRegisteredHook{
			{PluginID: "p1", Events: []string{"before_model_resolve", "llm_input"}},
			{PluginID: "p1", Events: []string{"agent_end"}},
		},
		Channels: []PluginRegisteredChannel{
			{PluginID: "p1", ChannelID: "signal"},
		},
		InteractiveHandlers: []PluginInteractiveHandlerRegistration{
			{PluginID: "p1", Channel: "slack", Namespace: "ns-a"},
		},
		ConversationBindingListeners: []PluginConversationBindingListener{
			{PluginID: "p1"}, {PluginID: "p1"},
		},
	}
	st := BuildPluginInitStatus(cat, diags, httpRoutes, cliCommands, gatewayMethods, registeredServices, registerCli, providerSnap, extras)
	if len(st) != 2 {
		t.Fatal(st)
	}
	if st[0].PluginID != "p1" || !st[0].OK || len(st[0].Tools) != 2 || len(st[0].HTTPRoutes) != 2 || len(st[0].CLICommands) != 2 || len(st[0].GatewayMethods) != 2 || len(st[0].RegisteredServices) != 1 || len(st[0].RegisterCliCommands) != 3 || len(st[0].RegisteredProviders) != 1 ||
		len(st[0].RegisteredSpeechProviders) != 1 || len(st[0].RegisteredMediaUnderstandingProviders) != 1 || len(st[0].RegisteredImageGenerationProviders) != 1 || len(st[0].RegisteredWebSearchProviders) != 1 {
		t.Fatalf("p1: %+v", st[0])
	}
	if st[0].HTTPRoutes[0] != "GET /a" || st[0].HTTPRoutes[1] != "POST /b" {
		t.Fatalf("http routes: %+v", st[0].HTTPRoutes)
	}
	if st[0].CLICommands[0] != "cmd-a — d1" || st[0].CLICommands[1] != "cmd-b" {
		t.Fatalf("cli commands: %+v", st[0].CLICommands)
	}
	if st[0].GatewayMethods[0] != "demo.ping" || st[0].GatewayMethods[1] != "demo.status" {
		t.Fatalf("gateway methods: %+v", st[0].GatewayMethods)
	}
	if st[0].RegisteredServices[0] != "svc-a" {
		t.Fatalf("registered services: %+v", st[0].RegisteredServices)
	}
	if st[0].RegisterCliCommands[0] != "rc-a" || st[0].RegisterCliCommands[1] != "rc-b" || st[0].RegisterCliCommands[2] != "registerCli" {
		t.Fatalf("register cli: %+v", st[0].RegisterCliCommands)
	}
	if st[0].RegisteredProviders[0] != "prov-x — LX" {
		t.Fatalf("registered providers: %+v", st[0].RegisteredProviders)
	}
	if st[0].RegisteredSpeechProviders[0] != "sp1 — SpL" || st[0].RegisteredMediaUnderstandingProviders[0] != "mu1" ||
		st[0].RegisteredImageGenerationProviders[0] != "ig1 — IG" || st[0].RegisteredWebSearchProviders[0] != "ws1 — WS" {
		t.Fatalf("sibling providers: sp=%v mu=%v ig=%v ws=%v", st[0].RegisteredSpeechProviders, st[0].RegisteredMediaUnderstandingProviders, st[0].RegisteredImageGenerationProviders, st[0].RegisteredWebSearchProviders)
	}
	if len(st[0].RegisteredHooks) != 2 || st[0].RegisteredHooks[0] != "before_model_resolve, llm_input" || st[0].RegisteredHooks[1] != "agent_end" {
		t.Fatalf("hooks: %+v", st[0].RegisteredHooks)
	}
	if len(st[0].RegisteredChannels) != 1 || st[0].RegisteredChannels[0] != "signal" {
		t.Fatalf("channels: %+v", st[0].RegisteredChannels)
	}
	if len(st[0].RegisteredInteractiveHandlers) != 1 || st[0].RegisteredInteractiveHandlers[0] != "slack — ns-a" {
		t.Fatalf("interactive: %+v", st[0].RegisteredInteractiveHandlers)
	}
	if len(st[0].ConversationBindingResolvedListeners) != 2 || st[0].ConversationBindingResolvedListeners[0] != "onConversationBindingResolved" || st[0].ConversationBindingResolvedListeners[1] != "onConversationBindingResolved" {
		t.Fatalf("conv binding: %+v", st[0].ConversationBindingResolvedListeners)
	}
	if st[1].PluginID != "p2" || st[1].OK || st[1].Error != "e" || len(st[1].Tools) != 1 || len(st[1].HTTPRoutes) != 0 || len(st[1].CLICommands) != 0 || len(st[1].GatewayMethods) != 0 || len(st[1].RegisteredServices) != 0 || len(st[1].RegisterCliCommands) != 0 || len(st[1].RegisteredProviders) != 0 ||
		len(st[1].RegisteredSpeechProviders) != 0 || len(st[1].RegisteredMediaUnderstandingProviders) != 0 || len(st[1].RegisteredImageGenerationProviders) != 0 || len(st[1].RegisteredWebSearchProviders) != 0 ||
		len(st[1].RegisteredHooks) != 0 || len(st[1].RegisteredChannels) != 0 || len(st[1].RegisteredInteractiveHandlers) != 0 || len(st[1].ConversationBindingResolvedListeners) != 0 {
		t.Fatalf("p2: %+v", st[1])
	}
}

// PluginInitStatus is embedded in GET /plugins/status JSON; keep stable key names for operators and scripts.
func TestPluginInitStatus_JSONKeysForGateway(t *testing.T) {
	row := PluginInitStatus{
		PluginID:                              "demo",
		OK:                                    true,
		Tools:                                 []string{"t1"},
		HTTPRoutes:                            []string{"GET /x"},
		CLICommands:                           []string{"c — d"},
		GatewayMethods:                        []string{"m1"},
		RegisteredServices:                    []string{"svc"},
		RegisterCliCommands:                   []string{"rc"},
		RegisteredProviders:                   []string{"p"},
		RegisteredSpeechProviders:             []string{"sp"},
		RegisteredMediaUnderstandingProviders: []string{"mu"},
		RegisteredImageGenerationProviders:    []string{"ig"},
		RegisteredWebSearchProviders:          []string{"ws"},
		RegisteredHooks:                       []string{"e1, e2"},
		RegisteredChannels:                    []string{"ch"},
		RegisteredInteractiveHandlers:         []string{"slack — ns"},
		ConversationBindingResolvedListeners:  []string{"onConversationBindingResolved"},
	}
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, key := range []string{
		`"plugin_id"`,
		`"http_routes"`,
		`"cli_commands"`,
		`"gateway_methods"`,
		`"registered_services"`,
		`"register_cli_commands"`,
		`"registered_providers"`,
		`"registered_speech_providers"`,
		`"registered_media_understanding_providers"`,
		`"registered_image_generation_providers"`,
		`"registered_web_search_providers"`,
		`"registered_hooks"`,
		`"registered_channels"`,
		`"registered_interactive_handlers"`,
		`"conversation_binding_resolved_listeners"`,
	} {
		if !strings.Contains(s, key) {
			t.Fatalf("JSON missing %s: %s", key, s)
		}
	}
}
