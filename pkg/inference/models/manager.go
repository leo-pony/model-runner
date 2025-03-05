package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/docker/model-distribution/pkg/distribution"
	"github.com/docker/model-runner/pkg/logger"
)

const (
	// maximumConcurrentModelPulls is the maximum number of concurrent model
	// pulls that a model manager will allow.
	maximumConcurrentModelPulls = 2
)

// Manager manages inference model pulls and storage.
type Manager struct {
	// log is the associated logger.
	log logger.ComponentLogger
	// httpClient is the HTTP client to use for model pulls.
	httpClient *http.Client
	// pullTokens is a semaphore used to restrict the maximum number of
	// concurrent pull requests.
	pullTokens chan struct{}
	// router is the HTTP request router.
	router *http.ServeMux
	// distributionClient is the client for model distribution.
	distributionClient *distribution.Client
}

// NewManager creates a new model's manager.
func NewManager(log logger.ComponentLogger, httpClient *http.Client, distributionClient *distribution.Client) *Manager {
	// Create the manager.
	m := &Manager{
		log:                log,
		httpClient:         httpClient,
		pullTokens:         make(chan struct{}, maximumConcurrentModelPulls),
		router:             http.NewServeMux(),
		distributionClient: distributionClient,
	}

	// Register routes.
	m.router.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	m.router.HandleFunc("POST /ml/models/create", m.handleCreateModel)
	m.router.HandleFunc("GET /ml/models/json", m.handleGetModels)
	m.router.HandleFunc("GET /ml/models/{namespace}/{name}/json", m.handleGetModel)
	m.router.HandleFunc("DELETE /ml/models/{namespace}/{name}", m.handleDeleteModel)
	m.router.HandleFunc("GET /ml/{backend}/v1/models", m.handleOpenAIGetModels)
	m.router.HandleFunc("GET /ml/{backend}/v1/models/{namespace}/{name}", m.handleOpenAIGetModel)
	m.router.HandleFunc("GET /ml/v1/models", m.handleOpenAIGetModels)
	m.router.HandleFunc("GET /ml/v1/models/{namespace}/{name}", m.handleOpenAIGetModel)

	// Populate the pull concurrency semaphore.
	for i := 0; i < maximumConcurrentModelPulls; i++ {
		m.pullTokens <- struct{}{}
	}

	// Manager successfully initialized.
	return m
}

// handleCreateModel handles POST /ml/models/create requests.
func (m *Manager) handleCreateModel(w http.ResponseWriter, r *http.Request) {
	// Decode the request.
	var request ModelCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Pull the model. In the future, we may support additional operations here
	// besides pulling (such as model building).
	if err := m.PullModel(r.Context(), request.From); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// handleGetModels handles GET /ml/models/json requests.
func (m *Manager) handleGetModels(w http.ResponseWriter, r *http.Request) {
	// Query models.
	models, err := m.getModels("")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write the response.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(models); err != nil {
		m.log.Warnln("Error while encoding model listing response:", err)
	}
}

// handleGetModel handles GET /ml/models/{namespace}/{name}/json requests.
func (m *Manager) handleGetModel(w http.ResponseWriter, r *http.Request) {
	// Query the model.
	model, err := m.GetModel(r.PathValue("namespace") + "/" + r.PathValue("name"))
	if err != nil {
		if errors.Is(err, ErrModelNotFound) || errors.Is(err, distribution.ErrModelNotFound) { //TODO we should fix different types
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Write the response.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(model); err != nil {
		m.log.Warnln("Error while encoding model response:", err)
	}
}

// handleDeleteModel handles DELETE /ml/models/{namespace}/{name} requests.
func (m *Manager) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	// TODO: We probably want the manager to have a lock / unlock mechanism for
	// models so that active runners can retain / release a model, analogous to
	// a container blocking the release of an image. However, unlike containers,
	// runners are only evicted when idle or when memory is needed, so users
	// won't be able to release the images manually. Perhaps we can unlink the
	// corresponding GGUF files from disk and allow the OS to clean them up once
	// the runner process exits (though this won't work for Windows, where we
	// might need some separate cleanup process).

	err := m.distributionClient.DeleteModel(r.PathValue("namespace") + "/" + r.PathValue("name"))
	if err != nil {
		m.log.Warnln("Error while deleting model:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleOpenAIGetModels handles GET /ml/{backend}/v1/models and
// GET /ml/v1/models requests.
func (m *Manager) handleOpenAIGetModels(w http.ResponseWriter, r *http.Request) {
	// Query models.
	models, err := m.getModels("")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Write the response.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(models.toOpenAI()); err != nil {
		m.log.Warnln("Error while encoding OpenAI model listing response:", err)
	}
}

// handleOpenAIGetModel handles GET /ml/{backend}/v1/models/{namespace}/{name}
// and GET /ml/v1/models/{namespace}/{name} requests.
func (m *Manager) handleOpenAIGetModel(w http.ResponseWriter, r *http.Request) {
	// Query the model.
	model, err := m.GetModel(r.PathValue("namespace") + "/" + r.PathValue("name"))
	if err != nil {
		if errors.Is(err, ErrModelNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Write the response.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(model.toOpenAI()); err != nil {
		m.log.Warnln("Error while encoding OpenAI model response:", err)
	}
}

// ServeHTTP implement net/http.Handler.ServeHTTP.
func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.router.ServeHTTP(w, r)
}

// getModels returns a list of all models or (if ref is a non-empty string) a
// list containing only a specific model. If no models exist or the specific
// model can't be found, then an empty (but non-nil) list is returned. Any error
// it returns is suitable for writing back to the client.
func (m *Manager) getModels(ref string) (ModelList, error) {
	// Initialize the model list. We always want to return a non-nil list (even
	// if it's empty) so that it can be encoded directly to JSON.
	models := make(ModelList, 0)

	if ref != "" {
		model, err := m.distributionClient.GetModel(ref)
		if err != nil {
			return nil, err
		}
		models = append(models, &Model{
			ID:      model.ID,
			Tags:    model.Tags,
			Files:   model.Files,
			Created: model.Created,
		})
		return models, nil
	}

	// Get all models from the distribution client
	available, err := m.distributionClient.ListModels()
	if err != nil {
		return nil, fmt.Errorf("error while listing models: %w", err)
	}

	// Convert distribution models to our model format
	for _, current := range available {
		models = append(models, &Model{
			ID:      current.ID,
			Tags:    current.Tags,
			Files:   current.Files,
			Created: current.Created,
		})
	}

	return models, nil
}

// GetModel looks up and returns a single model. It returns ErrModelNotFound if
// the model could not be located.
func (m *Manager) GetModel(ref string) (*Model, error) {
	models, err := m.getModels(ref)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, ErrModelNotFound
	}
	return models[0], nil
}

func (m *Manager) GetModelPath(ref string) (string, error) {
	model, err := m.GetModel(ref)
	if err != nil {
		return "", err
	}
	// TODO: Handle multiple files
	// Convert <algorithm>:<digest> to <algorithm>/<digest>
	blobName := model.Files[0]
	parts := strings.Split(blobName, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid blob format: %s", blobName)
	}
	return fmt.Sprintf("%s/blobs/%s/%s", m.distributionClient.GetStorePath(), parts[0], parts[1]), nil
}
