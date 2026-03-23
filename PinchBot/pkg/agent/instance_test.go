package agent

import "testing"

func TestAgentInstance_StopPluginHost_NilReceiver(t *testing.T) {
	var a *AgentInstance
	a.StopPluginHost()
}

func TestAgentInstance_StopPluginHost_IdempotentWithoutHost(t *testing.T) {
	a := &AgentInstance{}
	a.StopPluginHost()
	a.StopPluginHost()
}
