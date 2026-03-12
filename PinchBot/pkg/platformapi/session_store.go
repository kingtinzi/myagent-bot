package platformapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sipeed/pinchbot/pkg/fileutil"
)

const defaultSessionFilename = "platform-session.json"

type FileSessionStore struct {
	path string
}

func NewFileSessionStore(baseDir string) *FileSessionStore {
	return &FileSessionStore{
		path: filepath.Join(baseDir, defaultSessionFilename),
	}
}

func (s *FileSessionStore) Save(session Session) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(s.path, data, 0o600)
}

func (s *FileSessionStore) Load() (Session, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, fmt.Errorf("decode session: %w", err)
	}
	if session.AccessToken == "" || session.UserID == "" {
		return Session{}, fmt.Errorf("session file is incomplete")
	}
	return session, nil
}

func (s *FileSessionStore) Clear() error {
	err := os.Remove(s.path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
