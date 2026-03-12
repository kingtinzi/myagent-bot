// read_uploaded_file: tool to list and read content of files uploaded in a chat (e.g. DingTalk PDF/Excel).

package tools

import (
	"context"
	"fmt"
	"strings"
)

// UploadedFileInfo describes one uploaded file in a chat.
type UploadedFileInfo struct {
	ID   string
	Name string
}

// UploadedFileReader is implemented by channels (e.g. DingTalk) that support lazy-read uploaded files.
type UploadedFileReader interface {
	List(chatID string) []UploadedFileInfo
	ReadContent(chatID, fileIDOrName string) (text string, err error)
}

// ReadUploadedFileTool lets the agent list and read files uploaded in the current chat (e.g. PDF, Excel).
type ReadUploadedFileTool struct {
	reader UploadedFileReader
}

// NewReadUploadedFileTool creates a tool that uses the given reader. If reader is nil, the tool returns an error.
func NewReadUploadedFileTool(reader UploadedFileReader) *ReadUploadedFileTool {
	return &ReadUploadedFileTool{reader: reader}
}

func (t *ReadUploadedFileTool) Name() string {
	return "read_uploaded_file"
}

func (t *ReadUploadedFileTool) Description() string {
	return "List or read the content of files that the user uploaded in this chat (e.g. PDF, Excel). " +
		"Use list to see available files; use read with chat_id and file name or ID to get extracted text."
}

func (t *ReadUploadedFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"chat_id": map[string]any{
				"type":        "string",
				"description": "Chat ID (conversation ID) where the file was uploaded",
			},
			"action": map[string]any{
				"type":        "string",
				"description": "One of: list (list uploaded files), read (read file content as text)",
				"enum":        []string{"list", "read"},
			},
			"file_id_or_name": map[string]any{
				"type":        "string",
				"description": "For action=read: file ID or file name to read (e.g. report.pdf)",
			},
		},
		"required": []string{"chat_id", "action"},
	}
}

func (t *ReadUploadedFileTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.reader == nil {
		return ErrorResult("read_uploaded_file: no upload file reader configured (e.g. DingTalk channel)")
	}
	chatID, _ := args["chat_id"].(string)
	if chatID == "" {
		return ErrorResult("chat_id is required")
	}
	action, _ := args["action"].(string)
	action = strings.TrimSpace(strings.ToLower(action))
	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		infos := t.reader.List(chatID)
		if len(infos) == 0 {
			return NewToolResult("No uploaded files in this chat.")
		}
		var b strings.Builder
		b.WriteString("Uploaded files in this chat:\n")
		for _, f := range infos {
			b.WriteString(fmt.Sprintf("  - %s (id: %s)\n", f.Name, f.ID))
		}
		b.WriteString("Use action=read and file_id_or_name to read a file's text content.")
		return NewToolResult(b.String())
	case "read":
		fileIDOrName, _ := args["file_id_or_name"].(string)
		if fileIDOrName == "" {
			return ErrorResult("file_id_or_name is required for action=read")
		}
		text, err := t.reader.ReadContent(chatID, fileIDOrName)
		if err != nil {
			return ErrorResult(fmt.Sprintf("read file: %v", err))
		}
		return NewToolResult(text)
	default:
		return ErrorResult(fmt.Sprintf("action must be list or read, got %q", action))
	}
}
