//go:build !windows

package models

import (
	"context"
	"fmt"
)

// PullModel pulls a model to local storage. Any error it returns is suitable
// for writing back to the client.
func (m *Manager) PullModel(ctx context.Context, model string) error {
	// Restrict model pull concurrency.
	select {
	case <-m.pullTokens:
	case <-ctx.Done():
		return context.Canceled
	}
	defer func() {
		m.pullTokens <- struct{}{}
	}()

	// Pull the model using the Docker model distribution client
	m.log.Infoln("Pulling model:", model)
	if _, err := m.distributionClient.PullModel(ctx, model); err != nil {
		return fmt.Errorf("error while pulling model: %w", err)
	}

	return nil
}
