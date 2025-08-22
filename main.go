package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/panteparak/vault-autounseal-operator/pkg/controller"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	// SignalBufferSize is the buffer size for signal channel.
	SignalBufferSize = 2
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	// Build-time variables
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

// OperatorConfig holds the configuration for the operator.
type OperatorConfig struct {
	MetricsAddr          string
	ProbeAddr            string
	EnableLeaderElection bool
	ShowVersion          bool
	HealthCheck          bool
	Development          bool
}

// NewOperatorConfig creates a new operator configuration with defaults.
func NewOperatorConfig() *OperatorConfig {
	return &OperatorConfig{
		MetricsAddr:          ":8080",
		ProbeAddr:            ":8081",
		EnableLeaderElection: false,
		Development:          true,
	}
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vaultv1.AddToScheme(scheme))
}

func main() {
	config := NewOperatorConfig()
	parseFlags(config)

	if config.ShowVersion {
		printVersion()
		return
	}

	if config.HealthCheck {
		setupLog.Info("health check passed")
		return
	}

	os.Exit(runMain(config))
}

// runMain runs the main application logic and returns an exit code.
func runMain(config *OperatorConfig) int {
	ctx, cancel := setupSignalHandler()
	defer cancel()

	err := run(ctx, config)
	if err != nil {
		setupLog.Error(err, "operator failed")
		return 1
	}
	return 0
}

// parseFlags configures the operator config from command line flags.
func parseFlags(config *OperatorConfig) {
	flag.StringVar(&config.MetricsAddr, "metrics-bind-address", config.MetricsAddr,
		"The address the metric endpoint binds to.")
	flag.StringVar(&config.ProbeAddr, "health-probe-bind-address", config.ProbeAddr,
		"The address the probe endpoint binds to.")
	flag.BoolVar(&config.EnableLeaderElection, "leader-elect", config.EnableLeaderElection,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&config.ShowVersion, "version", config.ShowVersion, "Show version information and exit.")
	flag.BoolVar(&config.HealthCheck, "health-check", config.HealthCheck, "Perform health check and exit.")
	flag.BoolVar(&config.Development, "development", config.Development, "Enable development mode for logging.")

	opts := zap.Options{
		Development: config.Development,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
}

// printVersion displays version information.
func printVersion() {
	setupLog.Info("version information",
		"version", version,
		"build-time", buildTime,
		"git-commit", gitCommit,
	)
}

// setupSignalHandler creates a context that cancels on SIGTERM/SIGINT.
func setupSignalHandler() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	c := make(chan os.Signal, SignalBufferSize)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		setupLog.Info("received termination signal, shutting down")
		cancel()
	}()

	return ctx, cancel
}

// run starts the operator with the given configuration.
func run(ctx context.Context, config *OperatorConfig) error {
	setupLog.Info("starting vault auto-unseal operator",
		"version", version,
		"build-time", buildTime,
		"git-commit", gitCommit,
		"metrics-addr", config.MetricsAddr,
		"probe-addr", config.ProbeAddr,
		"leader-election", config.EnableLeaderElection,
	)

	kubeConfig, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf(
			"unable to get kubernetes config - ensure operator is running in cluster or has valid kubeconfig: %w",
			err)
	}

	mgr, err := ctrl.NewManager(kubeConfig, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                server.Options{BindAddress: config.MetricsAddr},
		HealthProbeBindAddress: config.ProbeAddr,
		LeaderElection:         config.EnableLeaderElection,
		LeaderElectionID:       "vault-autounseal-operator-leader",
	})
	if err != nil {
		return fmt.Errorf("unable to start manager: %w", err)
	}

	if err := setupControllers(mgr); err != nil {
		return fmt.Errorf("unable to setup controllers: %w", err)
	}

	if err := setupHealthChecks(mgr); err != nil {
		return fmt.Errorf("unable to setup health checks: %w", err)
	}

	setupLog.Info("starting vault auto-unseal operator manager")

	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("manager start failed: %w", err)
	}

	return nil
}

// setupControllers configures all controllers.
func setupControllers(mgr ctrl.Manager) error {
	clientRepository := controller.NewDefaultVaultClientRepository(nil)
	reconcilerOptions := controller.DefaultReconcilerOptions()

	reconciler := controller.NewVaultUnsealConfigReconciler(
		mgr.GetClient(),
		ctrl.Log.WithName("controllers").WithName("VaultUnsealConfig"),
		mgr.GetScheme(),
		clientRepository,
		reconcilerOptions,
	)

	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to setup reconciler: %w", err)
	}

	return nil
}

// setupHealthChecks configures health and readiness checks.
func setupHealthChecks(mgr ctrl.Manager) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up health check: %w", err)
	}

	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to set up ready check: %w", err)
	}

	return nil
}
