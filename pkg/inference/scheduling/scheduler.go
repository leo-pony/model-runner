package scheduling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	logpkg "log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logger"
	"github.com/docker/model-runner/pkg/paths"
)

const (
	// maximumRequestSize is the maximum OpenAI embedding or completion request
	// size that Scheduler will allow. This should be large enough to encompass
	// any real-world request but also small enough to avoid DoS attacks.
	maximumRequestSize = 10 * 1024 * 1024
	// maximumReadinessRetries is the maximum number of retries that the
	// scheduler will perform when pinging a backend for readiness.
	maximumReadinessRetries = 60
	// readinessRetryInterval is the interval at which the scheduler will retry
	// readiness checks for a backend.
	readinessRetryInterval = 500 * time.Millisecond
	// modelIdleTimeout is the maximum amount of time that a model can sit idle
	// (i.e. without any requests) before being unloaded from memory.
	modelIdleTimeout = 5 * time.Minute
)

// backendInstallStatus tracks the installation status for a backend. Only one
// of its channels will be closed by the installation loop.
type backendInstallStatus struct {
	// done is closed if the backend installation succeeded.
	done chan struct{}
	// failed is closed if the backend installation failed.
	failed chan struct{}
}

// schedulingRequest is used to transmit a request to the scheduler and wait for
// its completion.
type schedulingRequest struct {
	// backend is the requested inference backend name.
	backend string
	// model is the requested model name.
	model string
	// r is the underlying HTTP request.
	r *http.Request
	// w is the HTTP response writer.
	w http.ResponseWriter
	// done is closed by the scheduling loop once the request has been serviced.
	done chan<- struct{}
}

// Scheduler is used to coordinate inference scheduling across multiple backends
// and models.
type Scheduler struct {
	// log is the associated logger.
	log logger.ComponentLogger
	// backends are the supported inference backends.
	backends map[string]inference.Backend
	// backendsInstalled maps backend names to a structure that tracks their
	// installation status.
	backendsInstalled map[string]*backendInstallStatus
	// modelManager is the shared model manager.
	modelManager *models.Manager
	// httpClient is the HTTP client to use for backend installations.
	httpClient *http.Client
	// queue is the scheduling request queue.
	queue chan *schedulingRequest
	// router is the HTTP request router.
	router *http.ServeMux
}

// NewScheduler creates a new inference scheduler.
func NewScheduler(
	log logger.ComponentLogger,
	backends map[string]inference.Backend,
	modelManager *models.Manager,
	httpClient *http.Client,
) *Scheduler {
	// Create backend installation status trackers.
	backendsInstalled := make(map[string]*backendInstallStatus, len(backends))
	for name := range backends {
		backendsInstalled[name] = &backendInstallStatus{
			failed: make(chan struct{}),
			done:   make(chan struct{}),
		}
	}

	// Create the scheduler.
	s := &Scheduler{
		log:               log,
		backends:          backends,
		backendsInstalled: backendsInstalled,
		modelManager:      modelManager,
		httpClient:        httpClient,
		queue:             make(chan *schedulingRequest),
		router:            http.NewServeMux(),
	}

	// Register routes.
	s.router.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	s.router.HandleFunc("POST /ml/{backend}/v1/chat/completions", s.handleCompletionOrEmbedding)
	s.router.HandleFunc("POST /ml/{backend}/v1/completions", s.handleCompletionOrEmbedding)
	s.router.HandleFunc("POST /ml/{backend}/v1/embeddings", s.handleCompletionOrEmbedding)

	// Scheduler successfully initialized.
	return s
}

// Start starts scheduler operation. It is idempotent.
func (s *Scheduler) Start() {
	// TODO: Implement.
}

// Stop stops scheduler operation. It is idempotent.
func (s *Scheduler) Stop() {
	// TODO: Implement.
}

// Run is the scheduler's main run loop. It starts two sub-loops: one that
// drives backend installation and one that performs scheduling.
func (s *Scheduler) Run(ctx context.Context) {
	// Track subloops.
	var loops sync.WaitGroup
	loops.Add(2)

	// Start the installer loop.
	go func() {
		s.install(ctx)
		loops.Done()
	}()

	// Start the scheduling loop.
	go func() {
		s.schedule(ctx)
		loops.Done()
	}()

	// Wait for both loops to exit.
	loops.Wait()
}

// install loops over backends and ensures that they're installed.
// TODO: This method doesn't currently retry installs; we probably want to add a
// backoff + retry mechanism.
// TODO: This method currently tries to install all known backends. We may wish
// to add granular, per-backend settings. For now, with llama.cpp as our
// ubiquitous backend and mlx as a relatively lightweight backend on macOS only,
// this granularity is probably less of a concern.
func (s *Scheduler) install(ctx context.Context) {
	for name, backend := range s.backends {
		installStatus := s.backendsInstalled[name]
		if err := backend.Install(ctx, s.httpClient); err != nil {
			s.log.Warnf("Backend installation failed for %s: %v", name, err)
			close(installStatus.failed)
		} else {
			close(installStatus.done)
		}
	}
}

// schedule is the inference scheduling loop. It currently only allows a single
// backend / model pair to be running at once, and only a single request for
// that backend / model pair to run at once, though we will extend its logic
// with more sophisticated scheduling once we better understand backend
// performance.
func (s *Scheduler) schedule(ctx context.Context) {
	// TODO: The best way to extend the scheduling logic will be to define the
	// Backend.Run method to be safe for concurrent execution, extract this
	// running logic to a separate type (called runner) and create dynamic
	// backend communication sockets for each backend / model pair.

	// Track the current backend / model combination, the cancellation function
	// regulating the execution, and the channel indicating that the backend's
	// Run method has returned.
	var backend, model string
	var backendCancel context.CancelFunc
	var backendDone chan struct{}

	// Create a utility function to unload any active backend / model.
	unloadAnyActiveBackend := func() {
		if backendCancel != nil {
			backendCancel()
			<-backendDone
			backend = ""
			model = ""
			backendCancel = nil
			backendDone = nil
		}
	}

	// Defer active backend shutdown.
	defer unloadAnyActiveBackend()

	// Create a dialer / transport that dynamically target the active backend.
	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			socket := paths.HostServiceSockets().InferenceBackend(backend)
			return dialer.DialContext(ctx, "unix", socket)
		},
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Create a client that we can use internally to ping the backend.
	client := &http.Client{Transport: transport}
	defer client.CloseIdleConnections()

	// Create a reverse proxy to target the active backend. The virtual URL that
	// we use here is merely a placeholder; the transport always dials the
	// active backend HTTP endpoint and the hostname is always overwritten in
	// our reverse proxy. This URL is not accessible from anywhere.
	upstream, err := url.Parse("http://inference.docker.internal")
	if err != nil {
		s.log.Errorf("Unable to parse virtual backend URL: %v", err)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	standardDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		standardDirector(r)
		// HACK: Most backends will be happier with a "localhost" hostname than
		// an "inference.docker.internal" hostname (which they may reject).
		r.Host = "localhost"
		// Remove the prefix up to the OpenAI API root.
		r.URL.Path = trimRequestPathToOpenAIRoot(r.URL.Path)
		r.URL.RawPath = trimRequestPathToOpenAIRoot(r.URL.RawPath)
	}
	proxy.Transport = transport
	logStream := s.log.Writer()
	defer logStream.Close()
	proxy.ErrorLog = logpkg.New(logStream, "", 0)

	// Create a timer (initially stopped / drained) to unload backends after
	// some period of inactivity.
	idleTimer := time.NewTimer(modelIdleTimeout)
	defer idleTimer.Stop()
	if !idleTimer.Stop() {
		<-idleTimer.C
	}

	// Handle scheduling requests until termination.
	for {
		// Wait for a scheduling request or cancellation.
		var request *schedulingRequest
		select {
		case request = <-s.queue:
		case <-idleTimer.C:
			s.log.Infof("Unloading idle backend / model: %s / %s", backend, model)
			unloadAnyActiveBackend()
			continue
		case <-ctx.Done():
			return
		}

		// Clear the idle timer. We don't know its state (i.e. running, stopped,
		// or expired), so we have to drain it conservatively.
		idleTimer.Stop()
		select {
		case <-idleTimer.C:
		default:
		}

		// Load the required backend / model.
		if request.backend == backend && request.model == model {
			// This is the happy and most common path; there's no need to unload
			// or load anything.
		} else {
			// Close any idle connections to the active backend from our client.
			client.CloseIdleConnections()

			// Unload the active backend, if any.
			unloadAnyActiveBackend()

			// Start the new backend. We validate the backend in
			// handleCompletionOrEmbedding and ensure that it is installed
			// before dispatching requests to schedule, so we can be sure that
			// it exists here.
			backend = request.backend
			model = request.model
			var backendCtx context.Context
			backendCtx, backendCancel = context.WithCancel(ctx)
			backendDone = make(chan struct{})
			go func() {
				s.backends[backend].Run(
					backendCtx,
					paths.HostServiceSockets().InferenceBackend(backend),
					model,
				)
				close(backendDone)
			}()

			// Wait for the OpenAI model listing endpoint to respond as an
			// indication that the backend is up.
			var ready bool
			for i := 0; i < maximumReadinessRetries; i++ {
				readyRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/v1/models", http.NoBody)
				if err != nil {
					s.log.Errorf("Readiness request creation failed: %v", err)
					return
				}
				response, err := client.Do(readyRequest)
				if err == nil {
					response.Body.Close()
				}
				if err != nil || response.StatusCode != http.StatusOK {
					select {
					case <-time.After(readinessRetryInterval):
						continue
					case <-ctx.Done():
						http.Error(request.w, "inference service is shutting down", http.StatusServiceUnavailable)
						close(request.done)
						return
					}
				}
				ready = true
				break
			}
			if !ready {
				http.Error(request.w, "backend took too long to initialize", http.StatusServiceUnavailable)
				close(request.done)
				unloadAnyActiveBackend()
				continue
			}
		}

		// TODO: Adjust the request to be regulated by schedule's context (in
		// addition to its default context). We will need a utility function for
		// this purpose.

		// Proxy the request to the backend and signal completion.
		proxy.ServeHTTP(request.w, request.r)
		close(request.done)

		// Reset the idle timeout.
		idleTimer.Reset(modelIdleTimeout)
	}
}

// handleCompletionOrEmbedding handles scheduling both
// POST /ml/{backend}/v1/chat/completions and POST /ml/{backend}/v1/embeddings
// requests.
func (s *Scheduler) handleCompletionOrEmbedding(w http.ResponseWriter, r *http.Request) {
	// Determine the requested backend and ensure that it's valid.
	backend, ok := s.backends[r.PathValue("backend")]
	if !ok {
		http.Error(w, "backend not found", http.StatusNotFound)
		return
	}

	// Read the entire request body. We put some basic size constraints in place
	// to avoid DoS attacks. We do this early to avoid client write timeouts.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximumRequestSize))
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			http.Error(w, "request too large", http.StatusBadRequest)
		} else {
			http.Error(w, "unknown error", http.StatusInternalServerError)
		}
		return
	}

	// Wait for the corresponding backend installation to complete or fail. We
	// don't allow any requests to be scheduled for a backend until it has
	// completed installation.
	installStatus := s.backendsInstalled[backend.Name()]
	select {
	case <-installStatus.done:
	case <-installStatus.failed:
		http.Error(w, "backend installation failed", http.StatusInternalServerError)
		return
	case <-r.Context().Done():
		return
	}

	// Decode the model specification portion of the request body.
	var request completionOrEmbeddingRequest
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if request.Model == "" {
		http.Error(w, "model is required", http.StatusBadRequest)
		return
	}

	// Check if the model manager already has the requested model available. If
	// not, perform a pull operation before scheduling the request.
	// TODO: We may wish to make this behavior configurable, with an option to
	// instead return a 404 if the model is unavailable.
	if !backend.UsesExternalModelManagement() {
		if _, err := s.modelManager.GetModel(request.Model); err != nil {
			if errors.Is(err, models.ErrModelNotFound) {
				if err = s.modelManager.PullModel(r.Context(), request.Model); err != nil {
					http.Error(w, fmt.Errorf("unable to pull model: %w", err).Error(), http.StatusInternalServerError)
					return
				}
			} else {
				http.Error(w, "model unavailable", http.StatusInternalServerError)
				return
			}
		}
	}

	// Create a request with the body replaced for forwarding upstream.
	upstreamRequest := r.Clone(r.Context())
	upstreamRequest.Body = io.NopCloser(bytes.NewReader(body))

	// Submit the request for scheduling.
	done := make(chan struct{})
	scheduleRequest := &schedulingRequest{
		backend: backend.Name(),
		model:   request.Model,
		r:       upstreamRequest,
		w:       w,
		done:    done,
	}
	select {
	case s.queue <- scheduleRequest:
	case <-r.Context().Done():
		return
	}

	// Wait for the request to complete.
	select {
	case <-done:
	case <-r.Context().Done():
	}
}

// ServeHTTP implements net/http.Handler.ServeHTTP.
func (s *Scheduler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO: Add some enablement/disablement mechanism similar regulated by
	// Start / Stop.

	// Dispatch the request accordingly.
	s.router.ServeHTTP(w, r)
}
