package tools

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripJSONCodeFences(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{`{"a":1}`, `{"a":1}`},
		{"```\n{\"a\":1}\n```", `{"a":1}`},
		{"```json\n{\"a\":1}\n```", `{"a":1}`},
		{"```oops\nnot-json", "```oops\nnot-json"},
	}
	for _, tc := range cases {
		got := stripJSONCodeFences(tc.in)
		require.Equal(t, tc.want, got)
	}
}

func TestValidateJSONSchema_Basic(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x": map[string]any{"type": "number"},
		},
		"required": []any{"x"},
	}
	err := validateJSONSchema(schema, map[string]any{"x": 1.0})
	require.NoError(t, err)
	err = validateJSONSchema(schema, map[string]any{"x": "nope"})
	require.Error(t, err)
}

func TestAllowedContains(t *testing.T) {
	t.Parallel()
	require.True(t, allowedContains([]string{"openai/gpt-4o", "anthropic/claude"}, "OpenAI/GPT-4o"))
	require.False(t, allowedContains([]string{"openai/gpt-4o"}, "openai/gpt-5"))
}
