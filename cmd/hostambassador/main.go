package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/openperouter/openperouter/internal/hostcredentials"
)

type Config struct {
	OutputPath string
	K8sPort    int
	APIServer  string
}

func main() {
	var (
		outputPath = flag.String("output-path", "/shared", "Path to write credentials")
		k8sPort    = flag.Int("k8s-port", 443, "Kubernetes API server port")
		apiServer  = flag.String("api-server", "", "Kubernetes API server address (if empty, will be resolved)")
	)
	flag.Parse()

	config := Config{
		OutputPath: *outputPath,
		K8sPort:    *k8sPort,
		APIServer:  *apiServer,
	}

	slog.Info("Starting hostambassador with configuration", "config", config)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	apiServerURL, err := getAPIServer(config)
	if err != nil {
		slog.Error("failed to get api server url", "error", err)
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			credentials, err := hostcredentials.ReadCredentials(hostcredentials.ServiceAccountDir)
			if err != nil {
				slog.Error("Failed to read credentials", "error", err)
				continue
			}

			if err := hostcredentials.ExportCredentials(credentials, apiServerURL, config.OutputPath); err != nil {
				slog.Error("Failed to export credentials", "error", err)
				continue
			}
		case sig := <-sigChan:
			slog.Info("Received signal, shutting down", "signal", sig)
			return
		}
	}
}

func getAPIServer(config Config) (string, error) {
	if config.APIServer != "" {
		res := "https://" + config.APIServer + ":" + strconv.Itoa(config.K8sPort)
		return res, nil
	}
	res, err := hostcredentials.ApiServerAddress(config.K8sPort)
	if err != nil {
		return "", err
	}

	return res, nil
}
