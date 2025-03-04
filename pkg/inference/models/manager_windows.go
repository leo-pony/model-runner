package models

import (
	"context"
	"errors"
)

// PullModel is unimplemented on Windows.
func (m *Manager) PullModel(ctx context.Context, model string) error {
	// TODO: The Hugging Face Hub package that we're using doesn't build on
	// Windows due to non-portable syscall package usage. With
	// docker/model-distribution just a few days away, it's not worth patching
	// or reimplementing. Once we switch to docker/model-distribution, delete
	// the _unix.go / _windows.go variants and move PullModel into manager.go.
	return errors.New("model pulls not yet supported on Windows")
}
