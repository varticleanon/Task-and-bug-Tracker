package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Storage interface {
	Save(key string, data []byte) error
	Open(key string) (io.ReadCloser, error)
	Delete(key string) error
	GetPath(key string) string
}

type LocalStorage struct {
	baseDir string
}

func NewLocalStorage(baseDir string) (*LocalStorage, error) {
	if baseDir == "" {
		baseDir = "./uploads"
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory %s: %w", baseDir, err)
	}
	return &LocalStorage{baseDir: baseDir}, nil
}

func (s *LocalStorage) GetPath(key string) string {
	return filepath.Join(s.baseDir, key)
}

func (s *LocalStorage) Save(key string, data []byte) error {
	path := s.GetPath(key)
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to atomic move file: %w", err)
	}
	return nil
}

func (s *LocalStorage) Open(key string) (io.ReadCloser, error) {
	path := s.GetPath(key)
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (s *LocalStorage) Delete(key string) error {
	path := s.GetPath(key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
