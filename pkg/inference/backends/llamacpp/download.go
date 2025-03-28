package llamacpp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/docker/model-runner/pkg/internal/dockerhub"
	"github.com/docker/model-runner/pkg/paths"
)

const (
	hubNamespace = "docker"
	hubRepo      = "docker-model-backend-llamacpp"
)

func ensureLatestLlamaCpp(ctx context.Context, httpClient *http.Client, llamaCppPath string) error {
	url := fmt.Sprintf("https://hub.docker.com/v2/namespaces/%s/repositories/%s/tags", hubNamespace, hubRepo)
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// https://docs.docker.com/reference/api/hub/latest/#tag/repositories/paths/~1v2~1namespaces~1%7Bnamespace%7D~1repositories~1%7Brepository%7D~1tags/get
	var response struct {
		Results []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		}
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	var latest string
	for _, tag := range response.Results {
		if tag.Name == "latest-update" {
			latest = tag.Digest
			break
		}
	}
	if latest == "" {
		return fmt.Errorf("could not find any latest-update tag")
	}

	currentVersionFile := filepath.Join(filepath.Dir(llamaCppPath), ".llamacpp_version")
	data, err := os.ReadFile(currentVersionFile)
	if err != nil {
		log.Warnf("failed to read current llama.cpp version: %v", err)
		log.Warnf("proceeding to update llama.cpp binary")
	} else if strings.TrimSpace(string(data)) == latest {
		log.Infoln("current llama.cpp version is already up to date")
		if _, err := os.Stat(llamaCppPath); err == nil {
			return nil
		}
		log.Infoln("llama.cpp binary must be updated, proceeding to update it")
	} else {
		log.Infof("current llama.cpp version is outdated: %s vs %s, proceeding to update it", strings.TrimSpace(string(data)), latest)
	}

	image := fmt.Sprintf("registry-1.docker.io/%s/%s@%s", hubNamespace, hubRepo, latest)
	downloadDir := paths.DockerHome(".llamacpp-tmp")
	defer os.RemoveAll(downloadDir)

	if err := extractFromImage(ctx, image, runtime.GOOS, runtime.GOARCH, downloadDir); err != nil {
		return fmt.Errorf("could not extract image: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(llamaCppPath), 0o755); err != nil {
		return fmt.Errorf("could not create directory for llama.cpp binary: %w", err)
	}
	rootDir := "com.docker.llama-server.native.darwin.metal.arm64"
	if err := os.Rename(fmt.Sprintf("%s/%s/%s", downloadDir, rootDir, "bin/com.docker.llama-server"), llamaCppPath); err != nil {
		return fmt.Errorf("could not move llama.cpp binary: %w", err)
	}
	if err := os.Chmod(llamaCppPath, 0o755); err != nil {
		return fmt.Errorf("could not chmod llama.cpp binary: %w", err)
	}
	if err := os.Rename(fmt.Sprintf("%s/%s/%s", downloadDir, rootDir, "lib"),
		filepath.Join(filepath.Dir(filepath.Dir(llamaCppPath)), "lib")); err != nil {
		return fmt.Errorf("could not move llama.cpp libs: %w", err)
	}

	log.Infoln("successfully updated llama.cpp binary")
	log.Infoln("running llama.cpp version:", getLlamaCppVersion(llamaCppPath))

	if err := os.WriteFile(currentVersionFile, []byte(latest), 0o644); err != nil {
		log.Warnf("failed to save llama.cpp version: %v", err)
	}

	return nil
}

func extractFromImage(ctx context.Context, image, requiredOs, requiredArch, destination string) error {
	log.Infof("Extracting image %q to %q", image, destination)
	tmpDir, err := os.MkdirTemp("", "docker-tar-extract")
	if err != nil {
		return err
	}
	imageTar := filepath.Join(tmpDir, "save.tar")
	if err := dockerhub.PullPlatform(ctx, image, imageTar, requiredOs, requiredArch); err != nil {
		return err
	}
	return dockerhub.Extract(imageTar, requiredArch, requiredOs, destination)
}

func getLlamaCppVersion(llamaCpp string) string {
	output, err := exec.Command(llamaCpp, "--version").CombinedOutput()
	if err != nil {
		log.Warnf("could not get llama.cpp version: %v", err)
		return "unknown"
	}
	re := regexp.MustCompile(`version: \d+ \((\w+)\)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) == 2 {
		return matches[1]
	}
	log.Warnf("failed to parse llama.cpp version from output:\n%s", strings.TrimSpace(string(output)))
	return "unknown"
}
