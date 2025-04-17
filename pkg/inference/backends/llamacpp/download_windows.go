package llamacpp

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/docker/model-runner/pkg/logging"
)

func (l *llamaCpp) ensureLatestLlamaCpp(ctx context.Context, log logging.Logger, httpClient *http.Client,
	llamaCppPath, vendoredServerStoragePath string,
) error {
	nvGPUInfoBin := filepath.Join(vendoredServerStoragePath, "com.docker.nv-gpu-info.exe")
	var canUseCUDA11 bool
	var err error
	ShouldUseGPUVariantLock.Lock()
	defer ShouldUseGPUVariantLock.Unlock()
	if ShouldUseGPUVariant {
		canUseCUDA11, err = hasCUDA11CapableGPU(ctx, nvGPUInfoBin)
		if err != nil {
			l.status = fmt.Sprintf("failed to check CUDA 11 capability: %v", err)
			return fmt.Errorf("failed to check CUDA 11 capability: %w", err)
		}
	}
	desiredVersion := "latest"
	desiredVariant := "cpu"
	if canUseCUDA11 {
		desiredVariant = "cuda"
	}
	l.status = fmt.Sprintf("looking for updates for %s variant", desiredVariant)
	return l.downloadLatestLlamaCpp(ctx, log, httpClient, llamaCppPath, vendoredServerStoragePath, desiredVersion,
		desiredVariant)
}
