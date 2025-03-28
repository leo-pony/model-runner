package llamacpp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logger"
	"github.com/docker/model-runner/pkg/paths"
)

const (
	// Name is the backend name.
	Name = "llama.cpp"
	// componentName is the component name.
	componentName = "inference-" + Name
)

var (
	// log is the log for the backend service.
	log = logger.Default.WithComponent(componentName)
	// serveLog is the log for llamaCppProcess
	serveLog = logger.MakeFileOnly("", componentName)
)

// llamaCpp is the llama.cpp-based backend implementation.
type llamaCpp struct {
	// modelManager is the shared model manager.
	modelManager    *models.Manager
	updatedLlamaCpp bool
}

// New creates a new llama.cpp-based backend.
func New(modelManager *models.Manager) (inference.Backend, error) {
	return &llamaCpp{modelManager: modelManager}, nil
}

// Name implements inference.Backend.Name.
func (l *llamaCpp) Name() string {
	return Name
}

// UsesExternalModelManagement implements
// inference.Backend.UsesExternalModelManagement.
func (l *llamaCpp) UsesExternalModelManagement() bool {
	return false
}

// Install implements inference.Backend.Install.
func (l *llamaCpp) Install(ctx context.Context, httpClient *http.Client) error {
	// We don't currently support this backend on Windows or Linux. We'll likely
	// never support it on Intel Macs.
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		return errors.New("not implemented")
	} else if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
		return errors.New("platform not supported")
	}

	// Temporary workaround for dynamically downloading llama.cpp from Docker Hub.
	// Internet access and an available docker/docker-model-backend-llamacpp:latest-update on Docker Hub are required.
	// Even if docker/docker-model-backend-llamacpp:latest-update has been downloaded before, we still require its
	// digest to be equal to the one on Docker Hub.
	llamaCppPath := paths.DockerHome("bin", "inference", "com.docker.llama-server")
	if err := ensureLatestLlamaCpp(ctx, httpClient, llamaCppPath); err != nil {
		log.Infof("failed to ensure latest llama.cpp: %v\n", err)
		if errors.Is(err, context.Canceled) {
			return err
		}
	} else {
		l.updatedLlamaCpp = true
	}

	return nil
}

// Run implements inference.Backend.Run.
func (l *llamaCpp) Run(ctx context.Context, socket, model string, mode inference.BackendMode) error {
	modelPath, err := l.modelManager.GetModelPath(model)
	log.Infof("Model path: %s", modelPath)
	if err != nil {
		return fmt.Errorf("failed to get model path: %w", err)
	}

	if err := os.RemoveAll(socket); err != nil {
		log.Warnln("failed to remove socket file %s: %w", socket, err)
		log.Warnln("llama.cpp may not be able to start")
	}

	binPath := paths.DockerHome("bin", "inference")
	if !l.updatedLlamaCpp {
		binPath, err = paths.InstallPaths.BinResourcesPath()
		if err != nil {
			return fmt.Errorf("failed to get llama.cpp path: %w", err)
		}
	}
	llamaCppArgs := []string{"--model", modelPath, "--jinja"}
	if mode == inference.BackendModeEmbedding {
		llamaCppArgs = append(llamaCppArgs, "--embeddings")
	}
	llamaCppProcess := exec.CommandContext(
		ctx,
		filepath.Join(binPath, "com.docker.llama-server"),
		llamaCppArgs...,
	)
	llamaCppProcess.Env = append(os.Environ(),
		"DD_INF_UDS="+socket,
	)
	llamaCppProcess.Cancel = func() error {
		// TODO: Figure out the correct process to send on Windows if/when we
		// port this backend there.
		return llamaCppProcess.Process.Signal(os.Interrupt)
	}
	serveLogStream := serveLog.Writer()
	llamaCppProcess.Stdout = serveLogStream
	llamaCppProcess.Stderr = serveLogStream

	if err := llamaCppProcess.Start(); err != nil {
		return fmt.Errorf("unable to start llama.cpp: %w", err)
	}

	llamaCppErrors := make(chan error, 1)
	go func() {
		llamaCppErr := llamaCppProcess.Wait()
		serveLogStream.Close()
		llamaCppErrors <- llamaCppErr
		close(llamaCppErrors)
	}()
	defer func() {
		<-llamaCppErrors
	}()

	select {
	case <-ctx.Done():
		return nil
	case llamaCppErr := <-llamaCppErrors:
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		return fmt.Errorf("llama.cpp terminated unexpectedly: %w", llamaCppErr)
	}
}
