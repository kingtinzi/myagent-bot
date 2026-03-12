// Launcher channel: receives outbound messages from the agent and delivers them
// to waiting HTTP clients (e.g. Launcher chat window) by ChatID.

package channels

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/sipeed/pinchbot/pkg/bus"
)

// LauncherChannel is a virtual channel for the Launcher chat UI.
// HTTP POST /api/chat publishes an InboundMessage; the agent replies with
// OutboundMessage to channel "launcher"; Send() delivers the content to the waiter.
type LauncherChannel struct {
	running   atomic.Bool
	responses sync.Map // chatID string -> chan string
}

// NewLauncherChannel creates a launcher channel for the chat API.
func NewLauncherChannel() *LauncherChannel {
	return &LauncherChannel{}
}

// RegisterWaiter registers a response channel for the given chatID.
// When the agent sends an OutboundMessage with this ChatID, the content will be sent to ch.
func (c *LauncherChannel) RegisterWaiter(chatID string, ch chan string) {
	c.responses.Store(chatID, ch)
}

func (c *LauncherChannel) getAndDelete(chatID string) (chan string, bool) {
	v, ok := c.responses.LoadAndDelete(chatID)
	if !ok {
		return nil, false
	}
	ch, _ := v.(chan string)
	return ch, ch != nil
}

func (c *LauncherChannel) Name() string { return "launcher" }

func (c *LauncherChannel) Start(ctx context.Context) error {
	c.running.Store(true)
	return nil
}

func (c *LauncherChannel) Stop(ctx context.Context) error {
	c.running.Store(false)
	return nil
}

func (c *LauncherChannel) IsRunning() bool { return c.running.Load() }

func (c *LauncherChannel) IsAllowed(senderID string) bool { return true }

func (c *LauncherChannel) IsAllowedSender(sender bus.SenderInfo) bool { return true }

func (c *LauncherChannel) ReasoningChannelID() string { return "" }

func (c *LauncherChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.running.Load() {
		return ErrNotRunning
	}
	ch, ok := c.getAndDelete(msg.ChatID)
	if !ok {
		return nil // no waiter, ignore
	}
	select {
	case ch <- msg.Content:
	default:
		// waiter may have timed out; don't block
	}
	return nil
}
