package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type lobsterTool struct {
	workspace string
	restrict  bool
}

func NewLobsterTool(workspace string, restrictToWorkspace bool) Tool {
	return &lobsterTool{
		workspace: workspace,
		restrict:  restrictToWorkspace,
	}
}

func (t *lobsterTool) Name() string {
	return "lobster"
}

func (t *lobsterTool) Description() string {
	return "Run Lobster workflows with resumable approvals."
}

func (t *lobsterTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"run", "resume"},
				"description": "Operation mode: run pipeline or resume approval",
			},
			"pipeline": map[string]any{
				"type":        "string",
				"description": "Pipeline name/path for action=run",
			},
			"argsJson": map[string]any{
				"type":        "string",
				"description": "Optional JSON argument payload for action=run",
			},
			"token": map[string]any{
				"type":        "string",
				"description": "Resume token for action=resume",
			},
			"approve": map[string]any{
				"type":        "boolean",
				"description": "Approval decision for action=resume",
			},
			"cwd": map[string]any{
				"type":        "string",
				"description": "Relative working directory (optional). Must stay within workspace when restricted.",
			},
			"timeoutMs": map[string]any{
				"type":        "number",
				"description": "Subprocess timeout in milliseconds (default: 20000)",
			},
			"maxStdoutBytes": map[string]any{
				"type":        "number",
				"description": "Maximum stdout bytes to capture (default: 512000)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *lobsterTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	action, _ := args["action"].(string)
	action = strings.TrimSpace(action)
	if action == "" {
		return ErrorResult("action required")
	}

	cwd, err := t.resolveCwd(args["cwd"])
	if err != nil {
		return ErrorResult(err.Error()).WithError(err)
	}
	argv, err := buildLobsterArgv(action, args)
	if err != nil {
		return ErrorResult(err.Error()).WithError(err)
	}

	timeoutMs := readPositiveNumber(args["timeoutMs"], 20_000)
	maxStdoutBytes := readPositiveNumber(args["maxStdoutBytes"], 512_000)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "lobster", argv...)
	cmd.Dir = cwd

	output, err := cmd.CombinedOutput()
	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return ErrorResult("lobster subprocess timed out").WithError(runCtx.Err())
		}
		return ErrorResult(fmt.Sprintf("lobster failed: %v", err)).WithError(err)
	}

	if len(output) > maxStdoutBytes {
		return ErrorResult("lobster output exceeded maxStdoutBytes")
	}

	envelope, err := parseLobsterEnvelope(string(output))
	if err != nil {
		return ErrorResult(err.Error()).WithError(err)
	}

	pretty, _ := json.MarshalIndent(envelope, "", "  ")
	return SilentResult(string(pretty))
}

func (t *lobsterTool) resolveCwd(raw any) (string, error) {
	base := t.workspace
	if strings.TrimSpace(base) == "" {
		base = "."
	}
	base = filepath.Clean(base)

	cwdRaw, ok := raw.(string)
	if !ok || strings.TrimSpace(cwdRaw) == "" {
		return base, nil
	}
	cwdRaw = strings.TrimSpace(cwdRaw)
	if filepath.IsAbs(cwdRaw) {
		return "", fmt.Errorf("cwd must be a relative path")
	}
	resolved := filepath.Clean(filepath.Join(base, cwdRaw))
	if !t.restrict {
		return resolved, nil
	}
	rel, err := filepath.Rel(base, resolved)
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cwd must stay within workspace")
	}
	return resolved, nil
}

func buildLobsterArgv(action string, args map[string]any) ([]string, error) {
	switch action {
	case "run":
		pipeline, _ := args["pipeline"].(string)
		pipeline = strings.TrimSpace(pipeline)
		if pipeline == "" {
			return nil, fmt.Errorf("pipeline required")
		}
		argv := []string{"run", "--mode", "tool", pipeline}
		if argsJSON, ok := args["argsJson"].(string); ok && strings.TrimSpace(argsJSON) != "" {
			argv = append(argv, "--args-json", strings.TrimSpace(argsJSON))
		}
		return argv, nil
	case "resume":
		token, _ := args["token"].(string)
		token = strings.TrimSpace(token)
		if token == "" {
			return nil, fmt.Errorf("token required")
		}
		approve, ok := args["approve"].(bool)
		if !ok {
			return nil, fmt.Errorf("approve required")
		}
		approveValue := "no"
		if approve {
			approveValue = "yes"
		}
		return []string{"resume", "--token", token, "--approve", approveValue}, nil
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func parseLobsterEnvelope(stdout string) (map[string]any, error) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return nil, fmt.Errorf("lobster returned empty output")
	}

	tryParse := func(s string) (map[string]any, bool) {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(s), &parsed); err != nil {
			return nil, false
		}
		return parsed, true
	}

	if parsed, ok := tryParse(trimmed); ok {
		return parsed, nil
	}

	start := strings.LastIndex(trimmed, "{")
	if start >= 0 {
		if parsed, ok := tryParse(trimmed[start:]); ok {
			return parsed, nil
		}
	}
	return nil, fmt.Errorf("lobster returned invalid JSON")
}

func readPositiveNumber(raw any, defaultValue int) int {
	switch value := raw.(type) {
	case float64:
		if value > 0 {
			return int(value)
		}
	case int:
		if value > 0 {
			return value
		}
	}
	return defaultValue
}
