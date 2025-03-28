package mlx

import (
	"context"
	"errors"
	"net/http"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logger"
)

const (
	// Name is the backend name.
	Name = "mlx"
	// componentName is the component name.
	componentName = "inference-" + Name
)

var (
	// log is the log for the backend service.
	log = logger.Default.WithComponent(componentName)
)

// mlx is the MLX-based backend implementation.
type mlx struct {
	// modelManager is the shared model manager.
	modelManager *models.Manager
}

// New creates a new MLX-based backend.
func New(modelManager *models.Manager) (inference.Backend, error) {
	return &mlx{modelManager: modelManager}, nil
}

// Name implements inference.Backend.Name.
func (m *mlx) Name() string {
	return Name
}

// UsesExternalModelManagement implements
// inference.Backend.UsesExternalModelManagement.
func (l *mlx) UsesExternalModelManagement() bool {
	return false
}

// Install implements inference.Backend.Install.
func (m *mlx) Install(ctx context.Context, httpClient *http.Client) error {
	// TODO: Implement.
	return errors.New("not implemented")
}

// Run implements inference.Backend.Run.
func (m *mlx) Run(ctx context.Context, socket, model string, mode inference.BackendMode) error {
	// TODO: Implement.
	log.Warn("MLX backend is not yet supported")
	return errors.New("not implemented")
}
