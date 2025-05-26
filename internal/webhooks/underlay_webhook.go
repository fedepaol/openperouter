// SPDX-License-Identifier:Apache-2.0

package webhooks

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/openperouter/openperouter/api/v1alpha1"
	v1 "k8s.io/api/admission/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	underlayValidationWebhookPath = "/validate-openperouter-io-v1alpha1-underlay"
)

var ValidateUnderlays func(underlays []v1alpha1.Underlay) error

type UnderlayValidator struct {
	client  client.Client
	decoder admission.Decoder
}

func SetupUnderlay(mgr ctrl.Manager) error {
	validator := &UnderlayValidator{
		client:  mgr.GetClient(),
		decoder: admission.NewDecoder(mgr.GetScheme()),
	}

	mgr.GetWebhookServer().Register(
		vniValidationWebhookPath,
		&webhook.Admission{Handler: validator})

	return nil
}

//+kubebuilder:webhook:verbs=create;update,path=/validate-openperouter-io-v1alpha1-underlay,mutating=false,failurePolicy=fail,groups=openperouter.io,resources=underlays,versions=v1alpha1,name=underlayvalidationwebhook.openperouter.io,sideEffects=None,admissionReviewVersions=v1

func (v *UnderlayValidator) Handle(ctx context.Context, req admission.Request) (resp admission.Response) {
	var underlay v1alpha1.Underlay
	var oldUnderlay v1alpha1.Underlay
	if req.Operation == v1.Delete {
		if err := v.decoder.DecodeRaw(req.OldObject, &underlay); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	} else {
		if err := v.decoder.Decode(req, &underlay); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		if req.OldObject.Size() > 0 {
			if err := v.decoder.DecodeRaw(req.OldObject, &oldUnderlay); err != nil {
				return admission.Errored(http.StatusBadRequest, err)
			}
		}
	}

	var warnings []string
	switch req.Operation {
	case v1.Create:
		w, err := validateUnderlayCreate(&underlay)
		if err != nil {
			return admission.Denied(err.Error())
		}
		warnings = w
	case v1.Update:
		w, err := validateUnderlayUpdate(&underlay)
		if err != nil {
			return admission.Denied(err.Error())
		}
		warnings = w
	case v1.Delete:
		w, err := validateUnderlayDelete(&underlay)
		if err != nil {
			return admission.Denied(err.Error())
		}
		warnings = w
	}
	return admission.Allowed("").WithWarnings(warnings...)
}

func validateUnderlayCreate(underlay *v1alpha1.Underlay) ([]string, error) {
	Logger.Debug("webhook underlay", "action", "create", "name", underlay.Name, "namespace", underlay.Namespace)
	defer Logger.Debug("webhook underlay", "action", "end create", "name", underlay.Name, "namespace", underlay.Namespace)

	return validateUnderlay(underlay)
}

func validateUnderlayUpdate(underlay *v1alpha1.Underlay) ([]string, error) {
	Logger.Debug("webhook underlay", "action", "update", "name", underlay.Name, "namespace", underlay.Namespace)
	defer Logger.Debug("webhook underlay", "action", "end update", "name", underlay.Name, "namespace", underlay.Namespace)

	return validateUnderlay(underlay)
}

func validateUnderlayDelete(_ *v1alpha1.Underlay) ([]string, error) {
	return []string{}, nil
}

func validateUnderlay(vni *v1alpha1.Underlay) ([]string, error) {
	var warnings []string

	existingUnderlays, err := getUnderlays()
	if err != nil {
		return warnings, err
	}

	if err := ValidateUnderlays(existingUnderlays.Items); err != nil {
		return warnings, fmt.Errorf("validation failed: %w", err)
	}
	return warnings, nil
}

var getUnderlays = func() (*v1alpha1.UnderlayList, error) {
	vniList := &v1alpha1.UnderlayList{}
	err := WebhookClient.List(context.Background(), vniList, &client.ListOptions{})
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to get existing FRRConfiguration objects"))
	}
	return vniList, nil
}
