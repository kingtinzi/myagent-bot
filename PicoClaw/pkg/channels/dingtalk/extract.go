// Package dingtalk: extract plain text from PDF and Excel for read_uploaded_file tool.

package dingtalk

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/xuri/excelize/v2"
)

// extractTextFromFile returns plain text from a local file (PDF, xlsx, xls). Others return an error.
func extractTextFromFile(localPath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(localPath))
	switch ext {
	case ".pdf":
		return extractPDF(localPath)
	case ".xlsx", ".xls":
		return extractExcel(localPath)
	default:
		return "", fmt.Errorf("unsupported file type for text extraction: %s (supported: .pdf, .xlsx, .xls)", ext)
	}
}

func extractPDF(localPath string) (string, error) {
	f, r, err := pdf.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()
	reader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("pdf get text: %w", err)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("pdf read text: %w", err)
	}
	text := string(raw)
	text = strings.TrimSpace(text)
	if len(text) > 500000 {
		text = text[:500000] + "\n...[truncated]"
	}
	return text, nil
}

func extractExcel(localPath string) (string, error) {
	xl, err := excelize.OpenFile(localPath)
	if err != nil {
		return "", fmt.Errorf("open excel: %w", err)
	}
	defer xl.Close()
	var b strings.Builder
	for _, name := range xl.GetSheetList() {
		rows, err := xl.GetRows(name)
		if err != nil {
			logger.WarnCF("dingtalk", "excel sheet read skip", map[string]any{"sheet": name, "error": err.Error()})
			continue
		}
		b.WriteString(fmt.Sprintf("--- Sheet: %s ---\n", name))
		for _, row := range rows {
			b.WriteString(strings.Join(row, "\t"))
			b.WriteString("\n")
		}
	}
	text := strings.TrimSpace(b.String())
	if len(text) > 500000 {
		text = text[:500000] + "\n...[truncated]"
	}
	return text, nil
}
