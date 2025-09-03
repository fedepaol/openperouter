// SPDX-License-Identifier:Apache-2.0

package webhooks

import (
	"log/slog"

	"github.com/openperouter/openperouter/internal/conversion"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	Logger        *slog.Logger
	WebhookClient client.Reader
)

func validateHostSessions() error {
	l3VNIs, err := getL3VNIs()
	if err != nil {
		return err
	}
	l3Passthroughs, err := getL3Passthroughs()
	if err != nil {
		return err
	}
	return conversion.ValidateHostSessions(l3VNIs.Items, l3Passthroughs.Items)
}
