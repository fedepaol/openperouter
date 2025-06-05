/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/cert-controller/pkg/rotator"
	periov1alpha1 "github.com/openperouter/openperouter/api/v1alpha1"
	"github.com/openperouter/openperouter/internal/controller/nodeindex"
	"github.com/openperouter/openperouter/internal/conversion"
	"github.com/openperouter/openperouter/internal/logging"
	"github.com/openperouter/openperouter/internal/webhooks"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(periov1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var (
		nodeName                      string
		namespace                     string
		logLevel                      string
		probeAddr                     string
		enableWebhook                 bool
		webhookPort                   int
		disableCertRotation           bool
		restartOnRotatorSecretRefresh bool
		certDir                       string
		certServiceName               string
	)
	flag.BoolVar(&disableCertRotation, "disable-cert-rotation", false, "disable automatic generation and rotation of webhook TLS certificates/keys")
	flag.BoolVar(&restartOnRotatorSecretRefresh, "restart-on-rotator-secret-refresh", false, "Restart the pod when the rotator refreshes its cert.")
	flag.StringVar(&certDir, "cert-dir", "/tmp/k8s-webhook-server/serving-certs", "The directory where certs are stored")
	flag.StringVar(&certServiceName, "cert-service-name", "openpe-webhook-service", "The service name used to generate the TLS cert's hostname")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":9081", "The address the probe endpoint binds to.")
	flag.StringVar(&nodeName, "nodename", "", "The name of the node the controller runs on")
	flag.StringVar(&namespace, "namespace", "", "The namespace the controller runs in")
	flag.StringVar(&logLevel, "loglevel", "info", "the verbosity of the process")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "the verbosity of the process")
	flag.BoolVar(&enableWebhook, "enablewebhook", true, "enable the webhook server")

	flag.Parse()

	logger, err := logging.New(logLevel)
	if err != nil {
		fmt.Println("unable to init logger", err)
		os.Exit(1)
	}
	ctrl.SetLogger(logr.FromSlogHandler(logger.Handler()))

	/* TODO: to be used for the metrics endpoints while disabiling
	http2
	tlsOpts = append(tlsOpts, func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	})*/

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		Cache:                  cache.Options{},
		WebhookServer: webhook.NewServer(
			webhook.Options{
				Port: webhookPort,
			},
		),
	})

	if err := webhooks.SetupHealth(mgr); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	startListeners := make(chan struct{})
	if enableWebhook && !disableCertRotation {
		setupLog.Info("Starting certs generator")
		if err := setupCertRotation(startListeners, mgr, logger, namespace, certDir, certServiceName, restartOnRotatorSecretRefresh); err != nil {
			setupLog.Error(err, "unable to set up cert rotator")
			os.Exit(1)
		}
	} else {
		close(startListeners)
	}

	signalHandlerContext := ctrl.SetupSignalHandler()
	go func() {
		<-startListeners

		if err = (&nodeindex.NodesReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			LogLevel: logLevel,
			Logger:   logger,
		}).SetupWithManager(signalHandlerContext, mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "NodeReconciler")
			os.Exit(1)
		}
		// +kubebuilder:scaffold:builder

		if enableWebhook {
			setupLog.Info("Starting webhooks")
			err := setupWebhook(mgr, logger)
			if err != nil {
				setupLog.Error(err, "unable to create", "webhooks")
				os.Exit(1)
			}
			return // We currently support only a onlywebhook mode
		}

		setupLog.Info("Starting controllers")

	}()

	setupLog.Info("starting manager")

	if err := mgr.Start(signalHandlerContext); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

const (
	caName         = "cert"
	caOrganization = "openperouter.io" //nolint:gosec
)

var (
	webhookName       = "openpe-validating-webhook-configuration"
	webhookSecretName = "openpe-webhook-server-cert" //#nosec G101
)

func setupCertRotation(notifyFinished chan struct{}, mgr manager.Manager, logger *slog.Logger,
	namespace, certDir, certServiceName string, restartOnSecretRefresh bool) error {
	webhooks := []rotator.WebhookInfo{
		{
			Name: webhookName,
			Type: rotator.Validating,
		},
	}

	logger.Info("setting up cert rotation", "op", "startup")
	err := rotator.AddRotator(mgr, &rotator.CertRotator{
		SecretKey: types.NamespacedName{
			Namespace: namespace,
			Name:      webhookSecretName,
		},
		CertDir:                certDir,
		CAName:                 caName,
		CAOrganization:         caOrganization,
		DNSName:                fmt.Sprintf("%s.%s.svc", certServiceName, namespace),
		IsReady:                notifyFinished,
		Webhooks:               webhooks,
		FieldOwner:             "openpe",
		RestartOnSecretRefresh: restartOnSecretRefresh,
	})
	if err != nil {
		logger.Error("unable to set up cert rotation", "error", err)
		return err
	}
	return nil
}

func setupWebhook(mgr manager.Manager, logger *slog.Logger) error {
	logger.Info("webhooks enabled")

	webhooks.Logger = logger
	webhooks.WebhookClient = mgr.GetAPIReader()
	webhooks.ValidateVNIs = conversion.ValidateVNIs
	webhooks.ValidateUnderlays = conversion.ValidateUnderlays

	if err := webhooks.SetupVNI(mgr); err != nil {
		logger.Error("unable to create the webook", "error", err, "webhook", "VNIs")
		return err
	}
	if err := webhooks.SetupUnderlay(mgr); err != nil {
		logger.Error("unable to create the webook", "error", err, "webhook", "Underlays")
		return err
	}
	return nil
}
