# New Workflow Plans

## Branch
### feat/* (push)
Run: lint, test, helm-package

### feat/* (merge request)
Run: lint, test, helm-package, (modern-integration-test)

### main
Run: lint, test, build with tag main and commit, security-scan, helm-package, (modern-integration-test), Security Scanning, Performance Benchmarking, Resource Profiling

### release to tag
Run: image promote from main with versioning, build and publish docs, build and publish helm chart


# GitHub Actions CI/CD Workflow Comprehensive Summary

**Last Updated:** August 20, 2025
**Total Workflows:** 11
**Analysis Status:** Complete - All workflows analyzed

---

## Executive Summary

The CI/CD pipeline consists of 11 separate workflows with significant overlap and complexity. Current setup results in ~45-60 CI minutes per PR with duplicate jobs and inefficient resource usage. This analysis identifies consolidation opportunities that could reduce CI time by 40-50%.

## Detailed Workflow Analysis

### 1. **Main CI/CD Pipeline** (`ci.yaml`)
- **Purpose**: Primary build, test, and deployment workflow
- **Triggers**: Push to main/develop, PRs to main
- **Concurrency**: Grouped by workflow and ref with cancel-in-progress
- **Jobs**:
  - `lint`: golangci-lint code analysis
  - `test`: Go unit tests with race detection
  - `integration-tests`: Docker Compose-based integration tests ⚠️ *Legacy*
  - `build`: Multi-platform Docker image building
  - `security`: Trivy vulnerability scanning ⚠️ *Duplicated*
  - `helm-package`: Helm chart packaging
  - `deploy`: Conditional deployment (staging/production)
- **Issues**: Contains legacy integration tests and duplicated security scanning
- **Estimated Duration**: ~25-30 minutes

### 2. **Dedicated Test Workflow** (`test.yml`)
- **Purpose**: Focused testing execution with coverage
- **Triggers**: Push to main/develop, PRs to main/develop
- **Go Version**: 1.24 (newer than other workflows)
- **Jobs**:
  - `lint`: golangci-lint with 5m timeout ⚠️ *Duplicated*
  - `unit-tests`: Unit tests with race detection and Codecov upload
  - `integration-tests`: Basic integration testing ⚠️ *Outdated*
  - `build`: Binary compilation and testing
- **Issues**: Major overlap with ci.yaml, outdated integration test paths
- **Estimated Duration**: ~15-20 minutes

### 3. **Modern Integration Tests** (`integration-tests-go.yml`)
- **Purpose**: TestContainers-based integration testing (recommended approach)
- **Triggers**: Push to main/develop, PRs to main, weekly schedule
- **Strategy**: Matrix testing with scenarios (basic, failover, multi-vault)
- **Jobs**:
  - `setup-infrastructure`: K3d cluster setup and operator image building
  - `integration-test`: Scenario-based testing with comprehensive validation
  - `test-summary`: Result aggregation
  - `quality-check`: Quality gates validation
- **Features**: Production-like testing, sealed vault validation, resource lifecycle
- **Estimated Duration**: ~35-40 minutes
- **Status**: ✅ **This is the preferred integration test approach**

### 4. **Legacy Integration Tests** (`integration-tests.yml`)
- **Purpose**: Docker Compose-based integration tests (comprehensive but legacy)
- **Triggers**: Push/PR events, weekly schedule
- **Features**:
  - Matrix strategy with Vault versions (1.20.0)
  - Multi-scenario testing (basic, failover, multi-vault)
  - K3d cluster deployment
  - Comprehensive validation (11 test phases)
  - Production Vault testing with unsealing
- **Jobs**: Single comprehensive job with extensive testing phases
- **Estimated Duration**: ~25-30 minutes
- **Status**: ⚠️ **Should be consolidated with integration-tests-go.yml**

### 5. **Security Scanning** (`security.yml`)
- **Purpose**: Dedicated security analysis
- **Triggers**: Push to main/develop, PRs to main, daily schedule (2 AM UTC)
- **Jobs**:
  - `trivy-scan`: Docker image and filesystem vulnerability scanning
  - SARIF result upload to GitHub Security tab
- **Features**: Comprehensive vulnerability scanning
- **Issues**: Overlaps with security scanning in ci.yaml
- **Estimated Duration**: ~8-10 minutes

### 6. **Performance Benchmarking** (`benchmark.yml`)
- **Purpose**: Go performance benchmarking and regression detection
- **Triggers**: Push to main, PRs to main, weekly schedule (Sunday)
- **Go Version**: 1.21
- **Jobs**:
  - `benchmark`: Comprehensive performance testing
    - Memory profiling (benchmem)
    - CPU profiling
    - Race condition testing
    - Performance regression checks
- **Artifacts**: Benchmark results, CPU/memory profiles (*.prof)
- **Estimated Duration**: ~10-15 minutes

### 7. **Resource Profiling** (`resource-profiling.yml`)
- **Purpose**: Extended testing with comprehensive resource monitoring
- **Triggers**: Push to main, PRs to main, weekly schedule (Monday 2 AM UTC)
- **Features**: Advanced performance analysis including:
  - **Extended integration tests** with system monitoring
  - **Chaos engineering tests** (50 workers, memory pressure)
  - **Load testing** with profiling
  - **Property-based testing** with memory tracking
  - **Security-focused testing**
  - **Memory leak detection**
  - **Vault compatibility testing** (multiple versions)
- **Matrix Strategy**: Vault versions (1.12.0, 1.13.0, 1.14.0, 1.15.0)
- **Jobs**: `extended-integration-tests`, `compatibility-testing`, `summary-report`
- **Estimated Duration**: ~45-60 minutes
- **Status**: ✅ **Comprehensive but should run less frequently**

### 8. **Documentation Generation** (`docs.yml`)
- **Purpose**: Automated documentation building and GitHub Pages deployment
- **Triggers**:
  - Push to main (docs/, *.md, pkg/**/*.go, helm/**)
  - PRs to main (docs/, *.md)
- **Features**:
  - MkDocs with Material theme
  - API documentation generation
  - CRD documentation
  - Mermaid diagram support
- **Jobs**: `build-docs` with GitHub Pages deployment
- **Estimated Duration**: ~5-8 minutes

### 9. **Helm Repository Management** (`helm-repo.yaml`)
- **Purpose**: Helm chart packaging and GitHub Pages repository
- **Triggers**: Tag creation (v*), manual dispatch
- **Features**:
  - CRD generation and chart updates
  - Multi-version Helm repository maintenance
  - Combined documentation and chart hosting
- **Jobs**: `build-docs`, `build-helm-repo`, `combine-and-deploy`
- **Concurrency**: Pages-specific concurrency group
- **Estimated Duration**: ~10-15 minutes

### 10. **Release Automation** (`release.yaml`)
- **Purpose**: Automated release process with semantic versioning
- **Triggers**: Tag creation (v*), manual dispatch
- **Features**:
  - Multi-platform Docker image building
  - GitHub release creation
  - Helm chart releases
  - Semantic release automation (currently disabled)
- **Jobs**: `determine-version`, `semantic-release` (disabled), build and release jobs
- **Estimated Duration**: ~15-20 minutes

### 11. **Dependabot Auto-merge** (`dependabot-automerge.yaml`)
- **Purpose**: Automated dependency update management
- **Triggers**: Dependabot PRs (opened, synchronize)
- **Features**:
  - Auto-approval for patch/minor updates with 'automerge' label
  - Manual review required for major version updates
- **Jobs**: `auto-merge` (conditional on dependabot actor)
- **Estimated Duration**: ~1-2 minutes

---

## Overlap and Duplication Analysis

### Critical Overlaps

| Function | Workflows | Impact | Recommendation |
|----------|-----------|---------|----------------|
| **Linting** | `ci.yaml`, `test.yml` | ~3-4 minutes duplication | Consolidate to primary workflow |
| **Unit Testing** | `ci.yaml`, `test.yml` | ~5-8 minutes duplication | Keep in test.yml, remove from ci.yaml |
| **Security Scanning** | `ci.yaml`, `security.yml` | ~8-10 minutes duplication | Move to security.yml only |
| **Integration Testing** | `ci.yaml`, `test.yml`, `integration-tests.yml`, `integration-tests-go.yml` | ~20-30 minutes duplication | Consolidate to integration-tests-go.yml |
| **Build Process** | `ci.yaml`, `test.yml`, `release.yaml` | ~5-8 minutes overlap | Standardize build process |

### Version Inconsistencies

- **Go Version**: 1.21 (most workflows) vs 1.24 (`test.yml`)
- **Node Version**: 18 (`helm-repo.yaml`)
- **Vault Version**: 1.20.0 (most tests) vs multiple versions (resource-profiling)

---

## Performance Impact Analysis

### Current State
- **Total Workflows**: 11
- **Average PR Duration**: 45-60 minutes
- **Parallel Jobs**: ~8-12 concurrent
- **Duplicate Operations**: ~25-30 minutes per PR
- **CI Resource Usage**: High (multiple Docker builds, multiple test runs)

### Consolidation Opportunities

#### Phase 1: Immediate Consolidation (Est. 25-30% improvement)
1. **Merge test.yml into ci.yaml** - Remove duplicate linting and unit tests
2. **Deprecate legacy integration tests** - Use only integration-tests-go.yml
3. **Centralize security scanning** - Remove from ci.yaml, keep in security.yml

#### Phase 2: Strategic Reorganization (Est. 15-20% additional improvement)
1. **Create workflow hierarchy**:
   - **Primary CI** (`ci-primary.yaml`): Build, lint, unit tests, basic integration
   - **Extended Testing** (`ci-extended.yaml`): Full integration, security, performance
   - **Release** (`release.yaml`): Release automation
   - **Specialized** (`docs.yaml`, `dependabot-automerge.yaml`): Specific use cases

---

## Recommended Workflow Structure

```
.github/workflows/
├── ci-primary.yaml              # Main CI/CD (15-20 min)
│   ├── lint (golangci-lint)
│   ├── unit-tests (with coverage)
│   ├── build (multi-platform)
│   └── basic-integration (smoke tests)
│
├── ci-extended.yaml             # Extended testing (30-35 min)
│   ├── integration-tests (full scenarios)
│   ├── security-scan (comprehensive)
│   ├── performance-tests
│   └── compatibility-tests
│
├── release.yaml                 # Release automation (15-20 min)
│   ├── version-determination
│   ├── multi-platform-build
│   ├── helm-chart-release
│   └── github-release
│
├── docs.yaml                    # Documentation (5-8 min)
├── dependabot-automerge.yaml    # Dependency automation (1-2 min)
└── resource-profiling.yaml      # Deep analysis (45-60 min, weekly only)
```

### Trigger Strategy
- **ci-primary.yaml**: Every push/PR (fast feedback)
- **ci-extended.yaml**: Daily schedule + manual trigger
- **resource-profiling.yaml**: Weekly only
- **release.yaml**: Tags only
- **docs.yaml**: Documentation changes only

---

## Implementation Priority

### High Priority (Immediate - Week 1)
1. ✅ **Audit workflow usage** - Determine which workflows are actually used
2. 🔄 **Migrate integration tests** - Fully standardize on TestContainers approach
3. 🔄 **Remove duplicate jobs** - Eliminate duplicate linting, unit tests, security scans
4. ⚠️ **Standardize Go version** - Use consistent Go version across all workflows

### Medium Priority (Week 2-3)
1. **Create consolidated workflows** - Implement primary/extended structure
2. **Optimize build process** - Reduce Docker build redundancy
3. **Implement smart triggering** - Path-based workflow triggering

### Low Priority (Month 2)
1. **Performance monitoring** - Baseline and track CI improvements
2. **Advanced optimizations** - Cache strategies, parallel execution
3. **Documentation updates** - Document simplified workflow structure

---

## Risk Assessment

### High Risk
- **Multiple integration test approaches** - Risk of test gaps during migration
- **Go version inconsistencies** - Potential compatibility issues

### Medium Risk
- **Overlapping security scans** - Potential security blind spots during consolidation
- **Release workflow complexity** - Risk of breaking release automation

### Low Risk
- **Documentation workflows** - Low impact if temporarily broken
- **Dependency automation** - Non-critical for core development

---

## Success Metrics

### Performance Targets (Post-Consolidation)
- **PR Duration**: 25-30 minutes (was 45-60 minutes)
- **Workflow Count**: 6-7 (from 11)
- **Duplicate Jobs**: 0 (from 4-5)
- **CI Resource Usage**: 40-50% reduction

### Quality Assurance
- **Test Coverage**: Maintain or improve current levels
- **Security Scanning**: Comprehensive coverage maintained
- **Integration Testing**: More reliable with TestContainers
- **Release Process**: Streamlined but robust

---

## Next Steps

1. **Create consolidation branch** for workflow migration
2. **Test new workflow structure** in isolation
3. **Gradual migration** with fallback to current workflows
4. **Monitor performance** and iterate
5. **Document final structure** for team adoption

---

*This analysis was generated by comprehensive review of all 11 GitHub Actions workflows in the repository. The recommendations prioritize maintainability, performance, and developer experience while preserving all critical CI/CD functionality.*
