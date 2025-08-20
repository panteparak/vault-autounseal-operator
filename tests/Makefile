# Test Module Makefile for vault-autounseal-operator
# Provides organized test execution similar to Java's main/test structure

.PHONY: help test-all test-unit test-integration test-e2e test-performance test-chaos test-boundary
.PHONY: test-clean test-setup test-deps test-report test-coverage
.PHONY: run-unit run-integration run-e2e run-performance run-chaos run-boundary

# Default timeout for tests
TEST_TIMEOUT ?= 30m
VERBOSE ?= false
PARALLELISM ?= 4

# Configurable external component versions (can be overridden via environment)
VAULT_VERSION ?= 1.19.0
K3S_VERSION ?= v1.30.8-k3s1
GO_VERSION ?= 1.24
TESTCONTAINERS_VERSION ?= 0.26.0

# Test configuration
TEST_MAX_RETRIES ?= 3
TEST_MEMORY_LIMIT ?= 2Gi
TEST_CPU_LIMIT ?= 2
TEST_SCENARIO ?= basic
ENABLE_COMPATIBILITY_TESTING ?= false

# Test module directories
UNIT_DIR := unit
INTEGRATION_DIR := integration
E2E_DIR := e2e
PERFORMANCE_DIR := performance
CHAOS_DIR := chaos
BOUNDARY_DIR := boundary

# Integration test scenarios from legacy migration
INTEGRATION_SCENARIOS := vault_api k3s_crd failover multi_vault operator_status

# Go test flags
ifeq ($(VERBOSE),true)
	GO_TEST_FLAGS += -v
endif

GO_TEST_FLAGS += -timeout $(TEST_TIMEOUT) -parallel $(PARALLELISM) -failfast -count=1
GO_TEST_COVERAGE_FLAGS := -coverprofile=coverage.out -covermode=atomic

# Fail-fast settings for CI
ifeq ($(CI),true)
	GO_TEST_FLAGS += -race
endif

help: ## Show this help
	@echo "Test Module Makefile for vault-autounseal-operator"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Test Categories:"
	@echo "  test-unit         Run unit tests (fast, isolated)"
	@echo "  test-integration  Run integration tests (TestContainers)"
	@echo "  test-e2e         Run end-to-end tests (full workflows)"
	@echo "  test-performance Run performance/load tests"
	@echo "  test-chaos       Run chaos engineering tests"
	@echo "  test-boundary    Run boundary/edge case tests"
	@echo "  test-all         Run all test categories"
	@echo ""
	@echo "Utilities:"
	@echo "  test-setup       Set up test environment"
	@echo "  test-clean       Clean test artifacts"
	@echo "  test-deps        Download test dependencies"
	@echo "  test-coverage    Generate coverage report"
	@echo "  test-report      Generate test report"
	@echo ""
	@echo "Configuration:"
	@echo "  TEST_TIMEOUT     Test timeout (default: 30m)"
	@echo "  VERBOSE          Verbose output (default: false)"
	@echo "  PARALLELISM      Parallel test count (default: 4)"
	@echo ""
	@echo "Component Versions (configurable via environment):"
	@echo "  VAULT_VERSION    Vault version (default: $(VAULT_VERSION))"
	@echo "  K3S_VERSION      K3s version (default: $(K3S_VERSION))"
	@echo "  GO_VERSION       Go version (default: $(GO_VERSION))"
	@echo ""
	@echo "Test Configuration:"
	@echo "  TEST_SCENARIO    Test scenario (basic,stress,chaos) (default: $(TEST_SCENARIO))"
	@echo "  TEST_MAX_RETRIES Max retries for flaky tests (default: $(TEST_MAX_RETRIES))"
	@echo "  ENABLE_COMPATIBILITY_TESTING Enable compatibility tests (default: $(ENABLE_COMPATIBILITY_TESTING))"

# Main test targets
test-all: test-deps ## Run all test categories
	@echo "🧪 Running all test categories..."
	@$(MAKE) test-unit || echo "❌ Unit tests failed"
	@$(MAKE) test-integration || echo "❌ Integration tests failed"
	@$(MAKE) test-e2e || echo "❌ E2E tests failed"
	@$(MAKE) test-performance || echo "❌ Performance tests failed"
	@$(MAKE) test-chaos || echo "❌ Chaos tests failed"
	@$(MAKE) test-boundary || echo "❌ Boundary tests failed"
	@echo "✅ All test categories completed"

test-unit: test-deps ## Run unit tests
	@echo "🔬 Running unit tests..."
	@go test $(GO_TEST_FLAGS) $(GO_TEST_COVERAGE_FLAGS) ./$(UNIT_DIR)/...
	@echo "✅ Unit tests completed"

test-integration: test-deps ## Run integration tests
	@echo "🔗 Running integration tests..."
	@echo "Using versions: Vault=$(VAULT_VERSION), K3s=$(K3S_VERSION), Go=$(GO_VERSION)"
	@VAULT_VERSION=$(VAULT_VERSION) K3S_VERSION=$(K3S_VERSION) GO_VERSION=$(GO_VERSION) \
	 TEST_SCENARIO=$(TEST_SCENARIO) TEST_MAX_RETRIES=$(TEST_MAX_RETRIES) \
	 ENABLE_COMPATIBILITY_TESTING=$(ENABLE_COMPATIBILITY_TESTING) \
	 go test $(GO_TEST_FLAGS) ./$(INTEGRATION_DIR)/...

test-integration-scenarios: test-deps ## Run specific integration test scenarios
	@echo "🎯 Running integration test scenarios..."
	@for scenario in $(INTEGRATION_SCENARIOS); do \
		echo "▶️ Running $$scenario tests..."; \
		case $$scenario in \
			vault_api) go test $(GO_TEST_FLAGS) -run "TestVaultAPITestSuite" ./$(INTEGRATION_DIR)/ || exit 1 ;; \
			k3s_crd) go test $(GO_TEST_FLAGS) -run "TestK3sCRDTestSuite" ./$(INTEGRATION_DIR)/ || exit 1 ;; \
			failover) go test $(GO_TEST_FLAGS) -run "TestFailoverTestSuite" ./$(INTEGRATION_DIR)/ || exit 1 ;; \
			multi_vault) go test $(GO_TEST_FLAGS) -run "TestMultiVaultTestSuite" ./$(INTEGRATION_DIR)/ || exit 1 ;; \
			operator_status) go test $(GO_TEST_FLAGS) -run "TestOperatorStatusTestSuite" ./$(INTEGRATION_DIR)/ || exit 1 ;; \
			*) echo "⚠️ Unknown scenario: $$scenario" ;; \
		esac; \
	done
	@echo "✅ Integration tests completed"

test-e2e: test-deps ## Run end-to-end tests
	@echo "🌐 Running end-to-end tests..."
	@go test $(GO_TEST_FLAGS) ./$(E2E_DIR)/...
	@echo "✅ E2E tests completed"

test-performance: test-deps ## Run performance tests
	@echo "⚡ Running performance tests..."
	@go test $(GO_TEST_FLAGS) ./$(PERFORMANCE_DIR)/...
	@echo "✅ Performance tests completed"

test-chaos: test-deps ## Run chaos engineering tests
	@echo "🌪️  Running chaos tests..."
	@go test $(GO_TEST_FLAGS) ./$(CHAOS_DIR)/...
	@echo "✅ Chaos tests completed"

test-boundary: test-deps ## Run boundary condition tests
	@echo "🏔️  Running boundary tests..."
	@go test $(GO_TEST_FLAGS) ./$(BOUNDARY_DIR)/...
	@echo "✅ Boundary tests completed"

# Individual test runners (for CI/CD flexibility)
run-unit:
	@go test $(GO_TEST_FLAGS) ./$(UNIT_DIR)/...

run-integration:
	@go test $(GO_TEST_FLAGS) ./$(INTEGRATION_DIR)/...

run-e2e:
	@go test $(GO_TEST_FLAGS) ./$(E2E_DIR)/...

run-performance:
	@go test $(GO_TEST_FLAGS) ./$(PERFORMANCE_DIR)/...

run-chaos:
	@go test $(GO_TEST_FLAGS) ./$(CHAOS_DIR)/...

run-boundary:
	@go test $(GO_TEST_FLAGS) ./$(BOUNDARY_DIR)/...

# Specific test suites
test-validation: test-deps ## Run validation unit tests
	@echo "✅ Running validation tests..."
	@go test $(GO_TEST_FLAGS) ./$(UNIT_DIR)/validation/...

test-vault-client: test-deps ## Run vault client tests
	@echo "🔐 Running vault client tests..."
	@go test $(GO_TEST_FLAGS) ./$(UNIT_DIR)/vault/...

test-controller: test-deps ## Run controller tests
	@echo "🎮 Running controller tests..."
	@go test $(GO_TEST_FLAGS) ./$(UNIT_DIR)/controller/...

# Utility targets
test-setup: ## Set up test environment
	@echo "🔧 Setting up test environment..."
	@mkdir -p reports coverage
	@docker info > /dev/null || (echo "❌ Docker not available" && exit 1)
	@echo "✅ Test environment ready"

test-deps: ## Download test dependencies
	@echo "📦 Downloading test dependencies..."
	@go mod download
	@go mod tidy

test-clean: ## Clean test artifacts
	@echo "🧹 Cleaning test artifacts..."
	@rm -rf reports/ coverage/ *.out *.xml *.log
	@go clean -testcache
	@docker system prune -f --volumes --filter "label=org.testcontainers.sessionId" 2>/dev/null || true
	@k3d cluster delete vault-test 2>/dev/null || true
	@k3d cluster delete vault-test-* 2>/dev/null || true
	@docker rm -f $(shell docker ps -aq --filter "name=vault-") 2>/dev/null || true
	@docker rm -f $(shell docker ps -aq --filter "name=k3s-") 2>/dev/null || true
	@echo "✅ Test artifacts cleaned"

test-coverage: test-unit ## Generate coverage report
	@echo "📊 Generating coverage report..."
	@mkdir -p coverage
	@go tool cover -html=coverage.out -o coverage/coverage.html
	@go tool cover -func=coverage.out > coverage/coverage.txt
	@echo "✅ Coverage report generated at coverage/"

test-report: ## Generate comprehensive test report
	@echo "📋 Generating test report..."
	@mkdir -p reports
	@echo "# Test Execution Report" > reports/test-report.md
	@echo "Generated: $$(date)" >> reports/test-report.md
	@echo "" >> reports/test-report.md
	@echo "## Test Categories" >> reports/test-report.md
	@echo "- Unit Tests: Fast, isolated component tests" >> reports/test-report.md
	@echo "- Integration Tests: TestContainer-based integration testing" >> reports/test-report.md
	@echo "- E2E Tests: Complete workflow and scenario testing" >> reports/test-report.md
	@echo "- Performance Tests: Load and benchmark testing" >> reports/test-report.md
	@echo "- Chaos Tests: Fault injection and resilience testing" >> reports/test-report.md
	@echo "- Boundary Tests: Edge cases and limit testing" >> reports/test-report.md
	@echo "✅ Test report generated at reports/"

# Docker-based test execution (for CI/CD)
test-docker: ## Run tests in Docker container
	@echo "🐳 Running tests in Docker..."
	@docker run --rm -v $(PWD):/workspace -w /workspace/tests \
		-e TEST_TIMEOUT=$(TEST_TIMEOUT) -e VERBOSE=$(VERBOSE) \
		golang:1.23.1 make test-all

# Watch mode for development
test-watch: ## Run tests in watch mode (requires entr)
	@echo "👀 Watching for changes..."
	@find . -name "*.go" | entr -c make test-unit

# Quick smoke test
test-smoke: test-deps ## Quick smoke test
	@echo "💨 Running smoke tests..."
	@go test $(GO_TEST_FLAGS) -short ./$(UNIT_DIR)/validation/
	@echo "✅ Smoke tests completed"

# Benchmark tests
test-bench: test-deps ## Run benchmark tests
	@echo "🏃 Running benchmarks..."
	@go test -bench=. -benchmem ./$(PERFORMANCE_DIR)/...
	@echo "✅ Benchmarks completed"

# Test with race detection
test-race: test-deps ## Run tests with race detection
	@echo "🏁 Running tests with race detection..."
	@go test $(GO_TEST_FLAGS) -race ./$(UNIT_DIR)/...
	@echo "✅ Race detection tests completed"

# Version-specific and compatibility testing
test-compatibility: test-deps ## Run compatibility tests across multiple versions
	@echo "🔄 Running compatibility tests..."
	@if [ "$(ENABLE_COMPATIBILITY_TESTING)" = "true" ]; then \
		echo "Testing Vault versions: 1.16.0, 1.17.0, 1.18.0, 1.19.0"; \
		for version in 1.16.0 1.17.0 1.18.0 1.19.0; do \
			echo "▶️ Testing Vault $$version"; \
			VAULT_VERSION=$$version go test $(GO_TEST_FLAGS) -run "TestCompatibility" ./$(INTEGRATION_DIR)/ || exit 1; \
		done; \
		echo "Testing K3s versions: v1.28.0-k3s1, v1.29.0-k3s1, v1.30.8-k3s1"; \
		for version in v1.28.0-k3s1 v1.29.0-k3s1 v1.30.8-k3s1; do \
			echo "▶️ Testing K3s $$version"; \
			K3S_VERSION=$$version go test $(GO_TEST_FLAGS) -run "TestCompatibility" ./$(INTEGRATION_DIR)/ || exit 1; \
		done; \
	else \
		echo "⚠️ Compatibility testing disabled. Set ENABLE_COMPATIBILITY_TESTING=true to enable."; \
	fi
	@echo "✅ Compatibility tests completed"

test-versions: ## Show current configured versions
	@echo "📋 Current Version Configuration:"
	@echo "  Vault:         $(VAULT_VERSION)"
	@echo "  K3s:           $(K3S_VERSION)"
	@echo "  Go:            $(GO_VERSION)"
	@echo "  TestContainers: $(TESTCONTAINERS_VERSION)"
	@echo "  Test Scenario: $(TEST_SCENARIO)"
	@echo "  Max Retries:   $(TEST_MAX_RETRIES)"
	@echo "  Compatibility: $(ENABLE_COMPATIBILITY_TESTING)"

test-config: ## Validate configuration files
	@echo "⚙️ Validating configuration..."
	@go test $(GO_TEST_FLAGS) -run "TestConfig" ./config/...
	@echo "✅ Configuration validation completed"
