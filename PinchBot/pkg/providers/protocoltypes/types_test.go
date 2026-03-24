package protocoltypes

import (
	"encoding/json"
	"testing"
)

func TestToolCallUnmarshal_FunctionShape(t *testing.T) {
	raw := `{
		"id":"c1",
		"type":"function",
		"function":{"name":"lobster","arguments":"{\"action\":\"run\",\"pipeline\":\"commands.list\"}"}
	}`
	var tc ToolCall
	if err := json.Unmarshal([]byte(raw), &tc); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if tc.Name != "lobster" {
		t.Fatalf("Name = %q, want %q", tc.Name, "lobster")
	}
	if tc.Function == nil || tc.Function.Name != "lobster" {
		t.Fatalf("Function.Name mismatch: %#v", tc.Function)
	}
	if got := tc.Arguments["action"]; got != "run" {
		t.Fatalf("Arguments[action] = %#v, want %q", got, "run")
	}
}

func TestToolCallUnmarshal_TopLevelShape(t *testing.T) {
	raw := `{
		"id":"c2",
		"type":"function",
		"name":"lobster",
		"arguments":"{\"action\":\"run\",\"pipeline\":\"commands.list\"}"
	}`
	var tc ToolCall
	if err := json.Unmarshal([]byte(raw), &tc); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if tc.Name != "lobster" {
		t.Fatalf("Name = %q, want %q", tc.Name, "lobster")
	}
	if tc.Function == nil || tc.Function.Name != "lobster" {
		t.Fatalf("Function.Name mismatch: %#v", tc.Function)
	}
	if got := tc.Arguments["pipeline"]; got != "commands.list" {
		t.Fatalf("Arguments[pipeline] = %#v, want %q", got, "commands.list")
	}
}

