package safetensors

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/docker/model-runner/pkg/distribution/internal/partial"
	"github.com/docker/model-runner/pkg/distribution/types"
)

// NewModel creates a new safetensors model from one or more safetensors files
// If a sharded model pattern is detected (e.g., model-00001-of-00002.safetensors),
// it will auto-discover all related shards
func NewModel(paths []string) (*Model, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("at least one safetensors file is required")
	}

	// Auto-discover shards if the first path matches the shard pattern
	allPaths := discoverSafetensorsShards(paths[0])
	if len(allPaths) == 0 {
		// No shards found, use provided paths as-is
		allPaths = paths
	}

	layers := make([]v1.Layer, len(allPaths))
	diffIDs := make([]v1.Hash, len(allPaths))

	for i, path := range allPaths {
		layer, err := partial.NewLayer(path, types.MediaTypeSafetensors)
		if err != nil {
			return nil, fmt.Errorf("create safetensors layer from %q: %w", path, err)
		}
		diffID, err := layer.DiffID()
		if err != nil {
			return nil, fmt.Errorf("get safetensors layer diffID: %w", err)
		}
		layers[i] = layer
		diffIDs[i] = diffID
	}

	created := time.Now()
	return &Model{
		configFile: types.ConfigFile{
			Config: configFromFiles(paths),
			Descriptor: types.Descriptor{
				Created: &created,
			},
			RootFS: v1.RootFS{
				Type:    "rootfs",
				DiffIDs: diffIDs,
			},
		},
		layers: layers,
	}, nil
}

// NewModelWithConfigArchive creates a new safetensors model with a config archive
func NewModelWithConfigArchive(safetensorsPaths []string, configArchivePath string) (*Model, error) {
	model, err := NewModel(safetensorsPaths)
	if err != nil {
		return nil, err
	}

	// Add config archive layer
	if configArchivePath != "" {
		configLayer, err := partial.NewLayer(configArchivePath, types.MediaTypeVLLMConfigArchive)
		if err != nil {
			return nil, fmt.Errorf("create config archive layer from %q: %w", configArchivePath, err)
		}

		diffID, err := configLayer.DiffID()
		if err != nil {
			return nil, fmt.Errorf("get config archive layer diffID: %w", err)
		}

		model.layers = append(model.layers, configLayer)
		model.configFile.RootFS.DiffIDs = append(model.configFile.RootFS.DiffIDs, diffID)
	}

	return model, nil
}

// discoverSafetensorsShards attempts to auto-discover all shards for a given safetensors file
// It looks for the pattern: <name>-XXXXX-of-YYYYY.safetensors
// Returns an empty slice if no shards are found or if it's a single file
func discoverSafetensorsShards(path string) []string {
	// Pattern: model-00001-of-00003.safetensors
	pattern := regexp.MustCompile(`^(.+)-(\d{5})-of-(\d{5})\.safetensors$`)

	baseName := filepath.Base(path)
	matches := pattern.FindStringSubmatch(baseName)

	if len(matches) != 4 {
		// Not a sharded file, return empty to indicate single file
		return nil
	}

	prefix := matches[1]
	totalShards, err := strconv.Atoi(matches[3])
	if err != nil {
		return nil
	}

	dir := filepath.Dir(path)
	var shards []string

	// Look for all shards in the same directory
	for i := 1; i <= totalShards; i++ {
		shardName := fmt.Sprintf("%s-%05d-of-%05d.safetensors", prefix, i, totalShards)
		shardPath := filepath.Join(dir, shardName)

		// Check if the file exists
		if _, err := os.Stat(shardPath); err == nil {
			shards = append(shards, shardPath)
		}
	}

	// Only return if we found all expected shards
	if len(shards) == totalShards {
		// Shards are already in order due to sequential loop
		return shards
	}

	return nil
}

func configFromFiles(paths []string) types.Config {
	// Extract basic metadata from file paths
	// This is a simplified version - in production, you might want to
	// parse safetensors headers for more detailed metadata

	var totalFiles int
	var architecture string

	if len(paths) > 0 {
		totalFiles = len(paths)
		// Try to extract architecture from filename
		baseName := filepath.Base(paths[0])
		baseName = strings.ToLower(baseName)

		// Common patterns in model filenames
		if strings.Contains(baseName, "llama") {
			architecture = "llama"
		} else if strings.Contains(baseName, "mistral") {
			architecture = "mistral"
		} else if strings.Contains(baseName, "qwen") {
			architecture = "qwen"
		} else if strings.Contains(baseName, "gemma") {
			architecture = "gemma"
		}
	}

	safetensorsMetadata := map[string]string{
		"total_files": fmt.Sprintf("%d", totalFiles),
	}

	if architecture != "" {
		safetensorsMetadata["architecture"] = architecture
	}

	return types.Config{
		Format:       types.FormatSafetensors,
		Architecture: architecture,
		Safetensors:  safetensorsMetadata,
	}
}
