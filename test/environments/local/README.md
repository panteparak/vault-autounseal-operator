# Local Development Environment

This directory contains Docker Compose files for local development and testing.

## Setup

### Start the Local Environment
```bash
# From project root
docker-compose -f test/environments/local/docker-compose.yml up -d
```

### Services

- **vault-dev** (port 8200): Development mode Vault with root token `dev-root-token`
- **vault-sealed** (port 8201): Production-like sealed Vault that needs initialization
- **vault-init**: Helper service that initializes the sealed Vault
- **consul** (port 8500): Consul backend for HA Vault
- **vault-ha** (port 8202): HA-enabled Vault using Consul backend

### Usage

#### Run Integration Tests Locally
```bash
# Start services
docker-compose -f test/environments/local/docker-compose.yml up -d

# Wait for services to be ready
sleep 10

# Run integration tests
./scripts/run-fast-integration.sh -D

# Clean up
docker-compose -f test/environments/local/docker-compose.yml down -v
```

#### Manual Testing
```bash
# Access dev Vault
export VAULT_ADDR=http://localhost:8200
export VAULT_TOKEN=dev-root-token
vault status

# Check sealed Vault status
export VAULT_ADDR=http://localhost:8201
vault status

# Get unsealing keys (after initialization)
docker exec vault-init-local cat /vault/keys/unseal_keys.txt
docker exec vault-init-local cat /vault/keys/root_token.txt
```

#### Debugging
```bash
# View logs
docker-compose -f test/environments/local/docker-compose.yml logs vault-sealed
docker-compose -f test/environments/local/docker-compose.yml logs vault-init

# Access Consul UI
open http://localhost:8500

# Execute commands in containers
docker exec -it vault-sealed-local vault status
docker exec -it vault-init-local sh
```

### Volumes

Persistent volumes are created for:
- `vault-dev-data`: Development Vault data
- `vault-sealed-data`: Sealed Vault data
- `vault-keys`: Vault initialization keys and tokens
- `consul-data`: Consul data storage

### Network

All services run on the `vault-local` network (172.21.0.0/16) for isolated communication.
