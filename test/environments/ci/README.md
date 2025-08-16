# CI Environment Docker Compose Files

This directory contains Docker Compose files specifically designed for CI environments and testing scenarios.

## Files

- `docker-compose.test.yml` - Main testing environment with Vault dev, sealed, and storage services
- `docker-compose.fast-integration.yml` - Fast integration testing with optimized startup times
- `docker-compose.scenario1-dev-vault.yml` - Development Vault scenario
- `docker-compose.scenario2-sealed-vault.yml` - Sealed Vault scenario
- `docker-compose.scenario3-multi-vault.yml` - Multi-Vault scenario

## Usage

### Local Testing
Run integration tests locally using the fast integration setup:
```bash
# From project root
docker-compose -f test/environments/ci/docker-compose.fast-integration.yml up -d
```

### CI Environment
The GitHub Actions workflows automatically use these compose files for testing different scenarios.

### Scripts
The `scripts/run-fast-integration.sh` script provides convenient access to these environments with various configuration options.
