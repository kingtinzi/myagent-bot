// Package dingtalk: implements tools.UploadedFileReader for read_uploaded_file tool.

package dingtalk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sipeed/pinchbot/pkg/logger"
	"github.com/sipeed/pinchbot/pkg/tools"
)

// uploadedFileReader implements tools.UploadedFileReader using the channel's fileStore and downloader.
type uploadedFileReader struct {
	store    UploadedFileStore
	downloader *DingTalkDownloader
	cacheDir string
}

// List returns uploaded file infos for the chat.
func (r *uploadedFileReader) List(chatID string) []tools.UploadedFileInfo {
	recs := r.store.List(chatID)
	out := make([]tools.UploadedFileInfo, 0, len(recs))
	for _, rec := range recs {
		out = append(out, tools.UploadedFileInfo{ID: rec.FileID, Name: rec.Filename})
	}
	return out
}

// ReadContent downloads the file if needed, then extracts text (PDF/Excel). Returns error for unsupported types.
func (r *uploadedFileReader) ReadContent(chatID, fileIDOrName string) (string, error) {
	rec, ok := r.store.Get(chatID, fileIDOrName)
	if !ok {
		return "", fmt.Errorf("file not found for chat %q: %q", chatID, fileIDOrName)
	}
	localPath := rec.CachedPath
	if localPath == "" || notExists(localPath) {
		cacheDir := filepath.Join(r.cacheDir, chatID)
		if err := os.MkdirAll(cacheDir, 0o700); err != nil {
			return "", fmt.Errorf("create cache dir: %w", err)
		}
		ctx := context.Background()
		path, err := r.downloader.DownloadToTemp(ctx, rec.DownloadCode, cacheDir, rec.Filename)
		if err != nil {
			return "", err
		}
		localPath = path
		r.store.SetCachedPath(chatID, rec.FileID, path)
	}
	text, err := extractTextFromFile(localPath)
	if err != nil {
		logger.WarnCF("dingtalk", "extract text failed", map[string]any{"path": localPath, "error": err.Error()})
		return "", err
	}
	return text, nil
}

func notExists(p string) bool {
	_, err := os.Stat(p)
	return os.IsNotExist(err)
}

// GetUploadedFileReader returns a reader for the read_uploaded_file tool. Safe to call when fileStore is nil (returns nil).
func (c *DingTalkChannel) GetUploadedFileReader() tools.UploadedFileReader {
	if c.fileStore == nil || c.downloader == nil {
		return nil
	}
	return &uploadedFileReader{
		store:      c.fileStore,
		downloader: c.downloader,
		cacheDir:   c.fileCacheDir,
	}
}
