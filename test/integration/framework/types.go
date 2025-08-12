package framework

import (
	"context"
	"time"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestFramework provides infrastructure for integration tests
type TestFramework struct {
	KubeClient       client.Client
	KubernetesClient kubernetes.Interface
	Context          context.Context
	Namespace        string
	Config           *TestConfig
	Infrastructure   *TestInfrastructure
	Reporter         TestReporter
}

// TestConfig holds configuration for test execution
type TestConfig struct {
	VaultVersion     string            `yaml:"vaultVersion"`
	TestScenarios    []string          `yaml:"testScenarios"`
	Timeouts         TimeoutConfig     `yaml:"timeouts"`
	VaultConfig      VaultTestConfig   `yaml:"vaultConfig"`
	OperatorConfig   OperatorConfig    `yaml:"operatorConfig"`
	TestSettings     TestSettings      `yaml:"testSettings"`
	Environment      map[string]string `yaml:"environment"`
}

// TimeoutConfig defines various timeouts for test operations
type TimeoutConfig struct {
	VaultStartup    time.Duration `yaml:"vaultStartup"`
	OperatorReady   time.Duration `yaml:"operatorReady"`
	VaultUnseal     time.Duration `yaml:"vaultUnseal"`
	StatusUpdate    time.Duration `yaml:"statusUpdate"`
	TestExecution   time.Duration `yaml:"testExecution"`
	CleanupTimeout  time.Duration `yaml:"cleanupTimeout"`
}

// VaultTestConfig holds Vault-specific test configuration
type VaultTestConfig struct {
	DevMode          bool              `yaml:"devMode"`
	InitializeVaults bool              `yaml:"initializeVaults"`
	UnsealThreshold  int               `yaml:"unsealThreshold"`
	SecretShares     int               `yaml:"secretShares"`
	Endpoints        map[string]string `yaml:"endpoints"`
	TLSConfig        TLSConfig         `yaml:"tlsConfig"`
}

// TLSConfig holds TLS configuration for Vault connections
type TLSConfig struct {
	SkipVerify bool   `yaml:"skipVerify"`
	CACert     string `yaml:"caCert"`
	ClientCert string `yaml:"clientCert"`
	ClientKey  string `yaml:"clientKey"`
}

// OperatorConfig holds operator-specific test configuration
type OperatorConfig struct {
	Image      string            `yaml:"image"`
	Tag        string            `yaml:"tag"`
	LogLevel   string            `yaml:"logLevel"`
	Resources  ResourceLimits    `yaml:"resources"`
	HelmValues map[string]string `yaml:"helmValues"`
}

// ResourceLimits defines resource constraints for test components
type ResourceLimits struct {
	CPU    string `yaml:"cpu"`
	Memory string `yaml:"memory"`
}

// TestSettings holds general test execution settings
type TestSettings struct {
	Parallel           bool   `yaml:"parallel"`
	MaxConcurrency     int    `yaml:"maxConcurrency"`
	FailFast           bool   `yaml:"failFast"`
	VerboseLogging     bool   `yaml:"verboseLogging"`
	CollectLogs        bool   `yaml:"collectLogs"`
	GenerateReports    bool   `yaml:"generateReports"`
	KeepResourcesOnFail bool  `yaml:"keepResourcesOnFail"`
}

// TestScenario represents a test scenario configuration
type TestScenario struct {
	Name         string                 `yaml:"name"`
	Description  string                 `yaml:"description"`
	VaultSetup   VaultScenarioSetup     `yaml:"vaultSetup"`
	TestCases    []string               `yaml:"testCases"`
	Prerequisites []string              `yaml:"prerequisites"`
	Cleanup      bool                   `yaml:"cleanup"`
	Metadata     map[string]interface{} `yaml:"metadata"`
}

// VaultScenarioSetup defines how Vault instances should be configured for a scenario
type VaultScenarioSetup struct {
	Instances []VaultInstanceSetup `yaml:"instances"`
	Network   NetworkSetup         `yaml:"network"`
}

// VaultInstanceSetup defines configuration for a single Vault instance
type VaultInstanceSetup struct {
	Name           string            `yaml:"name"`
	Port           int               `yaml:"port"`
	DevMode        bool              `yaml:"devMode"`
	InitialState   string            `yaml:"initialState"` // "unsealed", "sealed", "uninitialized"
	RootToken      string            `yaml:"rootToken"`
	UnsealKeys     []string          `yaml:"unsealKeys"`
	Environment    map[string]string `yaml:"environment"`
	HealthChecks   HealthCheckConfig `yaml:"healthChecks"`
}

// NetworkSetup defines networking configuration for test scenarios
type NetworkSetup struct {
	DockerNetwork string            `yaml:"dockerNetwork"`
	PortMappings  map[string]string `yaml:"portMappings"`
	HostAliases   map[string]string `yaml:"hostAliases"`
}

// HealthCheckConfig defines health check parameters
type HealthCheckConfig struct {
	Enabled         bool          `yaml:"enabled"`
	Endpoint        string        `yaml:"endpoint"`
	Interval        time.Duration `yaml:"interval"`
	Timeout         time.Duration `yaml:"timeout"`
	Retries         int           `yaml:"retries"`
	StartupDelay    time.Duration `yaml:"startupDelay"`
}

// TestCase represents an individual test case
type TestCase interface {
	Name() string
	Description() string
	Prerequisites() []string
	Execute(ctx context.Context, framework *TestFramework) *TestResult
	Cleanup(ctx context.Context, framework *TestFramework) error
	Tags() []string
}

// TestResult holds the result of a test case execution
type TestResult struct {
	TestName     string                 `json:"testName"`
	Success      bool                   `json:"success"`
	Duration     time.Duration          `json:"duration"`
	Error        error                  `json:"error,omitempty"`
	Details      map[string]interface{} `json:"details"`
	Logs         []string               `json:"logs"`
	Metrics      TestMetrics            `json:"metrics"`
	Timestamp    time.Time              `json:"timestamp"`
}

// TestMetrics holds performance and operational metrics for a test
type TestMetrics struct {
	VaultResponseTimes map[string]time.Duration `json:"vaultResponseTimes"`
	OperatorMetrics    map[string]float64       `json:"operatorMetrics"`
	ResourceUsage      ResourceUsageMetrics     `json:"resourceUsage"`
	APICallCounts      map[string]int           `json:"apiCallCounts"`
}

// ResourceUsageMetrics holds resource consumption data
type ResourceUsageMetrics struct {
	CPUUsage    map[string]float64 `json:"cpuUsage"`
	MemoryUsage map[string]int64   `json:"memoryUsage"`
	NetworkIO   map[string]int64   `json:"networkIO"`
	DiskIO      map[string]int64   `json:"diskIO"`
}

// TestSuite represents a collection of related test cases
type TestSuite interface {
	Name() string
	Description() string
	TestCases() []TestCase
	Setup(ctx context.Context, framework *TestFramework) error
	Teardown(ctx context.Context, framework *TestFramework) error
}

// TestReporter handles test reporting and output
type TestReporter interface {
	StartSuite(suite TestSuite)
	StartTest(testCase TestCase)
	EndTest(testCase TestCase, result *TestResult)
	EndSuite(suite TestSuite, results []*TestResult)
	GenerateReport() error
}

// TestInfrastructure manages the underlying test infrastructure
type TestInfrastructure interface {
	Setup(ctx context.Context, config *TestConfig) error
	CreateVaultInstance(ctx context.Context, setup VaultInstanceSetup) (*VaultInstance, error)
	DeployOperator(ctx context.Context, config OperatorConfig) error
	Cleanup(ctx context.Context) error
	GetLogs(ctx context.Context, component string) ([]string, error)
	GetMetrics(ctx context.Context) (*TestMetrics, error)
}

// VaultInstance represents a running Vault instance in the test environment
type VaultInstance struct {
	Name         string            `json:"name"`
	ContainerID  string            `json:"containerID"`
	Port         int               `json:"port"`
	Endpoint     string            `json:"endpoint"`
	State        VaultState        `json:"state"`
	UnsealKeys   []string          `json:"unsealKeys"`
	RootToken    string            `json:"rootToken"`
	Environment  map[string]string `json:"environment"`
	HealthStatus HealthStatus      `json:"healthStatus"`
}

// VaultState represents the current state of a Vault instance
type VaultState struct {
	Initialized bool      `json:"initialized"`
	Sealed      bool      `json:"sealed"`
	Version     string    `json:"version"`
	LastChecked time.Time `json:"lastChecked"`
}

// HealthStatus represents the health status of a component
type HealthStatus struct {
	Healthy     bool      `json:"healthy"`
	LastCheck   time.Time `json:"lastCheck"`
	ErrorCount  int       `json:"errorCount"`
	LastError   string    `json:"lastError,omitempty"`
}
