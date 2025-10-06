package packaging

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PackageFromDirectory scans a directory for safetensors files and config files,
// creating a temporary tar archive of the config files.
// It returns the paths to safetensors files, path to temporary config archive (if created),
// and any error encountered.
func PackageFromDirectory(dirPath string) (safetensorsPaths []string, tempConfigArchive string, err error) {
	// Read directory contents (only top level, no subdirectories)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, "", fmt.Errorf("read directory: %w", err)
	}

	var configFiles []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip subdirectories
		}

		name := entry.Name()
		fullPath := filepath.Join(dirPath, name)

		// Collect safetensors files
		if strings.HasSuffix(strings.ToLower(name), ".safetensors") {
			safetensorsPaths = append(safetensorsPaths, fullPath)
		}

		// Collect config files: *.json, merges.txt
		if strings.HasSuffix(strings.ToLower(name), ".json") || strings.EqualFold(name, "merges.txt") {
			configFiles = append(configFiles, fullPath)
		}
	}

	if len(safetensorsPaths) == 0 {
		return nil, "", fmt.Errorf("no safetensors files found in directory: %s", dirPath)
	}

	// Sort to ensure reproducible artifacts
	sort.Strings(safetensorsPaths)

	// Create temporary tar archive with config files if any exist
	if len(configFiles) > 0 {
		// Sort config files for reproducible tar archive
		sort.Strings(configFiles)

		tempConfigArchive, err = CreateTempConfigArchive(configFiles)
		if err != nil {
			return nil, "", fmt.Errorf("create config archive: %w", err)
		}
	}

	return safetensorsPaths, tempConfigArchive, nil
}

// CreateTempConfigArchive creates a temporary tar archive containing the specified config files.
// It returns the path to the temporary tar file and any error encountered.
// The caller is responsible for removing the temporary file when done.
func CreateTempConfigArchive(configFiles []string) (string, error) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "vllm-config-*.tar")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Create tar writer
	tw := tar.NewWriter(tmpFile)

	// Add each config file to tar (preserving just filename, not full path)
	for _, filePath := range configFiles {
		// Open the file
		file, err := os.Open(filePath)
		if err != nil {
			tw.Close()
			tmpFile.Close()
			os.Remove(tmpPath)
			return "", fmt.Errorf("open config file %s: %w", filePath, err)
		}

		// Get file info for tar header
		fileInfo, err := file.Stat()
		if err != nil {
			file.Close()
			tw.Close()
			tmpFile.Close()
			os.Remove(tmpPath)
			return "", fmt.Errorf("stat config file %s: %w", filePath, err)
		}

		// Create tar header (use only basename, not full path)
		header := &tar.Header{
			Name:    filepath.Base(filePath),
			Size:    fileInfo.Size(),
			Mode:    int64(fileInfo.Mode()),
			ModTime: fileInfo.ModTime(),
		}

		// Write header
		if err := tw.WriteHeader(header); err != nil {
			file.Close()
			tw.Close()
			tmpFile.Close()
			os.Remove(tmpPath)
			return "", fmt.Errorf("write tar header for %s: %w", filePath, err)
		}

		// Copy file contents
		if _, err := io.Copy(tw, file); err != nil {
			file.Close()
			tw.Close()
			tmpFile.Close()
			os.Remove(tmpPath)
			return "", fmt.Errorf("write tar content for %s: %w", filePath, err)
		}

		file.Close()
	}

	// Close tar writer and file
	if err := tw.Close(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("close tar writer: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("close temp file: %w", err)
	}

	return tmpPath, nil
}
