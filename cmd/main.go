package main

import (
	"flag"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/iqbalhakims/pv-operator/internal/controller"
	"github.com/iqbalhakims/pv-operator/internal/guard"
)

func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func main() {
	var (
		metricsAddr, probeAddr, certDir string
		allowedUsers, allowedGroups     string
		protectPVCs, enforceRetain      bool
	)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "Metrics endpoint address.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Health probe address.")
	flag.StringVar(&certDir, "webhook-cert-dir", "/tmp/k8s-webhook-server/serving-certs", "Directory with tls.crt/tls.key for the webhook server.")
	flag.StringVar(&allowedUsers, "allowed-users",
		"system:kube-controller-manager,system:serviceaccount:kube-system:persistent-volume-binder",
		"Comma-separated usernames that may delete protected PVs/PVCs.")
	flag.StringVar(&allowedGroups, "allowed-groups", "system:masters",
		"Comma-separated groups that may delete protected PVs/PVCs.")
	flag.BoolVar(&protectPVCs, "protect-pvcs", true,
		"Also block deleting PVCs bound to a PV with reclaimPolicy Delete.")
	flag.BoolVar(&enforceRetain, "enforce-retain", false,
		"Run a controller that switches every PV's reclaimPolicy from Delete to Retain.")
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	log := ctrl.Log.WithName("setup")

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		log.Error(err, "unable to build scheme")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		WebhookServer:          webhook.NewServer(webhook.Options{CertDir: certDir}),
	})
	if err != nil {
		log.Error(err, "unable to create manager")
		os.Exit(1)
	}

	policy := guard.Policy{
		AllowedUsers:  splitList(allowedUsers),
		AllowedGroups: splitList(allowedGroups),
		ProtectPVCs:   protectPVCs,
	}
	mgr.GetWebhookServer().Register("/validate-storage", &webhook.Admission{Handler: &guard.Handler{
		Client:  mgr.GetAPIReader(),
		Decoder: admission.NewDecoder(scheme),
		Policy:  policy,
	}})

	if enforceRetain {
		if err := (&controller.RetainReconciler{Client: mgr.GetClient()}).SetupWithManager(mgr); err != nil {
			log.Error(err, "unable to set up retain controller")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", mgr.GetWebhookServer().StartedChecker()); err != nil {
		log.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	log.Info("starting pv-guard", "policy", policy, "enforceRetain", enforceRetain)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "manager exited")
		os.Exit(1)
	}
}
