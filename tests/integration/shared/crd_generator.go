package shared

import (
	"fmt"
	"strings"
)

// CRDGenerator generates Kubernetes CRD manifests for testing
type CRDGenerator struct{}

// NewCRDGenerator creates a new CRD generator
func NewCRDGenerator() *CRDGenerator {
	return &CRDGenerator{}
}

// GenerateVaultUnsealConfigCRD generates the VaultUnsealConfig CRD manifest
func (g *CRDGenerator) GenerateVaultUnsealConfigCRD() string {
	return `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: vaultunsealconfigs.vault.io
  annotations:
    controller-gen.kubebuilder.io/version: v0.14.0
spec:
  group: vault.io
  names:
    kind: VaultUnsealConfig
    listKind: VaultUnsealConfigList
    plural: vaultunsealconfigs
    singular: vaultunsealconfig
    shortNames:
    - vuc
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          apiVersion:
            type: string
          kind:
            type: string
          metadata:
            type: object
          spec:
            type: object
            properties:
              vaultInstances:
                type: array
                items:
                  type: object
                  properties:
                    name:
                      type: string
                      description: "Name of the Vault instance"
                    endpoint:
                      type: string
                      description: "Vault server endpoint URL"
                    unsealKeys:
                      type: array
                      items:
                        type: string
                      description: "Base64 encoded unseal keys"
                    threshold:
                      type: integer
                      minimum: 1
                      description: "Number of keys required to unseal"
                    tlsSkipVerify:
                      type: boolean
                      description: "Skip TLS verification"
                      default: false
                    timeout:
                      type: string
                      description: "Timeout for unseal operations"
                      default: "30s"
                    retryPolicy:
                      type: object
                      properties:
                        maxRetries:
                          type: integer
                          minimum: 0
                          default: 3
                        backoffInterval:
                          type: string
                          default: "10s"
                      description: "Retry policy for failed operations"
                  required:
                  - name
                  - endpoint
                  - unsealKeys
                  - threshold
                description: "List of Vault instances to manage"
              globalSettings:
                type: object
                properties:
                  enableMetrics:
                    type: boolean
                    default: true
                    description: "Enable metrics collection"
                  logLevel:
                    type: string
                    enum: ["debug", "info", "warn", "error"]
                    default: "info"
                    description: "Log level for the operator"
                  reconcileInterval:
                    type: string
                    default: "5m"
                    description: "How often to reconcile resources"
                description: "Global settings for the operator"
            required:
            - vaultInstances
          status:
            type: object
            properties:
              conditions:
                type: array
                items:
                  type: object
                  properties:
                    type:
                      type: string
                    status:
                      type: string
                      enum: ["True", "False", "Unknown"]
                    lastTransitionTime:
                      type: string
                      format: date-time
                    reason:
                      type: string
                    message:
                      type: string
                  required:
                  - type
                  - status
              vaultInstanceStatuses:
                type: array
                items:
                  type: object
                  properties:
                    name:
                      type: string
                    status:
                      type: string
                      enum: ["Sealed", "Unsealed", "Unknown", "Error"]
                    lastSeen:
                      type: string
                      format: date-time
                    message:
                      type: string
                  required:
                  - name
                  - status
              observedGeneration:
                type: integer
                format: int64
        required:
        - spec
    subresources:
      status: {}
    additionalPrinterColumns:
    - name: Instances
      type: integer
      description: Number of Vault instances
      jsonPath: .spec.vaultInstances[*].name
    - name: Age
      type: date
      jsonPath: .metadata.creationTimestamp
---`
}

// GenerateRBACManifests generates RBAC manifests for the operator
func (g *CRDGenerator) GenerateRBACManifests(namespace string) string {
	if namespace == "" {
		namespace = "default"
	}

	return fmt.Sprintf(`apiVersion: v1
kind: ServiceAccount
metadata:
  name: vault-autounseal-operator
  namespace: %s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: vault-autounseal-operator
rules:
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
- apiGroups: ["vault.io"]
  resources: ["vaultunsealconfigs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["vault.io"]
  resources: ["vaultunsealconfigs/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["vault.io"]
  resources: ["vaultunsealconfigs/finalizers"]
  verbs: ["update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: vault-autounseal-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: vault-autounseal-operator
subjects:
- kind: ServiceAccount
  name: vault-autounseal-operator
  namespace: %s
---`, namespace, namespace)
}

// GenerateTestVaultUnsealConfig generates a test VaultUnsealConfig resource
func (g *CRDGenerator) GenerateTestVaultUnsealConfig(name, namespace string, vaultInstances []VaultInstanceConfig) string {
	if namespace == "" {
		namespace = "default"
	}

	var instancesYAML strings.Builder
	for i, instance := range vaultInstances {
		if i > 0 {
			instancesYAML.WriteString("\n")
		}
		instancesYAML.WriteString(fmt.Sprintf(`  - name: %s
    endpoint: %s
    threshold: %d
    tlsSkipVerify: %t
    unsealKeys:`, instance.Name, instance.Endpoint, instance.Threshold, instance.TLSSkipVerify))
		
		for _, key := range instance.UnsealKeys {
			instancesYAML.WriteString(fmt.Sprintf("\n    - \"%s\"", key))
		}

		if instance.Timeout != "" {
			instancesYAML.WriteString(fmt.Sprintf("\n    timeout: %s", instance.Timeout))
		}

		if instance.RetryPolicy != nil {
			instancesYAML.WriteString(fmt.Sprintf(`
    retryPolicy:
      maxRetries: %d
      backoffInterval: %s`, instance.RetryPolicy.MaxRetries, instance.RetryPolicy.BackoffInterval))
		}
	}

	return fmt.Sprintf(`apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: %s
  namespace: %s
spec:
  vaultInstances:
%s
  globalSettings:
    enableMetrics: true
    logLevel: info
    reconcileInterval: 30s`, name, namespace, instancesYAML.String())
}

// VaultInstanceConfig represents a vault instance configuration for testing
type VaultInstanceConfig struct {
	Name          string
	Endpoint      string
	UnsealKeys    []string
	Threshold     int
	TLSSkipVerify bool
	Timeout       string
	RetryPolicy   *RetryPolicyConfig
}

// RetryPolicyConfig represents retry policy configuration
type RetryPolicyConfig struct {
	MaxRetries       int
	BackoffInterval  string
}

// GenerateOperatorDeployment generates a deployment manifest for the operator
func (g *CRDGenerator) GenerateOperatorDeployment(namespace, image string) string {
	if namespace == "" {
		namespace = "default"
	}
	if image == "" {
		image = "vault-autounseal-operator:latest"
	}

	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: vault-autounseal-operator
  namespace: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: vault-autounseal-operator
  template:
    metadata:
      labels:
        app: vault-autounseal-operator
    spec:
      serviceAccountName: vault-autounseal-operator
      containers:
      - name: manager
        image: %s
        imagePullPolicy: IfNotPresent
        env:
        - name: WATCH_NAMESPACE
          value: ""
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: OPERATOR_NAME
          value: "vault-autounseal-operator"
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
---`, namespace, image)
}

// GenerateCompleteTestManifests generates all required manifests for testing
func (g *CRDGenerator) GenerateCompleteTestManifests(namespace string, vaultConfigs []VaultInstanceConfig) string {
	manifests := []string{
		g.GenerateVaultUnsealConfigCRD(),
		g.GenerateRBACManifests(namespace),
	}

	for i, config := range vaultConfigs {
		configName := fmt.Sprintf("test-vault-config-%d", i)
		manifests = append(manifests, g.GenerateTestVaultUnsealConfig(configName, namespace, []VaultInstanceConfig{config}))
	}

	return strings.Join(manifests, "\n---\n")
}