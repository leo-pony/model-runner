package models

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gomlx/go-huggingface/hub"

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
	// cacheDir is the model storage directory.
	cacheDir string
	// pullTokens is a semaphore used to restrict the maximum number of
	// concurrent pull requests.
	pullTokens chan struct{}
	// router is the HTTP request router.
	router *http.ServeMux
}

// NewManager creates a new models manager.
func NewManager(log logger.ComponentLogger, httpClient *http.Client) *Manager {
	// Create the manager.
	m := &Manager{
		log:        log,
		httpClient: httpClient,
		cacheDir:   hub.DefaultCacheDir(),
		pullTokens: make(chan struct{}, maximumConcurrentModelPulls),
		router:     http.NewServeMux(),
	}

	// Register routes.
	m.router.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	m.router.HandleFunc("POST /ml/models/create", m.handleCreateModel)
	m.router.HandleFunc("GET /ml/models/json", m.handleGetModels)
	m.router.HandleFunc("GET /ml/models/{namespace}/{name}/json", m.handleGetModel)
	m.router.HandleFunc("GET /ml/{backend}/v1/models", m.handleOpenAIGetModels)
	m.router.HandleFunc("GET /ml/{backend}/v1/models/{namespace}/{name}", m.handleOpenAIGetModel)

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

	// Pull the model. In the future, we may support aditional operations here
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

// handleGetModel handles GET /ml/models/{name}/json requests.
func (m *Manager) handleGetModel(w http.ResponseWriter, r *http.Request) {
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
	if err := json.NewEncoder(w).Encode(model); err != nil {
		m.log.Warnln("Error while encoding model response:", err)
	}
}

// handleOpenAIGetModels handles GET /ml/{backend}/v1/models requests.
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

// handleOpenAIGetModel handles GET /ml/{backend}/v1/models/{name} requests.
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

// PullModel pulls a model to local storage. Any error it returns is suitable
// for writing back to the client.
func (m *Manager) PullModel(ctx context.Context, model string) error {
	// Restrict model pull concurrency.
	// TODO: We may want something more sophisticated here, but it will be
	// clearer once we've switched to Docker Hub hosting.
	select {
	case <-m.pullTokens:
	case <-ctx.Done():
		return context.Canceled
	}
	defer func() {
		m.pullTokens <- struct{}{}
	}()

	// Query the model on Hugging Face.
	// TODO: Replace github.com/gomlx/go-huggingface/hub or capture stdout
	// because it doesn't accept a logger.
	// TODO: Use the systemproxy HTTP client (m.httpClient).
	repo := hub.New(model).WithCacheDir(m.cacheDir)
	var ggufFiles []string
	for fileName, err := range repo.IterFileNames() {
		if err != nil {
			return fmt.Errorf("error while enumerating remote files: %w", err)
		}
		if strings.HasSuffix(fileName, ".gguf") {
			ggufFiles = append(ggufFiles, fileName)
		}
	}

	// Download the relevant GGUF files from Hugging Face.
	m.log.Infoln("Downloading files:", ggufFiles)
	// TODO: Use the provided context to regulate the download operation.
	// TODO: Use the systemproxy HTTP client (m.httpClient).
	if _, err := repo.DownloadFiles(ggufFiles...); err != nil {
		return fmt.Errorf("error while downloading file(s): %w", err)
	}
	return nil
}

// getModels returns a list of all models or (if model is a non-empty string) a
// list containing only a specific model. If no models exist or the specific
// model can't be found, then an empty (but non-nil) list is returned. Any error
// it returns is suitable for writing back to the client.
func (m *Manager) getModels(model string, getGGUFPath ...any) (ModelList, error) {
	// Initialize the model list. We always want to return a non-nil list (even
	// if it's empty) so that it can be encoded directly to JSON.
	models := make(ModelList, 0)

	// Iterate over model directories and build the model list.
	dirs, err := os.ReadDir(m.cacheDir)
	if err != nil {
		return nil, fmt.Errorf("error while reading cache directory: %w", err)
	}
	for _, dir := range dirs {
		// Verify that the entry is a model directory.
		if !dir.IsDir() || !strings.HasPrefix(dir.Name(), "models--") {
			continue
		}

		// Parse the model name.
		parts := strings.Split(strings.TrimPrefix(dir.Name(), "models--"), "--")
		if len(parts) < 2 {
			continue
		}
		modelID := strings.Join(parts, "/")

		// Verify whether or not we're interested in this model.
		if model != "" && modelID != model {
			continue
		}

		// Compute some sort of fixed globally unique ID for the model.
		// TODO: Once we switch to Docker Hub-hosted models, this will be a
		// content digest of some sort; for now we'll just digest the name.
		modelGUID := fmt.Sprintf("%x", sha256.Sum256([]byte(modelID)))

		// Query the model creation time.
		dirInfo, err := dir.Info()
		if err != nil {
			m.log.Warnln("Error while getting directory info:", err)
			continue
		}
		created := dirInfo.ModTime().Unix()

		// List the model files.
		ggufFiles := make([]string, 0)
		if err := filepath.Walk(filepath.Join(m.cacheDir, dir.Name(), "snapshots"),
			func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !info.IsDir() && strings.HasSuffix(info.Name(), ".gguf") {
					if getGGUFPath != nil {
						ggufFiles = append(ggufFiles, path)
					} else {
						ggufFiles = append(ggufFiles, info.Name())
					}
				}
				return nil
			}); err != nil {
			m.log.Warnln("Error while walking snapshots directory:", err)
			continue
		}

		// Record the model.
		models = append(models, &Model{
			ID:      modelGUID,
			Tags:    []string{modelID},
			Files:   ggufFiles,
			Created: created,
		})
		if model != "" && getGGUFPath != nil {
			return models, nil
		}
	}

	// Success.
	return models, nil
}

// GetModel looks up and returns a single model. It returns ErrModelNotFound if
// the model could not be located.
func (m *Manager) GetModel(model string) (*Model, error) {
	models, err := m.getModels(model)
	if err != nil {
		return nil, err
	} else if len(models) == 0 {
		return nil, ErrModelNotFound
	}
	return models[0], nil
}

func (m *Manager) GetModelPath(model string) (string, error) {
	models, err := m.getModels(model, struct{}{})
	if err != nil {
		return "", err
	} else if len(models) == 0 {
		return "", ErrModelNotFound
	}
	return models[0].Files[0], nil
}
