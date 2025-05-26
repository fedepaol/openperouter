package webhooks

import (
	"net/http"

	ctrl "sigs.k8s.io/controller-runtime"
)

const healthPath = "/healthz"

func SetupHealth(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register(
		healthPath,
		&healthHandler{})

	return nil
}

type healthHandler struct{}

func (h *healthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(`{"status": "ok"}`))
	if err != nil {
		Logger.Error("webhook", "healthcheck", "error when writing reply", err)
	}
}
