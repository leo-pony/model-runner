package vllm

import (
	"context"
	"net/http"

	"github.com/docker/model-runner/pkg/errordef"
	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/logger"
)

const (
	// Name is the backend name.
	Name = "vllm"
	// componentName is the component name.
	componentName = "inference-" + Name
)

var (
	// errors is used for generating and wrapping errors.
	errors = errordef.NewHelper(componentName, nil)
	// log is the log for the backend service.
	log = logger.Default.WithComponent(componentName)
)

// vLLM is the vLLM-based backend implementation.
type vLLM struct{}

// New creates a new vLLM-based backend.
func New() (inference.Backend, error) {
	// TODO: Implement (using vLLM struct above).
	return nil, errors.New("not implemented")
}

// Name implements inference.Backend.Name.
func (v *vLLM) Name() string {
	return Name
}

// Install implements inference.Backend.Install.
func (v *vLLM) Install(ctx context.Context, httpClient *http.Client) error {
	// TODO: Implement.
	return errors.New("not implemented")
}

// Run implements inference.Backend.Run.
func (v *vLLM) Run(ctx context.Context, socket string) error {
	// TODO: Implement.
	return errors.New("not implemented")
}
