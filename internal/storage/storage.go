package storage

import (
	"context"
	"io"
)

type Object struct {
	Body        io.ReadCloser
	ContentType string
}

type Storage interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string) error
	Get(ctx context.Context, key string) (*Object, error)
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
}
