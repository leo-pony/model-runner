package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/model-runner/pkg/distribution/types"
	ggcrtypes "github.com/google/go-containerregistry/pkg/v1/types"
)

// Unpack creates and return a Bundle by unpacking files and config from model into dir.
func Unpack(dir string, model types.Model) (*Bundle, error) {
	bundle := &Bundle{
		dir: dir,
	}

	// Inspect layers to determine what to unpack
	modelFormat := detectModelFormat(model)

	// Unpack model weights based on detected format
	switch modelFormat {
	case types.FormatGGUF:
		if err := unpackGGUFs(bundle, model); err != nil {
			return nil, fmt.Errorf("unpack GGUF files: %w", err)
		}
	case types.FormatSafetensors:
		if err := unpackSafetensors(bundle, model); err != nil {
			return nil, fmt.Errorf("unpack safetensors files: %w", err)
		}
	default:
		return nil, fmt.Errorf("no supported model weights found (neither GGUF nor safetensors)")
	}

	// Unpack optional components based on their presence
	if hasLayerWithMediaType(model, types.MediaTypeMultimodalProjector) {
		if err := unpackMultiModalProjector(bundle, model); err != nil {
			return nil, fmt.Errorf("add multi-model projector file to runtime bundle: %w", err)
		}
	}

	if hasLayerWithMediaType(model, types.MediaTypeChatTemplate) {
		if err := unpackTemplate(bundle, model); err != nil {
			return nil, fmt.Errorf("add chat template file to runtime bundle: %w", err)
		}
	}

	if hasLayerWithMediaType(model, types.MediaTypeVLLMConfigArchive) {
		if err := unpackConfigArchive(bundle, model); err != nil {
			return nil, fmt.Errorf("add config archive to runtime bundle: %w", err)
		}
	}

	// Always create the runtime config
	if err := unpackRuntimeConfig(bundle, model); err != nil {
		return nil, fmt.Errorf("add config.json to runtime bundle: %w", err)
	}

	return bundle, nil
}

// detectModelFormat inspects the model to determine the primary model format
func detectModelFormat(model types.Model) types.Format {
	// Check for GGUF files
	ggufPaths, err := model.GGUFPaths()
	if err == nil && len(ggufPaths) > 0 {
		return types.FormatGGUF
	}

	// Check for Safetensors files
	safetensorsPaths, err := model.SafetensorsPaths()
	if err == nil && len(safetensorsPaths) > 0 {
		return types.FormatSafetensors
	}

	return ""
}

// hasLayerWithMediaType checks if the model contains a layer with the specified media type
func hasLayerWithMediaType(model types.Model, targetMediaType ggcrtypes.MediaType) bool {
	// Check specific media types using the model's methods
	switch targetMediaType {
	case types.MediaTypeMultimodalProjector:
		path, err := model.MMPROJPath()
		return err == nil && path != ""
	case types.MediaTypeChatTemplate:
		path, err := model.ChatTemplatePath()
		return err == nil && path != ""
	case types.MediaTypeVLLMConfigArchive:
		path, err := model.ConfigArchivePath()
		return err == nil && path != ""
	default:
		return false
	}
}

func unpackRuntimeConfig(bundle *Bundle, mdl types.Model) error {
	cfg, err := mdl.Config()
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(bundle.dir, "config.json"))
	if err != nil {
		return fmt.Errorf("create runtime config file: %w", err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(cfg); err != nil {
		return fmt.Errorf("encode runtime config: %w", err)
	}
	bundle.runtimeConfig = cfg
	return nil
}

func unpackGGUFs(bundle *Bundle, mdl types.Model) error {
	ggufPaths, err := mdl.GGUFPaths()
	if err != nil {
		return fmt.Errorf("get GGUF files for model: %w", err)
	}

	if len(ggufPaths) == 1 {
		if err := unpackFile(filepath.Join(bundle.dir, "model.gguf"), ggufPaths[0]); err != nil {
			return err
		}
		bundle.ggufFile = "model.gguf"
		return err
	}

	for i := range ggufPaths {
		name := fmt.Sprintf("model-%05d-of-%05d.gguf", i+1, len(ggufPaths))
		if err := unpackFile(filepath.Join(bundle.dir, name), ggufPaths[i]); err != nil {
			return err
		}
		bundle.ggufFile = name
	}

	return nil
}

func unpackMultiModalProjector(bundle *Bundle, mdl types.Model) error {
	path, err := mdl.MMPROJPath()
	if err != nil {
		return nil // no such file
	}
	if err = unpackFile(filepath.Join(bundle.dir, "model.mmproj"), path); err != nil {
		return err
	}
	bundle.mmprojPath = "model.mmproj"
	return nil
}

func unpackTemplate(bundle *Bundle, mdl types.Model) error {
	path, err := mdl.ChatTemplatePath()
	if err != nil {
		return nil // no such file
	}
	if err = unpackFile(filepath.Join(bundle.dir, "template.jinja"), path); err != nil {
		return err
	}
	bundle.chatTemplatePath = "template.jinja"
	return nil
}

func unpackSafetensors(bundle *Bundle, mdl types.Model) error {
	safetensorsPaths, err := mdl.SafetensorsPaths()
	if err != nil {
		return fmt.Errorf("get safetensors files for model: %w", err)
	}

	if len(safetensorsPaths) == 0 {
		return fmt.Errorf("no safetensors files found")
	}

	if len(safetensorsPaths) == 1 {
		if err := unpackFile(filepath.Join(bundle.dir, "model.safetensors"), safetensorsPaths[0]); err != nil {
			return err
		}
		bundle.safetensorsFile = "model.safetensors"
		return nil
	}

	// Handle sharded safetensors files
	for i := range safetensorsPaths {
		name := fmt.Sprintf("model-%05d-of-%05d.safetensors", i+1, len(safetensorsPaths))
		if err := unpackFile(filepath.Join(bundle.dir, name), safetensorsPaths[i]); err != nil {
			return err
		}
		if i == 0 {
			bundle.safetensorsFile = name
		}
	}

	return nil
}

func unpackConfigArchive(bundle *Bundle, mdl types.Model) error {
	archivePath, err := mdl.ConfigArchivePath()
	if err != nil {
		return nil // no config archive
	}

	// Create config directory
	configDir := filepath.Join(bundle.dir, "configs")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	// Extract the tar archive
	if err := extractTarArchive(archivePath, configDir); err != nil {
		return fmt.Errorf("extract config archive: %w", err)
	}

	bundle.configDir = "configs"
	return nil
}

func extractTarArchive(archivePath, destDir string) error {
	// For now, we'll just link the tar file.
	// TODO: Implement proper tar extraction using archive/tar package
	// This would extract files like tokenizer.json, config.json, etc.
	return os.Link(archivePath, filepath.Join(destDir, "config.tar"))
}

func unpackFile(bundlePath string, srcPath string) error {
	return os.Link(srcPath, bundlePath)
}
