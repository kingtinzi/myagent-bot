// Package usage provides token usage logging and a simple dashboard.
package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MaxStoredPromptLen is the max length for stored prompt/completion text in usage.jsonl.
const MaxStoredPromptLen = 32 * 1024

// Record is one LLM call record (one line in usage.jsonl).
type Record struct {
	Time             time.Time `json:"time"`
	SessionKey       string    `json:"session_key"`
	Channel          string    `json:"channel"`
	Source           string    `json:"source"` // "heartbeat", "dingtalk", "agent", etc.
	Model            string    `json:"model"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	Iteration        int       `json:"iteration"`
	// Prompt is the serialized input messages to the LLM (optional, may be truncated).
	Prompt string `json:"prompt,omitempty"`
	// Completion is the LLM output text (optional, may be truncated).
	Completion string `json:"completion,omitempty"`
}

// Logger appends usage records to workspace/usage.jsonl.
type Logger struct {
	mu          sync.Mutex
	workspace   string
	usagePath   string
	file        *os.File
	openErr     error
}

// NewLogger creates a usage logger that writes to workspace/usage.jsonl.
func NewLogger(workspace string) *Logger {
	usagePath := filepath.Join(workspace, "usage.jsonl")
	return &Logger{workspace: workspace, usagePath: usagePath}
}

// Record appends one usage record. Safe to call from multiple goroutines.
func (l *Logger) Record(r Record) {
	if r.Time.IsZero() {
		r.Time = time.Now()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.openErr != nil {
		return
	}
	if l.file == nil {
		if err := os.MkdirAll(l.workspace, 0o755); err != nil {
			l.openErr = err
			return
		}
		l.file, l.openErr = os.OpenFile(l.usagePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if l.openErr != nil {
			return
		}
	}
	enc := json.NewEncoder(l.file)
	enc.Encode(r)
}

// Close closes the log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}
