// Package constants provides shared constants across the codebase.
package constants

// internalChannels defines channels that are used for internal communication
// and should not be exposed to external users or recorded as last active channel.
var internalChannels = map[string]struct{}{
	"cli":      {},
	"system":   {},
	"subagent": {},
}

// IsInternalChannel returns true if the channel is an internal channel.
func IsInternalChannel(channel string) bool {
	_, found := internalChannels[channel]
	return found
}

// execTrustedChannels lists channels that may run shell commands when
// tools.exec.allow_remote is false. It includes local/trusted UIs (e.g. Launcher)
// in addition to IsInternalChannel.
//
// Note: "launcher" is intentionally NOT in internalChannels — outbound messages
// to channel "launcher" must still be dispatched; IsInternalChannel is also used
// to skip outbound routing for purely internal channels.
var execTrustedChannels = map[string]struct{}{
	"cli":      {},
	"system":   {},
	"subagent": {},
	"launcher": {},
}

// IsExecTrustedChannel returns true if exec (and cron command scheduling) is
// allowed without tools.exec.allow_remote for this channel.
func IsExecTrustedChannel(channel string) bool {
	_, ok := execTrustedChannels[channel]
	return ok
}
