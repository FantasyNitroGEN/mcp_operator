package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	controllers "github.com/FantasyNitroGEN/mcp_operator/controllers"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/services"
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

// mcpRegistryCRDExists checks if the MCPRegistry CRD exists in the cluster
func mcpRegistryCRDExists(ctx context.Context, client client.Client) bool {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := client.Get(ctx, types.NamespacedName{
		Name: "mcpregistries.mcp.io",
	}, crd)
	return err == nil
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var registryURL string
	var maxConcurrentReconcilesMCPServer int
	var maxConcurrentReconcilesMCPRegistry int
	var gogcPercent int

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metric endpoint binds to. "+
		"Use the port :8080. If not set, it will be 0 in order to disable the metrics server")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&registryURL, "registry-url", "",
		"URL of the MCP registry (default: Docker MCP Registry on GitHub)")
	flag.IntVar(&maxConcurrentReconcilesMCPServer, "max-concurrent-reconciles-mcpserver", 5,
		"Maximum number of concurrent reconciles for MCPServer controller")
	flag.IntVar(&maxConcurrentReconcilesMCPRegistry, "max-concurrent-reconciles-mcpregistry", 3,
		"Maximum number of concurrent reconciles for MCPRegistry controller")
	flag.IntVar(&gogcPercent, "gogc-percent", 100,
		"GOGC percentage for Go garbage collector (lower values = more frequent GC)")

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

	// Start pprof server for profiling (only in development)
	go func() {
		setupLog.Info("Starting pprof server on :6060")
		if err := http.ListenAndServe(":6060", nil); err != nil {
			setupLog.Error(err, "Failed to start pprof server")
		}
	}()

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancelation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "mcp-operator.mcp.allbeone.io",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup indexers for efficient object search
	setupLog.Info("Setting up field indexers for efficient object search")

	// Index MCPServer by spec.registry.name for efficient registry-based queries
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &mcpv1.MCPServer{}, "spec.registry.name", func(rawObj client.Object) []string {
		mcpServer := rawObj.(*mcpv1.MCPServer)
		if mcpServer.Spec.Registry.Name == "" {
			return nil
		}
		return []string{mcpServer.Spec.Registry.Name}
	}); err != nil {
		setupLog.Error(err, "unable to create index for MCPServer spec.registry.name")
		os.Exit(1)
	}

	// Index MCPServer by metadata.name for efficient name-based queries
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &mcpv1.MCPServer{}, "metadata.name", func(rawObj client.Object) []string {
		mcpServer := rawObj.(*mcpv1.MCPServer)
		return []string{mcpServer.Name}
	}); err != nil {
		setupLog.Error(err, "unable to create index for MCPServer metadata.name")
		os.Exit(1)
	}

	// Index MCPRegistry by metadata.name for efficient registry name-based queries
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &mcpv1.MCPRegistry{}, "metadata.name", func(rawObj client.Object) []string {
		mcpRegistry := rawObj.(*mcpv1.MCPRegistry)
		return []string{mcpRegistry.Name}
	}); err != nil {
		setupLog.Error(err, "unable to create index for MCPRegistry metadata.name")
		os.Exit(1)
	}

	setupLog.Info("Field indexers configured successfully")

	// Initialize service dependencies for decomposed architecture
	setupLog.Info("Initializing service dependencies")

	// Create event recorder
	eventRecorder := mgr.GetEventRecorderFor("mcp-operator")

	// Initialize core services
	kubernetesClientService := services.NewDefaultKubernetesClientService(mgr.GetClient())
	retryService := services.NewDefaultRetryService()
	eventService := services.NewDefaultEventService(eventRecorder)
	statusService := services.NewDefaultStatusService(kubernetesClientService)
	validationService := services.NewDefaultValidationService(kubernetesClientService)
	resourceBuilderService := services.NewSimpleResourceBuilderService()

	// Initialize cache service for efficient object caching
	cacheService := services.NewDefaultCacheService()

	// Initialize registry service
	registryService := services.NewDefaultRegistryService(mgr.GetClient())

	// Initialize deployment service
	deploymentService := services.NewDefaultDeploymentService(
		mgr.GetClient(),
		resourceBuilderService,
		kubernetesClientService,
	)

	// Initialize auto-update service
	autoUpdateService := services.NewDefaultAutoUpdateService(
		mgr.GetClient(),
		registryService,
		statusService,
		eventService,
	)

	// Setup MCPRegistry Controller with dependency injection (conditional registration)
	mcpRegistryEnabled := os.Getenv("MCP_REGISTRY_CONTROLLER_ENABLED")
	if mcpRegistryEnabled == "true" {
		// Check if MCPRegistry CRD exists
		ctx := context.Background()
		if mcpRegistryCRDExists(ctx, mgr.GetClient()) {
			setupLog.Info("Setting up MCPRegistry controller", "maxConcurrentReconciles", maxConcurrentReconcilesMCPRegistry)
			if err = (&controllers.MCPRegistryReconciler{
				Client:   mgr.GetClient(),
				Scheme:   mgr.GetScheme(),
				Recorder: mgr.GetEventRecorderFor("mcpregistry-controller"),
			}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "MCPRegistry")
				os.Exit(1)
			}
		} else {
			setupLog.Info("MCPRegistry CRD not found, skipping MCPRegistry controller setup")
		}
	} else {
		setupLog.Info("MCPRegistry controller disabled by configuration")
	}

	// Setup MCPServer Controller with dependency injection
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

	// Setup Auto-Update Controller for periodic template synchronization
	setupLog.Info("Setting up Auto-Update controller")
	autoUpdateInterval := 24 * time.Hour // Check for updates daily
	if err = controllers.AddAutoUpdateController(mgr, autoUpdateService, autoUpdateInterval); err != nil {
		setupLog.Error(err, "unable to create auto-update controller")
		os.Exit(1)
	}

	// Start periodic cache cleanup routine
	setupLog.Info("Starting periodic cache cleanup routine")
	go func() {
		ticker := time.NewTicker(30 * time.Minute) // Clean cache every 30 minutes
		defer ticker.Stop()

		for range ticker.C {
			cacheService.ClearExpired(context.Background())
			stats := cacheService.GetStats(context.Background())
			setupLog.V(1).Info("Cache statistics",
				"totalEntries", stats.TotalEntries,
				"expiredEntries", stats.ExpiredEntries,
				"registryHits", stats.RegistryHits,
				"registryMisses", stats.RegistryMisses,
				"mcpServerHits", stats.MCPServerHits,
				"mcpServerMisses", stats.MCPServerMisses,
				"registryServersHits", stats.RegistryServersHits,
				"registryServersMisses", stats.RegistryServersMisses,
			)
		}
	}()

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
