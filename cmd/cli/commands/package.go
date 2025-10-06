package commands

import (
	"archive/tar"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/docker/model-runner/pkg/distribution/builder"
	"github.com/docker/model-runner/pkg/distribution/registry"
	"github.com/docker/model-runner/pkg/distribution/tarball"
	"github.com/docker/model-runner/pkg/distribution/types"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/spf13/cobra"

	"github.com/docker/model-cli/commands/completion"
	"github.com/docker/model-cli/desktop"
)

func newPackagedCmd() *cobra.Command {
	var opts packageOptions

	c := &cobra.Command{
		Use:   "package (--gguf <path> | --safetensors-dir <path>) [--license <path>...] [--context-size <tokens>] [--push] MODEL",
		Short: "Package a GGUF file or Safetensors directory into a Docker model OCI artifact.",
		Long: "Package a GGUF file or Safetensors directory into a Docker model OCI artifact, with optional licenses. The package is sent to the model-runner, unless --push is specified.\n" +
			"When packaging a sharded GGUF model, --gguf should point to the first shard. All shard files should be siblings and should include the index in the file name (e.g. model-00001-of-00015.gguf).\n" +
			"When packaging a Safetensors model, --safetensors-dir should point to a directory containing .safetensors files and config files (*.json, merges.txt). All files will be auto-discovered and config files will be packaged into a tar archive.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf(
					"'docker model package' requires 1 argument.\n\n"+
						"Usage:  docker model %s\n\n"+
						"See 'docker model package --help' for more information",
					cmd.Use,
				)
			}

			// Validate that either --gguf or --safetensors-dir is provided (mutually exclusive)
			if opts.ggufPath == "" && opts.safetensorsDir == "" {
				return fmt.Errorf(
					"Either --gguf or --safetensors-dir path is required.\n\n" +
						"See 'docker model package --help' for more information",
				)
			}
			if opts.ggufPath != "" && opts.safetensorsDir != "" {
				return fmt.Errorf(
					"Cannot specify both --gguf and --safetensors-dir. Please use only one format.\n\n" +
						"See 'docker model package --help' for more information",
				)
			}

			// Validate GGUF path if provided
			if opts.ggufPath != "" {
				if !filepath.IsAbs(opts.ggufPath) {
					return fmt.Errorf(
						"GGUF path must be absolute.\n\n" +
							"See 'docker model package --help' for more information",
					)
				}
				opts.ggufPath = filepath.Clean(opts.ggufPath)
			}

			// Validate safetensors directory if provided
			if opts.safetensorsDir != "" {
				if !filepath.IsAbs(opts.safetensorsDir) {
					return fmt.Errorf(
						"Safetensors directory path must be absolute.\n\n" +
							"See 'docker model package --help' for more information",
					)
				}
				opts.safetensorsDir = filepath.Clean(opts.safetensorsDir)

				// Check if it's a directory
				info, err := os.Stat(opts.safetensorsDir)
				if err != nil {
					return fmt.Errorf(
						"Safetensors directory does not exist: %s\n\n"+
							"See 'docker model package --help' for more information",
						opts.safetensorsDir,
					)
				}
				if !info.IsDir() {
					return fmt.Errorf(
						"Safetensors path must be a directory: %s\n\n"+
							"See 'docker model package --help' for more information",
						opts.safetensorsDir,
					)
				}
			}

			for i, l := range opts.licensePaths {
				if !filepath.IsAbs(l) {
					return fmt.Errorf(
						"license path must be absolute.\n\n" +
							"See 'docker model package --help' for more information",
					)
				}
				opts.licensePaths[i] = filepath.Clean(l)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.tag = args[0]
			if err := packageModel(cmd, opts); err != nil {
				cmd.PrintErrln("Failed to package model")
				return fmt.Errorf("package model: %w", err)
			}
			return nil
		},
		ValidArgsFunction: completion.NoComplete,
	}

	c.Flags().StringVar(&opts.ggufPath, "gguf", "", "absolute path to gguf file")
	c.Flags().StringVar(&opts.safetensorsDir, "safetensors-dir", "", "absolute path to directory containing safetensors files and config")
	c.Flags().StringVar(&opts.chatTemplatePath, "chat-template", "", "absolute path to chat template file (must be Jinja format)")
	c.Flags().StringArrayVarP(&opts.licensePaths, "license", "l", nil, "absolute path to a license file")
	c.Flags().BoolVar(&opts.push, "push", false, "push to registry (if not set, the model is loaded into the Model Runner content store)")
	c.Flags().Uint64Var(&opts.contextSize, "context-size", 0, "context size in tokens")
	return c
}

type packageOptions struct {
	chatTemplatePath string
	contextSize      uint64
	ggufPath         string
	safetensorsDir   string
	licensePaths     []string
	push             bool
	tag              string
}

func packageModel(cmd *cobra.Command, opts packageOptions) error {
	var (
		target builder.Target
		err    error
	)
	if opts.push {
		target, err = registry.NewClient(
			registry.WithUserAgent("docker-model-cli/" + desktop.Version),
		).NewTarget(opts.tag)
	} else {
		target, err = newModelRunnerTarget(desktopClient, opts.tag)
	}
	if err != nil {
		return err
	}

	// Create package builder based on model format
	var pkg *builder.Builder
	if opts.ggufPath != "" {
		cmd.PrintErrf("Adding GGUF file from %q\n", opts.ggufPath)
		pkg, err = builder.FromGGUF(opts.ggufPath)
		if err != nil {
			return fmt.Errorf("add gguf file: %w", err)
		}
	} else {
		// Safetensors model from directory
		cmd.PrintErrf("Scanning directory %q for safetensors model...\n", opts.safetensorsDir)
		safetensorsPaths, tempConfigArchive, err := packageFromDirectory(opts.safetensorsDir)
		if err != nil {
			return fmt.Errorf("scan safetensors directory: %w", err)
		}

		// Clean up temp config archive when done
		if tempConfigArchive != "" {
			defer os.Remove(tempConfigArchive)
		}

		cmd.PrintErrf("Found %d safetensors file(s)\n", len(safetensorsPaths))
		pkg, err = builder.FromSafetensors(safetensorsPaths)
		if err != nil {
			return fmt.Errorf("create safetensors model: %w", err)
		}

		// Add config archive if it was created
		if tempConfigArchive != "" {
			cmd.PrintErrf("Adding config archive from directory\n")
			pkg, err = pkg.WithConfigArchive(tempConfigArchive)
			if err != nil {
				return fmt.Errorf("add config archive: %w", err)
			}
		}
	}

	// Set context size
	if opts.contextSize > 0 {
		cmd.PrintErrf("Setting context size %d\n", opts.contextSize)
		pkg = pkg.WithContextSize(opts.contextSize)
	}

	// Add license files
	for _, path := range opts.licensePaths {
		cmd.PrintErrf("Adding license file from %q\n", path)
		pkg, err = pkg.WithLicense(path)
		if err != nil {
			return fmt.Errorf("add license file: %w", err)
		}
	}

	if opts.chatTemplatePath != "" {
		cmd.PrintErrf("Adding chat template file from %q\n", opts.chatTemplatePath)
		if pkg, err = pkg.WithChatTemplateFile(opts.chatTemplatePath); err != nil {
			return fmt.Errorf("add chat template file from path %q: %w", opts.chatTemplatePath, err)
		}
	}

	if opts.push {
		cmd.PrintErrln("Pushing model to registry...")
	} else {
		cmd.PrintErrln("Loading model to Model Runner...")
	}
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		defer pw.Close()
		done <- pkg.Build(cmd.Context(), target, pw)
	}()

	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		progressLine := scanner.Text()
		if progressLine == "" {
			continue
		}

		// Parse the progress message
		var progressMsg desktop.ProgressMessage
		if err := json.Unmarshal([]byte(html.UnescapeString(progressLine)), &progressMsg); err != nil {
			cmd.PrintErrln("Error displaying progress:", err)
		}

		// Print progress messages
		TUIProgress(progressMsg.Message)
	}
	cmd.PrintErrln("") // newline after progress

	if err := scanner.Err(); err != nil {
		cmd.PrintErrln("Error streaming progress:", err)
	}
	if err := <-done; err != nil {
		if opts.push {
			return fmt.Errorf("failed to save packaged model: %w", err)
		}
		return fmt.Errorf("failed to load packaged model: %w", err)
	}

	if opts.push {
		cmd.PrintErrln("Model pushed successfully")
	} else {
		cmd.PrintErrln("Model loaded successfully")
	}
	return nil
}

// modelRunnerTarget loads model to Docker Model Runner via models/load endpoint
type modelRunnerTarget struct {
	client *desktop.Client
	tag    name.Tag
}

func newModelRunnerTarget(client *desktop.Client, tag string) (*modelRunnerTarget, error) {
	target := &modelRunnerTarget{
		client: client,
	}
	if tag != "" {
		var err error
		target.tag, err = name.NewTag(tag)
		if err != nil {
			return nil, fmt.Errorf("invalid tag: %w", err)
		}
	}
	return target, nil
}

func (t *modelRunnerTarget) Write(ctx context.Context, mdl types.ModelArtifact, progressWriter io.Writer) error {
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		defer pw.Close()
		target, err := tarball.NewTarget(pw)
		if err != nil {
			errCh <- err
			return
		}
		errCh <- target.Write(ctx, mdl, progressWriter)
	}()

	loadErr := t.client.LoadModel(ctx, pr)
	writeErr := <-errCh

	if loadErr != nil {
		return fmt.Errorf("loading model archive: %w", loadErr)
	}
	if writeErr != nil {
		return fmt.Errorf("writing model archive: %w", writeErr)
	}
	id, err := mdl.ID()
	if err != nil {
		return fmt.Errorf("get model ID: %w", err)
	}
	if t.tag.String() != "" {
		if err := desktopClient.Tag(id, parseRepo(t.tag), t.tag.TagStr()); err != nil {
			return fmt.Errorf("tag model: %w", err)
		}
	}
	return nil
}

// packageFromDirectory scans a directory for safetensors files and config files,
// creating a temporary tar archive of the config files
func packageFromDirectory(dirPath string) (safetensorsPaths []string, tempConfigArchive string, err error) {
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
		if filepath.Ext(name) == ".safetensors" {
			safetensorsPaths = append(safetensorsPaths, fullPath)
		}

		// Collect config files: *.json, merges.txt
		if filepath.Ext(name) == ".json" || name == "merges.txt" {
			configFiles = append(configFiles, fullPath)
		}
	}

	if len(safetensorsPaths) == 0 {
		return nil, "", fmt.Errorf("no safetensors files found in directory: %s", dirPath)
	}

	// Sort to ensure reproducible artifacts
	sortStrings(safetensorsPaths)

	// Create temporary tar archive with config files if any exist
	if len(configFiles) > 0 {
		// Sort config files for reproducible tar archive
		sortStrings(configFiles)

		tempConfigArchive, err = createTempConfigArchive(configFiles)
		if err != nil {
			return nil, "", fmt.Errorf("create config archive: %w", err)
		}
	}

	return safetensorsPaths, tempConfigArchive, nil
}

// createTempConfigArchive creates a temporary tar archive containing the specified config files
func createTempConfigArchive(configFiles []string) (string, error) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", "vllm-config-*.tar")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Import archive/tar at the top if not already imported
	// We'll use the tar package here
	tw := newTarWriter(tmpFile)

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
		header := newTarHeader(filepath.Base(filePath), fileInfo)

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

// sortStrings sorts a slice of strings in place
func sortStrings(s []string) {
	sort.Strings(s)
}

// newTarWriter creates a new tar writer
func newTarWriter(w io.Writer) *tar.Writer {
	return tar.NewWriter(w)
}

// newTarHeader creates a tar header from file info
func newTarHeader(name string, fileInfo os.FileInfo) *tar.Header {
	return &tar.Header{
		Name:    name,
		Size:    fileInfo.Size(),
		Mode:    int64(fileInfo.Mode()),
		ModTime: fileInfo.ModTime(),
	}
}
