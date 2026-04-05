package prototype

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"prototypehub/internal/config"
	"prototypehub/internal/storage"
)

func TestDetectEntryFilePrefersIndexThenHome(t *testing.T) {
	entry, err := detectEntryFile([]string{"about.html", "首页.html", "z.html"})
	if err != nil {
		t.Fatalf("detect entry file: %v", err)
	}
	if entry != "首页.html" {
		t.Fatalf("expected 首页.html, got %s", entry)
	}

	entry, err = detectEntryFile([]string{"首页.html", "index.html", "about.html"})
	if err != nil {
		t.Fatalf("detect entry file: %v", err)
	}
	if entry != "index.html" {
		t.Fatalf("expected index.html, got %s", entry)
	}
}

func TestProcessZip_WithSingleFolderIndex(t *testing.T) {
	result := processFixture(t, filepath.Join("..", "..", "测试2.zip"))
	if result.EntryFile != "index.html" {
		t.Fatalf("expected entry file index.html, got %s", result.EntryFile)
	}
}

func TestProcessZip_WithMultiPagePrototype(t *testing.T) {
	result := processFixture(t, filepath.Join("..", "..", "测试项目.zip"))
	if result.EntryFile == "" {
		t.Fatal("expected an entry html file")
	}
	if len(result.HTMLFiles) == 0 {
		t.Fatal("expected html files to be collected")
	}
}

func processFixture(t *testing.T, zipPath string) *Result {
	t.Helper()

	data, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	root := t.TempDir()
	store, err := storage.NewLocal(root)
	if err != nil {
		t.Fatalf("create local storage: %v", err)
	}

	cfg := config.Config{
		UploadMaxSizeBytes:     100 << 20,
		UploadMaxFileCount:     100000,
		UploadMaxExtractedSize: 500 << 20,
	}

	result, err := ProcessZip(context.Background(), cfg, store, data, "fixtures/upload.zip", "fixtures/public")
	if err != nil {
		t.Fatalf("process zip: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "fixtures", "public", filepath.FromSlash(result.EntryFile))); err != nil {
		t.Fatalf("entry file not stored: %v", err)
	}

	return result
}
