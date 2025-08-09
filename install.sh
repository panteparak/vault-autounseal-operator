#!/bin/bash

set -e

# Vault Auto-Unseal Operator Installation Script
echo "🚀 Installing Vault Auto-Unseal Operator..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE=${NAMESPACE:-vault-operator}
VERSION=${VERSION:-latest}
REGISTRY=${REGISTRY:-ghcr.io}
IMAGE_NAME=${IMAGE_NAME:-panteparak/vault-autounseal-operator}

# Functions
log() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."
    
    if ! command -v kubectl &> /dev/null; then
        error "kubectl is required but not installed"
    fi
    
    if ! kubectl cluster-info &> /dev/null; then
        error "Cannot connect to Kubernetes cluster"
    fi
    
    log "✅ Prerequisites check passed"
}

# Install CRD
install_crd() {
    log "Installing Custom Resource Definition..."
    
    if kubectl get crd vaultunsealconfigs.vault.io &> /dev/null; then
        warn "CRD already exists, updating..."
    fi
    
    kubectl apply -f https://github.com/panteparak/vault-autounseal-operator/releases/latest/download/crd.yaml
    
    log "✅ CRD installed successfully"
}

# Install RBAC
install_rbac() {
    log "Installing RBAC resources..."
    
    kubectl apply -f https://github.com/panteparak/vault-autounseal-operator/releases/latest/download/rbac.yaml
    
    log "✅ RBAC installed successfully"
}

# Install Operator
install_operator() {
    log "Installing operator deployment..."
    
    # Download and modify deployment
    curl -sSL https://github.com/panteparak/vault-autounseal-operator/releases/latest/download/deployment.yaml | \
    sed "s|vault-autounseal-operator:latest|${REGISTRY}/${IMAGE_NAME}:${VERSION}|g" | \
    kubectl apply -f -
    
    log "✅ Operator deployment installed successfully"
}

# Wait for operator to be ready
wait_for_operator() {
    log "Waiting for operator to be ready..."
    
    kubectl wait --for=condition=Available deployment/vault-autounseal-operator -n ${NAMESPACE} --timeout=300s
    
    if kubectl get pods -n ${NAMESPACE} -l app=vault-autounseal-operator | grep -q Running; then
        log "✅ Operator is running successfully"
    else
        error "Operator failed to start"
    fi
}

# Show status
show_status() {
    log "Installation completed! 🎉"
    echo
    echo -e "${BLUE}Operator Status:${NC}"
    kubectl get pods -n ${NAMESPACE} -l app=vault-autounseal-operator
    echo
    echo -e "${BLUE}Next Steps:${NC}"
    echo "1. Create a VaultUnsealConfig resource"
    echo "2. Check the examples: https://github.com/panteparak/vault-autounseal-operator/tree/main/examples"
    echo "3. Monitor with: kubectl get vaultunsealconfigs -A"
    echo
    echo -e "${BLUE}Documentation:${NC} https://panteparak.github.io/vault-autounseal-operator/"
    echo -e "${BLUE}Support:${NC} https://github.com/panteparak/vault-autounseal-operator/issues"
}

# Main installation flow
main() {
    echo "🔐 Vault Auto-Unseal Operator Installer"
    echo "========================================="
    echo
    
    check_prerequisites
    install_crd
    install_rbac
    install_operator
    wait_for_operator
    show_status
}

# Handle command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        --version)
            VERSION="$2"
            shift 2
            ;;
        --registry)
            REGISTRY="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [options]"
            echo
            echo "Options:"
            echo "  --namespace NAMESPACE    Kubernetes namespace (default: vault-operator)"
            echo "  --version VERSION        Operator version (default: latest)"
            echo "  --registry REGISTRY      Container registry (default: ghcr.io)"
            echo "  --help                   Show this help message"
            echo
            echo "Environment Variables:"
            echo "  NAMESPACE               Same as --namespace"
            echo "  VERSION                 Same as --version"
            echo "  REGISTRY                Same as --registry"
            echo
            exit 0
            ;;
        *)
            error "Unknown option: $1"
            ;;
    esac
done

# Run main installation
main