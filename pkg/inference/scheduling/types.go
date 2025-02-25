package scheduling

// completionOrEmbeddingRequest is used to extract the model specification from
// either a chat completion or embedding request in the OpenAI API.
type completionOrEmbeddingRequest struct {
	// Model is the requested model name.
	Model string `json:"model"`
}
