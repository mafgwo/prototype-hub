package prototype

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"prototypehub/internal/config"
	"prototypehub/internal/storage"
)

var blockedExtensions = []string{
	".exe", ".bat", ".cmd", ".com", ".dll", ".msi", ".sh", ".ps1", ".php", ".asp", ".aspx", ".jsp",
}

type Result struct {
	EntryFile string
	HTMLFiles []string
	ZipKey    string
	PublicKey string
}

func ProcessZip(ctx context.Context, cfg config.Config, store storage.Storage, zipData []byte, zipKey, publicPrefix string) (*Result, error) {
	if int64(len(zipData)) > cfg.UploadMaxSizeBytes {
		return nil, fmt.Errorf("zip file exceeds max size")
	}
	if err := store.Put(ctx, zipKey, bytes.NewReader(zipData), "application/zip"); err != nil {
		return nil, fmt.Errorf("store zip: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	if len(reader.File) == 0 {
		return nil, errors.New("zip archive is empty")
	}
	if len(reader.File) > cfg.UploadMaxFileCount {
		return nil, fmt.Errorf("zip contains too many files: %d > %d", len(reader.File), cfg.UploadMaxFileCount)
	}

	commonRoot := detectCommonRoot(reader.File)
	totalSize := int64(0)
	names := make([]string, 0, len(reader.File))

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		safeName, err := sanitizeZipPath(file.Name)
		if err != nil {
			return nil, err
		}
		if commonRoot != "" {
			safeName = strings.TrimPrefix(safeName, commonRoot+"/")
		}
		if safeName == "" {
			continue
		}
		if shouldSkipFile(safeName) {
			continue
		}
		if isBlockedFile(safeName) {
			return nil, fmt.Errorf("blocked file type: %s", safeName)
		}
		totalSize += int64(file.UncompressedSize64)
		if totalSize > cfg.UploadMaxExtractedSize {
			return nil, fmt.Errorf("archive expands beyond limit")
		}
		names = append(names, safeName)
	}

	entryFile, err := detectEntryFile(names)
	if err != nil {
		return nil, err
	}

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		safeName, err := sanitizeZipPath(file.Name)
		if err != nil {
			return nil, err
		}
		if commonRoot != "" {
			safeName = strings.TrimPrefix(safeName, commonRoot+"/")
		}
		if safeName == "" {
			continue
		}
		if shouldSkipFile(safeName) {
			continue
		}
		source, err := file.Open()
		if err != nil {
			return nil, err
		}
		contentType := mime.TypeByExtension(filepath.Ext(safeName))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		objectKey := path.Join(publicPrefix, safeName)
		if err := store.Put(ctx, objectKey, source, contentType); err != nil {
			source.Close()
			return nil, fmt.Errorf("store extracted file %s: %w", safeName, err)
		}
		source.Close()
	}

	return &Result{
		EntryFile: entryFile,
		HTMLFiles: collectHTMLFiles(names),
		ZipKey:    zipKey,
		PublicKey: publicPrefix,
	}, nil
}

func sanitizeZipPath(name string) (string, error) {
	clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	switch {
	case clean == ".":
		return "", nil
	case strings.HasPrefix(clean, "/"):
		return "", fmt.Errorf("absolute path is not allowed: %s", name)
	case strings.HasPrefix(clean, "../"), strings.Contains(clean, "/../"):
		return "", fmt.Errorf("invalid archive path: %s", name)
	}
	return clean, nil
}

func detectCommonRoot(files []*zip.File) string {
	root := ""
	for _, file := range files {
		if file.FileInfo().IsDir() {
			continue
		}
		name, err := sanitizeZipPath(file.Name)
		if err != nil || name == "" {
			continue
		}
		parts := strings.Split(name, "/")
		if len(parts) < 2 {
			return ""
		}
		if root == "" {
			root = parts[0]
			continue
		}
		if parts[0] != root {
			return ""
		}
	}
	return root
}

func detectEntryFile(names []string) (string, error) {
	if len(names) == 0 {
		return "", errors.New("zip archive does not contain usable files")
	}
	for _, preferred := range []string{"index.html", "首页.html"} {
		for _, name := range names {
			if strings.EqualFold(name, preferred) {
				return name, nil
			}
		}
	}

	htmlFiles := collectHTMLFiles(names)
	if len(htmlFiles) == 0 {
		return "", errors.New("no html entry file found in archive")
	}
	sort.Strings(htmlFiles)
	return htmlFiles[0], nil
}

func collectHTMLFiles(names []string) []string {
	htmlFiles := make([]string, 0)
	for _, name := range names {
		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm") {
			htmlFiles = append(htmlFiles, name)
		}
	}
	sort.Strings(htmlFiles)
	return htmlFiles
}

func isBlockedFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, blocked := range blockedExtensions {
		if ext == blocked {
			return true
		}
	}
	return false
}

func shouldSkipFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "__macosx/") || strings.HasSuffix(lower, "/.ds_store") || lower == ".ds_store"
}
