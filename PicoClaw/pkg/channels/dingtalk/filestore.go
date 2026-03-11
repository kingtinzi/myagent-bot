// Package dingtalk: file record store for lazy-load uploaded files (PDF, Excel, etc.)

package dingtalk

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// UploadedFileRecord holds metadata for a file uploaded in a chat (record-only or cached path).
type UploadedFileRecord struct {
	FileID       string
	ChatID       string
	DownloadCode string
	Filename     string
	ContentType  string
	CachedPath   string    // filled after first download
	CreatedAt    time.Time
}

// UploadedFileStore stores file records per chat for lazy download and read.
type UploadedFileStore interface {
	Add(chatID, downloadCode, filename, contentType string) (fileID string)
	List(chatID string) []UploadedFileRecord
	Get(chatID, fileIDOrName string) (*UploadedFileRecord, bool)
	SetCachedPath(chatID, fileID, path string)
}

type fileStore struct {
	mu     sync.RWMutex
	byID   map[string]*UploadedFileRecord // fileID -> record
	byChat map[string][]string           // chatID -> fileIDs (order)
}

// NewUploadedFileStore creates an in-memory store for uploaded file records.
func NewUploadedFileStore() UploadedFileStore {
	return &fileStore{
		byID:   make(map[string]*UploadedFileRecord),
		byChat: make(map[string][]string),
	}
}

func (s *fileStore) Add(chatID, downloadCode, filename, contentType string) (fileID string) {
	fileID = "file-" + uuid.New().String()[:8]
	r := &UploadedFileRecord{
		FileID:       fileID,
		ChatID:       chatID,
		DownloadCode: downloadCode,
		Filename:     filename,
		ContentType:  contentType,
		CreatedAt:    time.Now(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[fileID] = r
	s.byChat[chatID] = append(s.byChat[chatID], fileID)
	return fileID
}

func (s *fileStore) List(chatID string) []UploadedFileRecord {
	s.mu.RLock()
	ids := s.byChat[chatID]
	s.mu.RUnlock()
	if len(ids) == 0 {
		return nil
	}
	out := make([]UploadedFileRecord, 0, len(ids))
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, id := range ids {
		if r, ok := s.byID[id]; ok {
			out = append(out, *r)
		}
	}
	return out
}

func (s *fileStore) Get(chatID, fileIDOrName string) (*UploadedFileRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r, ok := s.byID[fileIDOrName]; ok && r.ChatID == chatID {
		return r, true
	}
	for _, id := range s.byChat[chatID] {
		if r, ok := s.byID[id]; ok && (r.Filename == fileIDOrName || r.FileID == fileIDOrName) {
			return r, true
		}
	}
	return nil, false
}

func (s *fileStore) SetCachedPath(chatID, fileID, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.byID[fileID]; ok && r.ChatID == chatID {
		r.CachedPath = path
	}
}
