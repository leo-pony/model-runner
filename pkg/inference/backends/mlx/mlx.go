package mlx

import (
	"context"
	"net/http"

	"github.com/docker/model-runner/pkg/errordef"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/logger"
)

const (
	// Name is the backend name.
	Name = "mlx"
	// componentName is the component name.
	componentName = "inference-" + Name
)

var (
	// errors is used for generating and wrapping errors.
	errors = errordef.NewHelper(componentName, nil)
	// log is the log for the backend service.
	log = logger.Default.WithComponent(componentName)
)

// mlx is the MLX-based backend implementation.
type mlx struct{}

// New creates a new MLX-based backend.
func New() (inference.Backend, error) {
	// TODO: Implement (using mlx struct above).
	return nil, errors.New("not implemented")
}

// Name implements inference.Backend.Name.
func (m *mlx) Name() string {
	return Name
}

// Install implements inference.Backend.Install.
func (m *mlx) Install(ctx context.Context, httpClient *http.Client) error {
	// TODO: Implement.
	return errors.New("not implemented")
}

// Run implements inference.Backend.Run.
func (m *mlx) Run(ctx context.Context, socket string) error {
	// TODO: Implement.
	return errors.New("not implemented")
}
