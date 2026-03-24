package protocoltypes

import (
	"encoding/json"
	"strings"
)

type ToolCall struct {
	ID               string         `json:"id"`
	Type             string         `json:"type,omitempty"`
	Function         *FunctionCall  `json:"function,omitempty"`
	Name             string         `json:"-"`
	Arguments        map[string]any `json:"-"`
	ThoughtSignature string         `json:"-"` // Internal use only
	ExtraContent     *ExtraContent  `json:"extra_content,omitempty"`
}

type ExtraContent struct {
	Google *GoogleExtra `json:"google,omitempty"`
}

type GoogleExtra struct {
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

type FunctionCall struct {
	Name             string `json:"name"`
	Arguments        string `json:"arguments"`
	ThoughtSignature string `json:"thought_signature,omitempty"`
}

// UnmarshalJSON accepts both OpenAI-style function tool calls:
//   {"function":{"name":"x","arguments":"{...}"}}
// and legacy/top-level shape:
//   {"name":"x","arguments":"{...}"}
// Some upstreams may emit one or the other (or mixed). We normalize into
// Function/Name/Arguments so downstream tool loop can execute reliably.
func (tc *ToolCall) UnmarshalJSON(data []byte) error {
	type rawToolCall struct {
		ID               string        `json:"id"`
		Type             string        `json:"type,omitempty"`
		Function         *FunctionCall `json:"function,omitempty"`
		Name             string        `json:"name,omitempty"`
		Arguments        string        `json:"arguments,omitempty"`
		ThoughtSignature string        `json:"thought_signature,omitempty"`
		ExtraContent     *ExtraContent `json:"extra_content,omitempty"`
	}
	var raw rawToolCall
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	tc.ID = raw.ID
	tc.Type = raw.Type
	tc.Function = raw.Function
	tc.ThoughtSignature = raw.ThoughtSignature
	tc.ExtraContent = raw.ExtraContent
	tc.Name = ""
	tc.Arguments = nil

	if tc.Function != nil {
		tc.Name = strings.TrimSpace(tc.Function.Name)
		if raw.ThoughtSignature == "" {
			tc.ThoughtSignature = tc.Function.ThoughtSignature
		}
		if strings.TrimSpace(tc.Function.Arguments) != "" {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &parsed); err == nil && parsed != nil {
				tc.Arguments = parsed
			}
		}
	}

	// Fallback for upstreams that place function name/args at top-level.
	if tc.Name == "" && strings.TrimSpace(raw.Name) != "" {
		tc.Name = strings.TrimSpace(raw.Name)
	}
	if len(tc.Arguments) == 0 && strings.TrimSpace(raw.Arguments) != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(raw.Arguments), &parsed); err == nil && parsed != nil {
			tc.Arguments = parsed
		}
		if tc.Function == nil {
			tc.Function = &FunctionCall{Name: tc.Name, Arguments: raw.Arguments}
		} else if strings.TrimSpace(tc.Function.Arguments) == "" {
			tc.Function.Arguments = raw.Arguments
		}
	}
	if tc.Function != nil && strings.TrimSpace(tc.Function.Name) == "" && tc.Name != "" {
		tc.Function.Name = tc.Name
	}

	return nil
}

type LLMResponse struct {
	Content          string            `json:"content"`
	ReasoningContent string            `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall        `json:"tool_calls,omitempty"`
	FinishReason     string            `json:"finish_reason"`
	Usage            *UsageInfo        `json:"usage,omitempty"`
	Reasoning        string            `json:"reasoning"`
	ReasoningDetails []ReasoningDetail `json:"reasoning_details"`
}

type ReasoningDetail struct {
	Format string `json:"format"`
	Index  int    `json:"index"`
	Type   string `json:"type"`
	Text   string `json:"text"`
}

type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CacheControl marks a content block for LLM-side prefix caching.
// Currently only "ephemeral" is supported (used by Anthropic).
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// ContentBlock represents a structured segment of a system message.
// Adapters that understand SystemParts can use these blocks to set
// per-block cache control (e.g. Anthropic's cache_control: ephemeral).
type ContentBlock struct {
	Type         string        `json:"type"` // "text"
	Text         string        `json:"text"`
	CacheControl *CacheControl `json:"cache_control,omitempty"`
}

type Message struct {
	Role             string         `json:"role"`
	Content          string         `json:"content"`
	Media            []string       `json:"media,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	SystemParts      []ContentBlock `json:"system_parts,omitempty"` // structured system blocks for cache-aware adapters
	ToolCalls        []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
}

type ToolDefinition struct {
	Type     string                 `json:"type"`
	Function ToolFunctionDefinition `json:"function"`
}

type ToolFunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}
