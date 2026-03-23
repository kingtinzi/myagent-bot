package plugins

import (
	"maps"
	"strings"
)

// ApplyPluginSettings merges cfg.Plugins.PluginSettings into discovered plugins by manifest id (case-insensitive).
// Values are shallow-copied so later mutation of the config map does not affect the slice passed to Node init.
func ApplyPluginSettings(discovered []DiscoveredPlugin, settings map[string]map[string]any) []DiscoveredPlugin {
	if len(settings) == 0 || len(discovered) == 0 {
		return discovered
	}
	byLower := make(map[string]map[string]any, len(settings))
	for k, v := range settings {
		kk := strings.ToLower(strings.TrimSpace(k))
		if kk == "" || len(v) == 0 {
			continue
		}
		byLower[kk] = v
	}
	if len(byLower) == 0 {
		return discovered
	}
	out := make([]DiscoveredPlugin, len(discovered))
	copy(out, discovered)
	for i := range out {
		lk := strings.ToLower(strings.TrimSpace(out[i].ID))
		pc, ok := byLower[lk]
		if !ok {
			continue
		}
		out[i].PluginConfig = maps.Clone(pc)
	}
	return out
}

// PluginInitStatus is one row for HTTP/CLI diagnostics (init result + tools from catalog).
type PluginInitStatus struct {
	PluginID                              string   `json:"plugin_id"`
	OK                                    bool     `json:"ok"`
	Error                                 string   `json:"error,omitempty"`
	Tools                                 []string `json:"tools,omitempty"`
	RuntimeStatus                         string   `json:"runtime_status,omitempty"` // ready | degraded | blocked
	Checks                                []RuntimeDependencyCheck `json:"checks,omitempty"`
	RepairActions                         []RuntimeRepairAction    `json:"repair_actions,omitempty"`
	HTTPRoutes                            []string `json:"http_routes,omitempty"`  // e.g. "GET /foo" from registerHttpRoute (Gateway dispatch optional)
	CLICommands                           []string `json:"cli_commands,omitempty"` // from registerCommand (execution not wired in PinchBot)
	GatewayMethods                        []string `json:"gateway_methods,omitempty"`
	RegisteredServices                    []string `json:"registered_services,omitempty"`
	RegisterCliCommands                   []string `json:"register_cli_commands,omitempty"` // from registerCli(opts.commands); placeholder "registerCli" if no names (handler not wired)
	RegisteredProviders                   []string `json:"registered_providers,omitempty"`  // registerProvider (id — label); inference not wired in PinchBot
	RegisteredSpeechProviders             []string `json:"registered_speech_providers,omitempty"`
	RegisteredMediaUnderstandingProviders []string `json:"registered_media_understanding_providers,omitempty"`
	RegisteredImageGenerationProviders    []string `json:"registered_image_generation_providers,omitempty"`
	RegisteredWebSearchProviders          []string `json:"registered_web_search_providers,omitempty"`
	RegisteredHooks                       []string `json:"registered_hooks,omitempty"`                        // registerHook; event names joined (comma-separated) per call
	RegisteredChannels                    []string `json:"registered_channels,omitempty"`                     // registerChannel channel plugin id
	RegisteredInteractiveHandlers         []string `json:"registered_interactive_handlers,omitempty"`         // channel — namespace
	ConversationBindingResolvedListeners  []string `json:"conversation_binding_resolved_listeners,omitempty"` // one line per onConversationBindingResolved(handler)
}

func providerStatusLines(rows []PluginRegisteredProvider) map[string][]string {
	out := make(map[string][]string)
	for _, rp := range rows {
		pid := strings.TrimSpace(rp.ID)
		if pid == "" {
			continue
		}
		line := pid
		if lb := strings.TrimSpace(rp.Label); lb != "" {
			line = pid + " — " + lb
		}
		out[rp.PluginID] = append(out[rp.PluginID], line)
	}
	return out
}

// BuildPluginInitStatus joins per-plugin diagnostics with catalog tool names (by plugin id), HTTP route declarations, CLI command metadata, gateway method names, registerService ids, registerCli declarations, register*Provider snapshots, and optional hook/channel/interactive/binding metadata from Node.
func BuildPluginInitStatus(cat []CatalogTool, diags []PluginInitDiagnostic, httpRoutes []PluginHTTPRoute, cliCommands []PluginCLICommand, gatewayMethods []PluginGatewayMethod, registeredServices []PluginRegisteredService, registerCli []PluginRegisterCli, providers PluginProviderSnapshots, extras PluginInitExtras) []PluginInitStatus {
	toolsByPlugin := make(map[string][]string)
	for _, ct := range cat {
		toolsByPlugin[ct.PluginID] = append(toolsByPlugin[ct.PluginID], ct.Name)
	}
	routesByPlugin := make(map[string][]string)
	for _, hr := range httpRoutes {
		line := strings.ToUpper(strings.TrimSpace(hr.Method)) + " " + strings.TrimSpace(hr.Path)
		routesByPlugin[hr.PluginID] = append(routesByPlugin[hr.PluginID], line)
	}
	commandsByPlugin := make(map[string][]string)
	for _, cc := range cliCommands {
		name := strings.TrimSpace(cc.Name)
		if name == "" {
			continue
		}
		line := name
		if d := strings.TrimSpace(cc.Description); d != "" {
			line = name + " — " + d
		}
		commandsByPlugin[cc.PluginID] = append(commandsByPlugin[cc.PluginID], line)
	}
	gatewayByPlugin := make(map[string][]string)
	for _, gm := range gatewayMethods {
		m := strings.TrimSpace(gm.Method)
		if m == "" {
			continue
		}
		gatewayByPlugin[gm.PluginID] = append(gatewayByPlugin[gm.PluginID], m)
	}
	servicesByPlugin := make(map[string][]string)
	for _, rs := range registeredServices {
		sid := strings.TrimSpace(rs.ServiceID)
		if sid == "" {
			continue
		}
		servicesByPlugin[rs.PluginID] = append(servicesByPlugin[rs.PluginID], sid)
	}
	registerCliByPlugin := make(map[string][]string)
	for _, rc := range registerCli {
		if len(rc.Commands) == 0 {
			registerCliByPlugin[rc.PluginID] = append(registerCliByPlugin[rc.PluginID], "registerCli")
			continue
		}
		for _, c := range rc.Commands {
			t := strings.TrimSpace(c)
			if t == "" {
				continue
			}
			registerCliByPlugin[rc.PluginID] = append(registerCliByPlugin[rc.PluginID], t)
		}
	}
	textProv := providerStatusLines(providers.Text)
	speechProv := providerStatusLines(providers.Speech)
	mediaProv := providerStatusLines(providers.MediaUnderstanding)
	imageGenProv := providerStatusLines(providers.ImageGeneration)
	webSearchProv := providerStatusLines(providers.WebSearch)

	hooksByPlugin := make(map[string][]string)
	for _, h := range extras.Hooks {
		pid := strings.TrimSpace(h.PluginID)
		if pid == "" {
			continue
		}
		var parts []string
		for _, e := range h.Events {
			t := strings.TrimSpace(e)
			if t != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) == 0 {
			continue
		}
		line := strings.Join(parts, ", ")
		hooksByPlugin[pid] = append(hooksByPlugin[pid], line)
	}
	channelsByPlugin := make(map[string][]string)
	for _, c := range extras.Channels {
		pid := strings.TrimSpace(c.PluginID)
		cid := strings.TrimSpace(c.ChannelID)
		if pid == "" || cid == "" {
			continue
		}
		channelsByPlugin[pid] = append(channelsByPlugin[pid], cid)
	}
	interactiveByPlugin := make(map[string][]string)
	for _, ih := range extras.InteractiveHandlers {
		pid := strings.TrimSpace(ih.PluginID)
		ch := strings.TrimSpace(ih.Channel)
		ns := strings.TrimSpace(ih.Namespace)
		if pid == "" || ch == "" || ns == "" {
			continue
		}
		line := ch + " — " + ns
		interactiveByPlugin[pid] = append(interactiveByPlugin[pid], line)
	}
	convBindByPlugin := make(map[string][]string)
	for _, lb := range extras.ConversationBindingListeners {
		pid := strings.TrimSpace(lb.PluginID)
		if pid == "" {
			continue
		}
		convBindByPlugin[pid] = append(convBindByPlugin[pid], "onConversationBindingResolved")
	}

	out := make([]PluginInitStatus, 0, len(diags))
	for _, d := range diags {
		runtimeStatus, checks, repairs := runtimeProbe(d.PluginID)
		if !d.OK {
			runtimeStatus = RuntimeStatusBlocked
		} else if strings.TrimSpace(runtimeStatus) == "" {
			runtimeStatus = RuntimeStatusReady
		}
		out = append(out, PluginInitStatus{
			PluginID:                              d.PluginID,
			OK:                                    d.OK,
			Error:                                 d.Error,
			Tools:                                 append([]string(nil), toolsByPlugin[d.PluginID]...),
			RuntimeStatus:                         runtimeStatus,
			Checks:                                append([]RuntimeDependencyCheck(nil), checks...),
			RepairActions:                         append([]RuntimeRepairAction(nil), repairs...),
			HTTPRoutes:                            append([]string(nil), routesByPlugin[d.PluginID]...),
			CLICommands:                           append([]string(nil), commandsByPlugin[d.PluginID]...),
			GatewayMethods:                        append([]string(nil), gatewayByPlugin[d.PluginID]...),
			RegisteredServices:                    append([]string(nil), servicesByPlugin[d.PluginID]...),
			RegisterCliCommands:                   append([]string(nil), registerCliByPlugin[d.PluginID]...),
			RegisteredProviders:                   append([]string(nil), textProv[d.PluginID]...),
			RegisteredSpeechProviders:             append([]string(nil), speechProv[d.PluginID]...),
			RegisteredMediaUnderstandingProviders: append([]string(nil), mediaProv[d.PluginID]...),
			RegisteredImageGenerationProviders:    append([]string(nil), imageGenProv[d.PluginID]...),
			RegisteredWebSearchProviders:          append([]string(nil), webSearchProv[d.PluginID]...),
			RegisteredHooks:                       append([]string(nil), hooksByPlugin[d.PluginID]...),
			RegisteredChannels:                    append([]string(nil), channelsByPlugin[d.PluginID]...),
			RegisteredInteractiveHandlers:         append([]string(nil), interactiveByPlugin[d.PluginID]...),
			ConversationBindingResolvedListeners:  append([]string(nil), convBindByPlugin[d.PluginID]...),
		})
	}
	return out
}
