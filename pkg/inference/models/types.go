package models

// ModelCreateRequest represents a model create request. It is designed to
// follow Docker Engine API conventions, most closely following the request
// associated with POST /images/create. At the moment is only designed to
// facilitate pulls, though in the future it may facilitate model building and
// refinement (such as fine tuning, quantization, or distillation).
type ModelCreateRequest struct {
	// From is the name of the model to pull.
	From string `json:"from"`
}

// Model represents a locally stored model. It is designed to follow Docker
// Engine API conventions, most closely following the image model, though the
// casing and typing of its fields have been made more idiomatic.
type Model struct {
	// ID is the globally unique model identifier.
	ID string `json:"id"`
	// Tags are the list of tags associated with the model.
	Tags []string `json:"tags"`
	// Files are the GGUF files associated with the model.
	Files []string `json:"files"`
	// Created is the Unix epoch timestamp corresponding to the model creation.
	Created int64 `json:"created"`
}

// Model converts the model to its OpenAI API representation.
func (m *Model) toOpenAI() *OpenAIModel {
	return &OpenAIModel{
		ID:      m.Tags[0],
		Object:  "model",
		Created: m.Created,
		OwnedBy: "docker",
	}
}

// ModelList represents a list of models.
type ModelList []*Model

// Model converts the model to its OpenAI API representation. This method never
// returns a nil slice (though it may return an empty slice).
func (l ModelList) toOpenAI() *OpenAIModelList {
	// Convert the constituent models.
	models := make([]*OpenAIModel, len(l))
	for m, model := range l {
		models[m] = model.toOpenAI()
	}

	// Create the OpenAI model list.
	return &OpenAIModelList{
		Object: "list",
		Data:   models,
	}
}

// OpenAIModel represents a locally stored model using OpenAI conventions.
type OpenAIModel struct {
	// ID is the model tag.
	ID string `json:"id"`
	// Object is the object type. For OpenAIModel, it is always "model".
	Object string `json:"object"`
	// Created is the Unix epoch timestamp corresponding to the model creation.
	Created int64 `json:"created"`
	// OwnedBy is the model owner. At the moment, it is always "docker".
	OwnedBy string `json:"owned_by"`
}

// OpenAIModelList represents a list of models using OpenAI conventions.
type OpenAIModelList struct {
	// Object is the object type. For OpenAIModelList, it is always "list".
	Object string `json:"object"`
	// Data is the list of models.
	Data []*OpenAIModel `json:"data"`
}
