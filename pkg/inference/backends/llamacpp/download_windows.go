package llamacpp

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/docker/model-runner/pkg/logging"
)

func ensureLatestLlamaCpp(ctx context.Context, log logging.Logger, httpClient *http.Client,
	llamaCppPath, vendoredServerStoragePath string,
) error {
	nvGPUInfoBin := filepath.Join(vendoredServerStoragePath, "com.docker.nv-gpu-info.exe")
	var canUseCUDA11 bool
	var err error
	if ShouldUseGPUVariant {
		canUseCUDA11, err = hasCUDA11CapableGPU(ctx, nvGPUInfoBin)
		if err != nil {
			return fmt.Errorf("failed to check CUDA 11 capability: %w", err)
		}
	}
	desiredVersion := "latest"
	desiredVariant := "cpu"
	if canUseCUDA11 {
		desiredVariant = "cuda"
	}
	return downloadLatestLlamaCpp(ctx, log, httpClient, llamaCppPath, vendoredServerStoragePath, desiredVersion,
		desiredVariant)
}
