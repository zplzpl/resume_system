package resume

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type StoredFile struct {
	StoragePath string
	ContentType string
	Hash        string
	Size        int64
}

type LocalStorage struct {
	baseDir string
}

func NewLocalStorage(baseDir string) (*LocalStorage, error) {
	if baseDir == "" {
		baseDir = "./data/resumes"
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	return &LocalStorage{baseDir: baseDir}, nil
}

func (s *LocalStorage) Save(fileName string, r io.Reader) (StoredFile, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	safeName := sanitizeBaseName(strings.TrimSuffix(fileName, filepath.Ext(fileName)))
	if safeName == "" {
		safeName = "resume"
	}
	storedFileName := fmt.Sprintf("%d_%s%s", time.Now().UTC().UnixNano(), safeName, ext)
	storagePath := filepath.Join(s.baseDir, storedFileName)

	f, err := os.Create(storagePath)
	if err != nil {
		return StoredFile{}, fmt.Errorf("create storage file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), r)
	if err != nil {
		return StoredFile{}, fmt.Errorf("write storage file: %w", err)
	}

	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return StoredFile{
		StoragePath: storagePath,
		ContentType: contentType,
		Hash:        hex.EncodeToString(h.Sum(nil)),
		Size:        n,
	}, nil
}

func (s *LocalStorage) Delete(storagePath string) error {
	storagePath = strings.TrimSpace(storagePath)
	if storagePath == "" {
		return nil
	}
	if err := os.Remove(storagePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete storage file: %w", err)
	}
	return nil
}

var unsafeFileChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeBaseName(s string) string {
	s = strings.TrimSpace(s)
	s = unsafeFileChars.ReplaceAllString(s, "_")
	s = strings.Trim(s, "._-")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}
