package llamacpp

import (
	"context"
	"net/http"

	"github.com/docker/model-runner/pkg/errordef"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/logger"
)

const (
	// Name is the backend name.
	Name = "llama.cpp"
	// componentName is the component name.
	componentName = "inference-" + Name
)

var (
	// errors is used for generating and wrapping errors.
	errors = errordef.NewHelper(componentName, nil)
	// log is the log for the backend service.
	log = logger.Default.WithComponent(componentName)
)

// llamaCpp is the llama.cpp-based backend implementation.
type llamaCpp struct{}

// New creates a new llama.cpp-based backend.
func New() (inference.Backend, error) {
	// TODO: Implement (using llamaCpp struct above).
	return nil, errors.New("not implemented")
}

// Name implements inference.Backend.Name.
func (l *llamaCpp) Name() string {
	return Name
}

// Install implements inference.Backend.Install.
func (l *llamaCpp) Install(ctx context.Context, httpClient *http.Client) error {
	// TODO: Implement.
	return errors.New("not implemented")
}

// Run implements inference.Backend.Run.
func (l *llamaCpp) Run(ctx context.Context, socket string) error {
	// TODO: Implement.
	return errors.New("not implemented")
}
