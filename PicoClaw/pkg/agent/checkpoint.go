// Package agent: checkpoint for task audit and resume (critical threshold + human-in-the-loop).

package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/providers"
)

const (
	// CriticalThresholdPercent is the iteration percentage (e.g. 80) at which we force a checkpoint report.
	CriticalThresholdPercent = 80
)

// CheckpointReport is the structured audit report saved to checkpoint.json.
type CheckpointReport struct {
	// CompletedSteps: 已达成目标
	CompletedSteps []string `json:"completed_steps"`
	// CurrentState: 当前状态/环境 (variables, paths, modified files summary)
	CurrentState string `json:"current_state"`
	// PendingSteps: 待办事项
	PendingSteps []string `json:"pending_steps"`
	// Warnings: 预警信息 (why not done yet)
	Warnings []string `json:"warnings"`
	// SummaryForContext: single summary used as context after trim (assistant message body)
	SummaryForContext string `json:"summary_for_context"`
	// OriginalUserMessage: first user message of this run, for session trim
	OriginalUserMessage string `json:"original_user_message,omitempty"`
	// Iteration when checkpoint was taken
	Iteration int `json:"iteration"`
	MaxIter   int `json:"max_iter"`
}

// FormatReport returns a human-readable report string for the user.
func (r *CheckpointReport) FormatReport() string {
	var b strings.Builder
	b.WriteString("## 任务进度审计报告 (Checkpoint)\n\n")
	b.WriteString("### [已达成目标]\n")
	for _, s := range r.CompletedSteps {
		b.WriteString("- " + s + "\n")
	}
	if len(r.CompletedSteps) == 0 {
		b.WriteString("- （暂无）\n")
	}
	b.WriteString("\n### [当前状态/环境]\n")
	b.WriteString(r.CurrentState)
	if r.CurrentState == "" {
		b.WriteString("（无摘要）\n")
	} else if !strings.HasSuffix(r.CurrentState, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n### [待办事项]\n")
	for _, s := range r.PendingSteps {
		b.WriteString("- " + s + "\n")
	}
	if len(r.PendingSteps) == 0 {
		b.WriteString("- （无）\n")
	}
	b.WriteString("\n### [预警信息]\n")
	for _, s := range r.Warnings {
		b.WriteString("- " + s + "\n")
	}
	if len(r.Warnings) == 0 {
		b.WriteString("- （无）\n")
	}
	return b.String()
}

// CheckpointPrompt is the human-in-the-loop question appended after the report.
const CheckpointPrompt = "以上是当前进度，我计划接下来执行 [待办事项]。你是否同意？或有其他修改建议？请回复 **y** 或 **继续** 以继续执行，或直接给出修改意见。"

func checkpointPath(workspace, sessionKey string) string {
	safe := strings.ReplaceAll(sessionKey, ":", "_")
	if safe == "" || safe == "." {
		safe = "default"
	}
	dir := filepath.Join(workspace, "checkpoints")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, safe+".json")
}

// SaveCheckpoint writes the report to checkpoint.json under workspace/checkpoints/.
func SaveCheckpoint(workspace, sessionKey string, report *CheckpointReport) error {
	if workspace == "" || report == nil {
		return fmt.Errorf("workspace and report required")
	}
	path := checkpointPath(workspace, sessionKey)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// LoadCheckpoint reads checkpoint.json if it exists.
func LoadCheckpoint(workspace, sessionKey string) (*CheckpointReport, error) {
	path := checkpointPath(workspace, sessionKey)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var report CheckpointReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}

// RemoveCheckpoint deletes checkpoint.json when the task is fully completed.
func RemoveCheckpoint(workspace, sessionKey string) error {
	path := checkpointPath(workspace, sessionKey)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// HasCheckpoint returns true if a checkpoint file exists for this session.
func HasCheckpoint(workspace, sessionKey string) bool {
	path := checkpointPath(workspace, sessionKey)
	_, err := os.Stat(path)
	return err == nil
}

// BuildReportFromMessages builds a CheckpointReport from the current message slice (heuristic).
func BuildReportFromMessages(messages []providers.Message, iteration, maxIter int) *CheckpointReport {
	report := &CheckpointReport{
		Iteration: iteration,
		MaxIter:   maxIter,
		Warnings:  []string{},
	}

	// Original user message: first user content we see (skip system)
	for _, m := range messages {
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			report.OriginalUserMessage = m.Content
			break
		}
	}

	// Collect tool calls and last tool results to build completed steps and state
	var completedToolNames []string
	var lastToolResults []string
	for i := range messages {
		msg := &messages[i]
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				name := tc.Name
				if tc.Function != nil && tc.Function.Name != "" {
					name = tc.Function.Name
				}
				completedToolNames = append(completedToolNames, name)
			}
		}
		if msg.Role == "tool" {
			content := strings.TrimSpace(msg.Content)
			if len(content) > 500 {
				content = content[:500] + "..."
			}
			lastToolResults = append(lastToolResults, content)
		}
	}

	// Dedupe and take last N tool names as "completed steps"
	seen := make(map[string]bool)
	for _, name := range completedToolNames {
		if !seen[name] {
			seen[name] = true
			report.CompletedSteps = append(report.CompletedSteps, "调用工具: "+name)
		}
	}
	// Keep last 5 as "recent" for completed steps if we have many
	if len(report.CompletedSteps) > 8 {
		report.CompletedSteps = report.CompletedSteps[len(report.CompletedSteps)-8:]
	}

	// Current state: last few tool results
	if len(lastToolResults) > 0 {
		take := len(lastToolResults)
		if take > 3 {
			take = 3
		}
		report.CurrentState = strings.Join(lastToolResults[len(lastToolResults)-take:], "\n\n")
	}

	// Pending: generic placeholder (LLM could refine on resume)
	report.PendingSteps = append(report.PendingSteps, "继续执行剩余步骤直至任务完成")

	// Warnings
	report.Warnings = append(report.Warnings,
		fmt.Sprintf("已达到本轮工具调用上限的 %d%%（%d/%d 次），为避免触顶中断已自动保存进度。",
			CriticalThresholdPercent, iteration, maxIter))
	if iteration >= maxIter {
		report.Warnings = append(report.Warnings, "可能原因：任务步骤较多、网络较慢或单轮迭代过多。建议回复「继续」后由我接着执行。")
	}

	// Summary for context: compact text used as the single assistant message after trim
	var sb strings.Builder
	sb.WriteString("[Checkpoint 摘要]\n")
	sb.WriteString("- 已执行步骤: ")
	for i, s := range report.CompletedSteps {
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(s)
	}
	sb.WriteString("\n- 当前状态摘要: ")
	stateSummary := strings.ReplaceAll(report.CurrentState, "\n", " ")
	if len(stateSummary) > 300 {
		sb.WriteString(stateSummary[:300])
		sb.WriteString("...")
	} else {
		sb.WriteString(stateSummary)
	}
	sb.WriteString("\n- 待办: ")
	sb.WriteString(strings.Join(report.PendingSteps, "; "))
	report.SummaryForContext = sb.String()

	return report
}
