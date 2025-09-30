package safetensors

import (
	"testing"

	"github.com/docker/model-runner/pkg/distribution/types"
)

func TestNewModel(t *testing.T) {
	// Create a test safetensors model
	// Note: In a real test, you would use actual safetensors files
	// For now, we'll test with dummy paths to verify the structure

	t.Run("single file", func(t *testing.T) {
		paths := []string{"test-model.safetensors"}
		model, err := NewModel(paths)
		if err == nil {
			t.Error("Expected error for non-existent file, got nil")
		}
		// The error is expected since the file doesn't exist
		// In a real test, we'd use test fixtures
		_ = model
	})

	t.Run("empty paths", func(t *testing.T) {
		var paths []string
		_, err := NewModel(paths)
		if err == nil {
			t.Error("Expected error for empty paths, got nil")
		}
	})

	t.Run("config extraction", func(t *testing.T) {
		config := configFromFiles([]string{"llama-7b-model.safetensors"})
		if config.Format != types.FormatSafetensors {
			t.Errorf("Expected format %s, got %s", types.FormatSafetensors, config.Format)
		}
		if config.Architecture != "llama" {
			t.Errorf("Expected architecture 'llama', got %s", config.Architecture)
		}
		if config.Safetensors["total_files"] != "1" {
			t.Errorf("Expected total_files '1', got %s", config.Safetensors["total_files"])
		}
	})

	t.Run("architecture detection", func(t *testing.T) {
		tests := []struct {
			filename string
			expected string
		}{
			{"mistral-7b-instruct.safetensors", "mistral"},
			{"qwen2-vl-7b.safetensors", "qwen"},
			{"gemma-2b.safetensors", "gemma"},
			{"unknown-model.safetensors", ""},
		}

		for _, tt := range tests {
			config := configFromFiles([]string{tt.filename})
			if config.Architecture != tt.expected {
				t.Errorf("For file %s, expected architecture %q, got %q",
					tt.filename, tt.expected, config.Architecture)
			}
		}
	})
}

func TestNewModelWithConfigArchive(t *testing.T) {
	// Test that the function properly handles config archives
	// In a real test, we'd use actual files

	safetensorsPaths := []string{"model.safetensors"}
	configPath := "config.tar"

	_, err := NewModelWithConfigArchive(safetensorsPaths, configPath)
	if err == nil {
		t.Error("Expected error for non-existent files, got nil")
	}
	// The error is expected since the files don't exist
}
