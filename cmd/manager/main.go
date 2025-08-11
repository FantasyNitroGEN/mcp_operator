package main

import (
	"flag"
	"fmt"
	"os"
	"time"

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
	"github.com/FantasyNitroGEN/mcp_operator/pkg/retry"
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
	var maxConcurrentReconcilesMCPServer int
	var maxConcurrentReconcilesMCPRegistry int
	var gogcPercent int

	// GitHub retry configuration flags
	var githubMaxRetries int
	var githubInitialDelay time.Duration
	var githubMaxDelay time.Duration
	var githubRateLimitMaxRetries int
	var githubRateLimitBaseDelay time.Duration
	var githubRateLimitMaxDelay time.Duration
	var githubNetworkMaxRetries int
	var githubNetworkBaseDelay time.Duration
	var githubNetworkMaxDelay time.Duration

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.IntVar(&webhookPort, "webhook-port", 9443, "The port the webhook server binds to.")
	flag.BoolVar(&enableWebhooks, "enable-webhooks", false, "Enable admission webhooks.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&maxConcurrentReconcilesMCPServer, "max-concurrent-reconciles-mcpserver", 5,
		"Maximum number of concurrent reconciles for MCPServer controller")
	flag.IntVar(&maxConcurrentReconcilesMCPRegistry, "max-concurrent-reconciles-mcpregistry", 3,
		"Maximum number of concurrent reconciles for MCPRegistry controller")
	flag.IntVar(&gogcPercent, "gogc-percent", 100,
		"GOGC percentage for Go garbage collector (lower values = more frequent GC)")

	// GitHub retry configuration flags
	flag.IntVar(&githubMaxRetries, "github-max-retries", 5, "Maximum number of retries for general GitHub API errors.")
	flag.DurationVar(&githubInitialDelay, "github-initial-delay", time.Second, "Initial delay before first retry for GitHub API calls.")
	flag.DurationVar(&githubMaxDelay, "github-max-delay", time.Minute*5, "Maximum delay between retries for GitHub API calls.")
	flag.IntVar(&githubRateLimitMaxRetries, "github-rate-limit-max-retries", 3, "Maximum number of retries for GitHub API rate limit errors.")
	flag.DurationVar(&githubRateLimitBaseDelay, "github-rate-limit-base-delay", time.Minute*15, "Base delay for GitHub API rate limit retries.")
	flag.DurationVar(&githubRateLimitMaxDelay, "github-rate-limit-max-delay", time.Hour, "Maximum delay for GitHub API rate limit retries.")
	flag.IntVar(&githubNetworkMaxRetries, "github-network-max-retries", 5, "Maximum number of retries for GitHub API network errors.")
	flag.DurationVar(&githubNetworkBaseDelay, "github-network-base-delay", time.Second*2, "Base delay for GitHub API network error retries.")
	flag.DurationVar(&githubNetworkMaxDelay, "github-network-max-delay", time.Minute*2, "Maximum delay for GitHub API network error retries.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Configure Go runtime optimizations
	if gogcPercent != 100 {
		if err := os.Setenv("GOGC", fmt.Sprintf("%d", gogcPercent)); err != nil {
			setupLog.Error(err, "Failed to set GOGC environment variable", "percentage", gogcPercent)
		} else {
			setupLog.Info("GOGC configured", "percentage", gogcPercent)
		}
	}

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

	// Create GitHub retry configuration from command line flags
	githubRetryConfig := retry.GitHubRetryConfig{
		MaxRetries:   githubMaxRetries,
		InitialDelay: githubInitialDelay,
		MaxDelay:     githubMaxDelay,
		Multiplier:   2.0,
		Jitter:       true,

		RateLimitMaxRetries: githubRateLimitMaxRetries,
		RateLimitBaseDelay:  githubRateLimitBaseDelay,
		RateLimitMaxDelay:   githubRateLimitMaxDelay,

		NetworkMaxRetries: githubNetworkMaxRetries,
		NetworkBaseDelay:  githubNetworkBaseDelay,
		NetworkMaxDelay:   githubNetworkMaxDelay,

		OnRetry: func(attempt int, err error, errorType retry.GitHubErrorType, delay time.Duration) {
			setupLog.Info("GitHub API retry attempt",
				"attempt", attempt,
				"errorType", errorType.String(),
				"delay", delay,
				"error", err.Error())
		},
	}

	registryService := services.NewDefaultRegistryServiceWithRetryConfig(githubRetryConfig)
	statusService := services.NewDefaultStatusService(kubernetesClient)
	validationService := services.NewDefaultValidationService(kubernetesClient)
	retryService := services.NewDefaultRetryService()
	eventService := services.NewDefaultEventService(mgr.GetEventRecorderFor("mcp-operator"))
	deploymentService := services.NewDefaultDeploymentService(mgr.GetClient(), resourceBuilder, kubernetesClient)
	autoUpdateService := services.NewDefaultAutoUpdateService(mgr.GetClient(), registryService, statusService, eventService)
	cacheService := services.NewDefaultCacheService()

	setupLog.Info("Setting up MCPRegistry controller", "maxConcurrentReconciles", maxConcurrentReconcilesMCPRegistry)
	if err = (&controllers.MCPRegistryReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		RegistryService:   registryService,
		StatusService:     statusService,
		ValidationService: validationService,
		RetryService:      retryService,
		EventService:      eventService,
		CacheService:      cacheService,
	}).SetupWithManagerAndConcurrency(mgr, maxConcurrentReconcilesMCPRegistry); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MCPRegistry")
		os.Exit(1)
	}

	setupLog.Info("Setting up MCPServer controller", "maxConcurrentReconciles", maxConcurrentReconcilesMCPServer)
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
	}).SetupWithManagerAndConcurrency(mgr, maxConcurrentReconcilesMCPServer); err != nil {
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
