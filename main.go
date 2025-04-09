package main

import (
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/docker/model-distribution/pkg/distribution"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/sirupsen/logrus"
)

var log = logrus.New()

func main() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home directory: %v", err)
	}
	distributionClient, err := distribution.NewClient(
		distribution.WithStoreRootPath(filepath.Join(userHomeDir, ".docker", "models")),
	)
	if err != nil {
		log.Fatalf("Failed to create distribution client: %v", err)
	}

	modelManager := models.NewManager(log, distributionClient)

	router := http.NewServeMux()
	for _, route := range modelManager.GetRoutes() {
		router.Handle(route, modelManager)
	}

	sockName := "model-runner.sock"
	if err := os.Remove(sockName); err != nil {
		if !os.IsNotExist(err) {
			log.Fatalf("Failed to remove existing socket: %v", err)
		}
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sockName, Net: "unix"})
	if err != nil {
		log.Fatalf("Failed to listen on socket: %v", err)
	}
	defer os.Remove(sockName)

	server := &http.Server{Handler: router}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(ln)
	}()
	defer server.Close()

	select {
	case err := <-serverErrors:
		if err != nil {
			log.Errorf("Server error: %v", err)
		}
	case <-stop:
		log.Infoln("Shutdown signal received")
	}
	log.Infoln("Server stopped")
}
