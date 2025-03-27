// SPDX-License-Identifier:Apache-2.0

package routerconfiguration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/internal/conversion"
	"github.com/openperouter/openperouter/internal/hostnetwork"
	"github.com/openperouter/openperouter/internal/pods"
)

type interfacesConfiguration struct {
	RouterPodUUID string `json:"routerPodUUID,omitempty"`
	PodRuntime    pods.Runtime
	NodeIndex     int                 `json:"nodeIndex,omitempty"`
	Underlays     []v1alpha1.Underlay `json:"underlays,omitempty"`
	Vnis          []v1alpha1.VNI      `json:"vnis,omitempty"`
}

type UnderlayRemovedError struct{}

func (n UnderlayRemovedError) Error() string {
	return "no underlays configured"
}

func configureInterfaces(ctx context.Context, config interfacesConfiguration) error {
	targetNS, err := config.PodRuntime.NetworkNamespace(ctx, config.RouterPodUUID)
	if err != nil {
		return fmt.Errorf("failed to retrieve namespace for pod %s: %w", config.RouterPodUUID, err)
	}

	hasAlreadyUnderlay, err := hostnetwork.HasUnderlayInterface(targetNS)
	if err != nil {
		return fmt.Errorf("failed to check if target namespace %s for pod %s has underlay: %w", targetNS, config.RouterPodUUID, err)
	}
	if hasAlreadyUnderlay && len(config.Underlays) == 0 {
		return UnderlayRemovedError{}
	}

	if len(config.Underlays) == 0 {
		return nil // nothing to do
	}

	slog.InfoContext(ctx, "configure interface start", "namespace", targetNS)
	defer slog.InfoContext(ctx, "configure interface end", "namespace", targetNS)
	underlayParams, vnis, err := conversion.APItoHostConfig(config.NodeIndex, targetNS, config.Underlays, config.Vnis)
	if err != nil {
		return fmt.Errorf("failed to convert config to host configuration: %w", err)
	}

	slog.InfoContext(ctx, "setting up underlay")
	if err := hostnetwork.SetupUnderlay(ctx, underlayParams); err != nil {
		return fmt.Errorf("failed to setup underlay: %w", err)
	}
	for _, vni := range vnis {
		slog.InfoContext(ctx, "setting up VNI", "vni", vni.VRF)
		if err := hostnetwork.SetupVNI(ctx, vni); err != nil {
			return fmt.Errorf("failed to setup vni: %w", err)
		}
	}
	if err := hostnetwork.RemoveNonConfiguredVNIs(targetNS, vnis); err != nil {
		return fmt.Errorf("failed to remove leftover vnis: %w", err)
	}
	return nil
}

// nonRecoverableHostError tells whether the router pod
// should be restarted instead of being reconfigured.
func nonRecoverableHostError(e error) bool {
	if errors.As(e, &UnderlayRemovedError{}) {
		return true
	}
	underlayExistsError := hostnetwork.UnderlayExistsError("")
	return errors.As(e, &underlayExistsError)
}
