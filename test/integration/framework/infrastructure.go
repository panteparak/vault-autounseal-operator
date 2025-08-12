package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// DockerInfrastructure implements TestInfrastructure using Docker
type DockerInfrastructure struct {
	client       *client.Client
	config       *TestConfig
	vaultInstances map[string]*VaultInstance
	operatorDeployed bool
}

// NewDockerInfrastructure creates a new Docker-based infrastructure manager
func NewDockerInfrastructure(config *TestConfig) (TestInfrastructure, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &DockerInfrastructure{
		client:         dockerClient,
		config:         config,
		vaultInstances: make(map[string]*VaultInstance),
	}, nil
}

// Setup initializes the test infrastructure
func (d *DockerInfrastructure) Setup(ctx context.Context, config *TestConfig) error {
	// Cleanup any existing containers from previous runs
	if err := d.cleanupExistingContainers(ctx); err != nil {
		return fmt.Errorf("failed to cleanup existing containers: %w", err)
	}

	// Create Docker network for test isolation
	if err := d.createTestNetwork(ctx); err != nil {
		return fmt.Errorf("failed to create test network: %w", err)
	}

	return nil
}

// CreateVaultInstance creates and starts a Vault instance
func (d *DockerInfrastructure) CreateVaultInstance(ctx context.Context, setup VaultInstanceSetup) (*VaultInstance, error) {
	// Check if instance already exists
	if existing, exists := d.vaultInstances[setup.Name]; exists {
		return existing, nil
	}

	// Pull Vault image if needed
	image := fmt.Sprintf("hashicorp/vault:%s", d.config.VaultVersion)
	if err := d.pullImageIfNeeded(ctx, image); err != nil {
		return nil, fmt.Errorf("failed to pull Vault image: %w", err)
	}

	// Prepare container configuration
	containerConfig := &container.Config{
		Image: image,
		Env:   d.buildEnvironmentVariables(setup.Environment),
		ExposedPorts: map[string]struct{}{
			"8200/tcp": {},
		},
		Healthcheck: &container.HealthConfig{
			Test:     []string{"CMD", "vault", "status", "-format=json"},
			Interval: setup.HealthChecks.Interval,
			Timeout:  setup.HealthChecks.Timeout,
			Retries:  setup.HealthChecks.Retries,
		},
	}

	// Configure for dev mode or production mode
	if setup.DevMode {
		containerConfig.Cmd = []string{"vault", "server", "-dev"}
	} else {
		containerConfig.Cmd = []string{"vault", "server", "-config=/vault/config/vault.hcl"}
	}

	hostConfig := &container.HostConfig{
		PortBindings: map[string][]types.PortBinding{
			"8200/tcp": {
				{HostPort: strconv.Itoa(setup.Port)},
			},
		},
		AutoRemove: true,
		CapAdd:     []string{"IPC_LOCK"},
	}

	// Create container
	resp, err := d.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, setup.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create container %s: %w", setup.Name, err)
	}

	// Start container
	if err := d.client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container %s: %w", setup.Name, err)
	}

	// Create VaultInstance object
	instance := &VaultInstance{
		Name:        setup.Name,
		ContainerID: resp.ID,
		Port:        setup.Port,
		Endpoint:    fmt.Sprintf("http://localhost:%d", setup.Port),
		Environment: setup.Environment,
		State: VaultState{
			LastChecked: time.Now(),
		},
		HealthStatus: HealthStatus{
			LastCheck: time.Now(),
		},
	}

	// Wait for Vault to be ready
	if err := d.waitForVaultReady(ctx, instance, setup.HealthChecks.StartupDelay); err != nil {
		return nil, fmt.Errorf("vault instance %s failed to become ready: %w", setup.Name, err)
	}

	// Initialize Vault if needed
	if !setup.DevMode && setup.InitialState != "uninitialized" {
		if err := d.initializeVault(ctx, instance); err != nil {
			return nil, fmt.Errorf("failed to initialize vault %s: %w", setup.Name, err)
		}
	}

	// Set initial state if specified
	if err := d.setVaultState(ctx, instance, setup.InitialState); err != nil {
		return nil, fmt.Errorf("failed to set vault state %s: %w", setup.Name, err)
	}

	d.vaultInstances[setup.Name] = instance
	return instance, nil
}

// DeployOperator deploys the vault-autounseal-operator using Helm
func (d *DockerInfrastructure) DeployOperator(ctx context.Context, config OperatorConfig) error {
	if d.operatorDeployed {
		return nil // Already deployed
	}

	// Build Helm command
	helmArgs := []string{
		"upgrade", "--install", "vault-autounseal-operator",
		"./helm/vault-autounseal-operator",
		"--namespace", "vault-operator-system",
		"--create-namespace",
		"--wait",
		"--timeout=600s",
		"--set", fmt.Sprintf("image.repository=%s", config.Image),
		"--set", fmt.Sprintf("image.tag=%s", config.Tag),
		"--set", "image.pullPolicy=Never",
		"--set", fmt.Sprintf("operator.logLevel=%s", config.LogLevel),
		"--set", fmt.Sprintf("resources.requests.cpu=%s", config.Resources.CPU),
		"--set", fmt.Sprintf("resources.requests.memory=%s", config.Resources.Memory),
		"--set", "crd.create=true",
	}

	// Add additional Helm values
	for key, value := range config.HelmValues {
		helmArgs = append(helmArgs, "--set", fmt.Sprintf("%s=%s", key, value))
	}

	cmd := exec.CommandContext(ctx, "helm", helmArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to deploy operator with helm: %w\nOutput: %s", err, output)
	}

	d.operatorDeployed = true
	return nil
}

// Cleanup removes all test infrastructure
func (d *DockerInfrastructure) Cleanup(ctx context.Context) error {
	var errors []string

	// Stop and remove Vault containers
	for name, instance := range d.vaultInstances {
		if err := d.client.ContainerStop(ctx, instance.ContainerID, container.StopOptions{}); err != nil {
			errors = append(errors, fmt.Sprintf("failed to stop container %s: %v", name, err))
		}
	}

	// Uninstall operator
	if d.operatorDeployed {
		cmd := exec.CommandContext(ctx, "helm", "uninstall", "vault-autounseal-operator", "-n", "vault-operator-system")
		if output, err := cmd.CombinedOutput(); err != nil {
			errors = append(errors, fmt.Sprintf("failed to uninstall operator: %v\nOutput: %s", err, output))
		}
	}

	// Remove test network
	if err := d.removeTestNetwork(ctx); err != nil {
		errors = append(errors, fmt.Sprintf("failed to remove test network: %v", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("cleanup errors: %s", strings.Join(errors, "; "))
	}

	return nil
}

// GetLogs retrieves logs from a component
func (d *DockerInfrastructure) GetLogs(ctx context.Context, component string) ([]string, error) {
	var logs []string

	switch component {
	case "operator":
		// Get operator logs using kubectl
		cmd := exec.CommandContext(ctx, "kubectl", "logs",
			"-l", "app.kubernetes.io/name=vault-autounseal-operator",
			"-n", "vault-operator-system", "--tail=100")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("failed to get operator logs: %w", err)
		}
		logs = strings.Split(string(output), "\n")

	default:
		// Check if it's a Vault instance
		if instance, exists := d.vaultInstances[component]; exists {
			containerLogs, err := d.client.ContainerLogs(ctx, instance.ContainerID, types.ContainerLogsOptions{
				ShowStdout: true,
				ShowStderr: true,
				Tail:       "100",
			})
			if err != nil {
				return nil, fmt.Errorf("failed to get container logs for %s: %w", component, err)
			}
			defer containerLogs.Close()

			// Read logs (simplified - in real implementation would properly parse Docker log format)
			buf := make([]byte, 1024*10) // 10KB buffer
			n, _ := containerLogs.Read(buf)
			logs = strings.Split(string(buf[:n]), "\n")
		} else {
			return nil, fmt.Errorf("unknown component: %s", component)
		}
	}

	return logs, nil
}

// GetMetrics retrieves metrics from the test infrastructure
func (d *DockerInfrastructure) GetMetrics(ctx context.Context) (*TestMetrics, error) {
	metrics := &TestMetrics{
		VaultResponseTimes: make(map[string]time.Duration),
		OperatorMetrics:    make(map[string]float64),
		ResourceUsage: ResourceUsageMetrics{
			CPUUsage:    make(map[string]float64),
			MemoryUsage: make(map[string]int64),
			NetworkIO:   make(map[string]int64),
			DiskIO:      make(map[string]int64),
		},
		APICallCounts: make(map[string]int),
	}

	// Collect metrics from Vault instances
	for name, instance := range d.vaultInstances {
		// Measure response time
		start := time.Now()
		_, err := d.getVaultStatus(ctx, instance)
		responseTime := time.Since(start)

		if err == nil {
			metrics.VaultResponseTimes[name] = responseTime
		}

		// Get container stats
		stats, err := d.client.ContainerStats(ctx, instance.ContainerID, false)
		if err == nil {
			var containerStats types.StatsJSON
			if err := json.NewDecoder(stats.Body).Decode(&containerStats); err == nil {
				metrics.ResourceUsage.CPUUsage[name] = calculateCPUPercentage(&containerStats)
				metrics.ResourceUsage.MemoryUsage[name] = int64(containerStats.MemoryStats.Usage)
			}
			stats.Body.Close()
		}
	}

	return metrics, nil
}

// Helper methods

func (d *DockerInfrastructure) cleanupExistingContainers(ctx context.Context) error {
	containers, err := d.client.ContainerList(ctx, types.ContainerListOptions{
		All: true,
	})
	if err != nil {
		return err
	}

	for _, container := range containers {
		for _, name := range container.Names {
			if strings.Contains(name, "vault-") || strings.Contains(name, "test-") {
				d.client.ContainerStop(ctx, container.ID, container.StopOptions{})
				d.client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{})
			}
		}
	}

	return nil
}

func (d *DockerInfrastructure) createTestNetwork(ctx context.Context) error {
	// Implementation for creating Docker network
	return nil
}

func (d *DockerInfrastructure) removeTestNetwork(ctx context.Context) error {
	// Implementation for removing Docker network
	return nil
}

func (d *DockerInfrastructure) pullImageIfNeeded(ctx context.Context, image string) error {
	// Check if image exists locally
	_, _, err := d.client.ImageInspectWithRaw(ctx, image)
	if err == nil {
		return nil // Image already exists
	}

	// Pull image
	_, err = d.client.ImagePull(ctx, image, types.ImagePullOptions{})
	return err
}

func (d *DockerInfrastructure) buildEnvironmentVariables(env map[string]string) []string {
	var envVars []string
	for key, value := range env {
		envVars = append(envVars, fmt.Sprintf("%s=%s", key, value))
	}
	return envVars
}

func (d *DockerInfrastructure) waitForVaultReady(ctx context.Context, instance *VaultInstance, startupDelay time.Duration) error {
	// Wait for startup delay
	time.Sleep(startupDelay)

	// Poll for readiness
	timeout := time.After(d.config.Timeouts.VaultStartup)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for vault to be ready")
		case <-ticker.C:
			if ready, _ := d.isVaultReady(ctx, instance); ready {
				return nil
			}
		}
	}
}

func (d *DockerInfrastructure) isVaultReady(ctx context.Context, instance *VaultInstance) (bool, error) {
	// Check container health
	containerJSON, err := d.client.ContainerInspect(ctx, instance.ContainerID)
	if err != nil {
		return false, err
	}

	if containerJSON.State.Health != nil && containerJSON.State.Health.Status == "healthy" {
		return true, nil
	}

	// Fallback to API check
	_, err = d.getVaultStatus(ctx, instance)
	return err == nil, err
}

func (d *DockerInfrastructure) getVaultStatus(ctx context.Context, instance *VaultInstance) (map[string]interface{}, error) {
	// Implementation would make HTTP call to Vault status endpoint
	// For now, return a placeholder
	return map[string]interface{}{
		"sealed":      false,
		"initialized": true,
	}, nil
}

func (d *DockerInfrastructure) initializeVault(ctx context.Context, instance *VaultInstance) error {
	// Implementation would initialize Vault if needed
	return nil
}

func (d *DockerInfrastructure) setVaultState(ctx context.Context, instance *VaultInstance, state string) error {
	// Implementation would set Vault to desired state (sealed/unsealed)
	return nil
}

func calculateCPUPercentage(stats *types.StatsJSON) float64 {
	// Simplified CPU calculation
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage - stats.PreCPUStats.SystemUsage)

	if systemDelta > 0 && cpuDelta > 0 {
		return (cpuDelta / systemDelta) * float64(len(stats.CPUStats.CPUUsage.PercpuUsage)) * 100.0
	}
	return 0
}
