//go:build !windows

package models

import (
	"context"
	"fmt"
	"strings"

	"github.com/gomlx/go-huggingface/hub"
)

// defaultCacheDir calls hub.DefaultCacheDir.
func defaultCacheDir() string {
	return hub.DefaultCacheDir()
}

// PullModel pulls a model to local storage. Any error it returns is suitable
// for writing back to the client.
func (m *Manager) PullModel(ctx context.Context, model string) error {
	// Restrict model pull concurrency.
	// TODO: We may want something more sophisticated here, but it will be
	// clearer once we've switched to Docker Hub hosting.
	select {
	case <-m.pullTokens:
	case <-ctx.Done():
		return context.Canceled
	}
	defer func() {
		m.pullTokens <- struct{}{}
	}()

	// Query the model on Hugging Face.
	// TODO: Replace github.com/gomlx/go-huggingface/hub or capture stdout
	// because it doesn't accept a logger.
	// TODO: Use the systemproxy HTTP client (m.httpClient).
	repo := hub.New(model).WithCacheDir(m.cacheDir)
	var ggufFiles []string
	for fileName, err := range repo.IterFileNames() {
		if err != nil {
			return fmt.Errorf("error while enumerating remote files: %w", err)
		}
		if strings.HasSuffix(fileName, ".gguf") {
			ggufFiles = append(ggufFiles, fileName)
		}
	}

	// Download the relevant GGUF files from Hugging Face.
	m.log.Infoln("Downloading files:", ggufFiles)
	// TODO: Use the provided context to regulate the download operation.
	// TODO: Use the systemproxy HTTP client (m.httpClient).
	if _, err := repo.DownloadFiles(ggufFiles...); err != nil {
		return fmt.Errorf("error while downloading file(s): %w", err)
	}
	return nil
}
