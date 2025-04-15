package llamacpp

import (
	"context"
	"errors"
	"net/http"

	"github.com/docker/model-runner/pkg/logging"
)

func ensureLatestLlamaCpp(ctx context.Context, log logging.Logger, httpClient *http.Client,
	llamaCppPath, vendoredServerStoragePath string,
) error {
	return errors.New("platform is not supported")
}
