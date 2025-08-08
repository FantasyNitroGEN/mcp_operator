package main

import (
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/controllers"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/services"
	mcpwebhook "github.com/FantasyNitroGEN/mcp_operator/pkg/webhook"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(mcpv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var webhookPort int
	var enableWebhooks bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "The port the webhook server binds to.")
	flag.BoolVar(&enableWebhooks, "enable-webhooks", false, "Enable admission webhooks.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgrOptions := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "mcp-operator.mcp.allbeone.io",
	}

	// Configure webhook server if webhooks are enabled
	if enableWebhooks {
		mgrOptions.WebhookServer = webhook.NewServer(webhook.Options{
			Port: webhookPort,
		})
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Initialize services for MCPServerReconciler
	// Create service instances with proper dependencies
	kubernetesClient := services.NewDefaultKubernetesClientService(mgr.GetClient())
	resourceBuilder := services.NewDefaultResourceBuilderService()
	registryService := services.NewDefaultRegistryService()
	statusService := services.NewDefaultStatusService(kubernetesClient)
	validationService := services.NewDefaultValidationService(kubernetesClient)
	retryService := services.NewDefaultRetryService()
	eventService := services.NewDefaultEventService(mgr.GetEventRecorderFor("mcp-operator"))
	deploymentService := services.NewDefaultDeploymentService(mgr.GetClient(), resourceBuilder, kubernetesClient)
	autoUpdateService := services.NewDefaultAutoUpdateService(mgr.GetClient(), registryService, statusService, eventService)
	cacheService := services.NewDefaultCacheService()

	if err = (&controllers.MCPServerReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		RegistryService:   registryService,
		DeploymentService: deploymentService,
		StatusService:     statusService,
		ValidationService: validationService,
		RetryService:      retryService,
		EventService:      eventService,
		AutoUpdateService: autoUpdateService,
		CacheService:      cacheService,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MCPServer")
		os.Exit(1)
	}

	// Setup webhooks if enabled
	if enableWebhooks {
		if err = mcpwebhook.SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to setup webhooks")
			os.Exit(1)
		}
		setupLog.Info("webhooks enabled")
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
