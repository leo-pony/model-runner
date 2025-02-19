package inference

import (
	"context"
	"net/http"
)

// Backend is the interface implemented by inference engine backends. Backend
// implementations need not be safe for concurrent invocation of the following
// methods, though their underlying server implementations do need to support
// concurrent API requests.
type Backend interface {
	// Name returns the backend name. It must be all lowercase and usable as a
	// path component in an HTTP request path and a Unix domain socket path. It
	// should also be suitable for presenting to users (at least in logs). The
	// package providing the backend implementation should also expose a
	// variable called Name which matches the value returned by this method.
	Name() string
	// Install ensures that the backend is installed. It should return a nil
	// error if the backend is already installed. The provided HTTP client
	// should be used for any HTTP operations.
	Install(ctx context.Context, httpClient *http.Client) error
	// Run is the run loop for the backend. It should start any process(es)
	// necessary for the backend to function. It should not return until either
	// the process(es) fail or the provided context is cancelled. By the time
	// Run returns, any process(es) it has spawned must terminate.
	//
	// If desired, implementations may implement their own restart mechanisms on
	// process failure, though implementations may also be "one-shot" (i.e.
	// returning from Run after failure of an underlying process), in which case
	// higher-level logic in the inference service will manage their restart.
	//
	// Run will be provided with the path to a Unix domain socket on which the
	// backend should listen for incoming OpenAI API requests.
	Run(ctx context.Context, socket string) error
}
