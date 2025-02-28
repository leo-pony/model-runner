package llamacpp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/docker/model-runner/pkg/errordef"
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
	// errors is used for generating and wrapping errors.
	errors = errordef.NewHelper(componentName, nil)
	// log is the log for the backend service.
	log = logger.Default.WithComponent(componentName)
	// serveLog is the log for llamaCppProcess
	serveLog = logger.MakeFileOnly("", componentName)
)

// llamaCpp is the llama.cpp-based backend implementation.
type llamaCpp struct {
	// modelManager is the shared model manager.
	modelManager *models.Manager
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
func (l *llamaCpp) Install(_ context.Context, _ *http.Client) error {
	// We don't currently support this backend on Windows or Linux. We'll likely
	// never support it on Intel Macs.
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		return errors.New("not implemented")
	} else if runtime.GOOS == "darwin" && runtime.GOARCH == "amd64" {
		return errors.New("platform not supported")
	}

	// TODO: Add support for dynamic updates on supported platforms.
	return nil
}

// Run implements inference.Backend.Run.
func (l *llamaCpp) Run(ctx context.Context, socket, model string) error {
	modelPath, err := l.modelManager.GetModelPath(model)
	if err != nil {
		return fmt.Errorf("failed to get model path: %w", err)
	}

	if err := os.RemoveAll(socket); err != nil {
		log.Warnln("failed to remove socket file %s: %w", socket, err)
		log.Warnln("llama.cpp may not be able to start")
	}

	binPath, err := paths.InstallPaths.BinResourcesPath()
	if err != nil {
		return fmt.Errorf("failed to get llama.cpp path: %w", err)
	}
	llamaCppProcess := exec.CommandContext(
		ctx,
		filepath.Join(binPath, "com.docker.llama-server"),
		"--model", modelPath,
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
		return errors.Wrap(err, "unable to start llama.cpp")
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
		return errors.Wrap(llamaCppErr, "llama.cpp terminated unexpectedly")
	}
}
