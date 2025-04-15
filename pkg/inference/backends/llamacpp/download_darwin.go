package llamacpp

import (
	"context"
	"net/http"

	"github.com/docker/model-runner/pkg/logging"
)

func ensureLatestLlamaCpp(ctx context.Context, log logging.Logger, httpClient *http.Client,
	llamaCppPath, vendoredServerStoragePath string,
) error {
	desiredVersion := "latest"
	desiredVariant := "metal"
	return downloadLatestLlamaCpp(ctx, log, httpClient, llamaCppPath, vendoredServerStoragePath, desiredVersion,
		desiredVariant)
}
