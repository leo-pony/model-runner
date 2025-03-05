package scheduling

import (
	"strings"

	"github.com/docker/model-runner/pkg/inference"
)

const (
	// maximumOpenAIInferenceRequestSize is the maximum OpenAI API embedding or
	// completion request size that Scheduler will allow. This should be large
	// enough to encompass any real-world request but also small enough to avoid
	// DoS attacks.
	maximumOpenAIInferenceRequestSize = 10 * 1024 * 1024
)

// trimRequestPathToOpenAIRoot trims a request path to start at the first
// instance of /v1/ to appear in the path.
func trimRequestPathToOpenAIRoot(path string) string {
	index := strings.Index(path, "/v1/")
	if index == -1 {
		return path
	}
	return path[index:]
}

// backendModeForRequest determines the backend operation mode to handle an
// OpenAI inference request. Its second parameter is true if and only if a valid
// mode could be determined.
func backendModeForRequest(path string) (inference.BackendMode, bool) {
	if strings.HasSuffix(path, "/v1/chat/completions") || strings.HasSuffix(path, "/v1/completions") {
		return inference.BackendModeCompletion, true
	} else if strings.HasSuffix(path, "/v1/embeddings") {
		return inference.BackendModeEmbedding, true
	}
	return inference.BackendMode(0), false
}

// OpenAIInferenceRequest is used to extract the model specification from either
// a chat completion or embedding request in the OpenAI API.
type OpenAIInferenceRequest struct {
	// Model is the requested model name.
	Model string `json:"model"`
}
