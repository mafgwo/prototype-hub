package storage

import (
	"context"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

type LocalStorage struct {
	root string
}

func NewLocal(root string) (*LocalStorage, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &LocalStorage{root: root}, nil
}

func (s *LocalStorage) Put(_ context.Context, key string, body io.Reader, _ string) error {
	fullPath := filepath.Join(s.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, body)
	return err
}

func (s *LocalStorage) Get(_ context.Context, key string) (*Object, error) {
	fullPath := filepath.Join(s.root, filepath.FromSlash(key))
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	contentType := mime.TypeByExtension(filepath.Ext(fullPath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return &Object{Body: file, ContentType: contentType}, nil
}

func (s *LocalStorage) Delete(_ context.Context, key string) error {
	key = strings.Trim(key, "/")
	if key == "" || key == "." {
		return nil
	}
	fullPath := filepath.Join(s.root, filepath.FromSlash(key))
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalStorage) DeletePrefix(_ context.Context, prefix string) error {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" || prefix == "." {
		return nil
	}
	fullPath := filepath.Join(s.root, filepath.FromSlash(prefix))
	return os.RemoveAll(fullPath)
}
