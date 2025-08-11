#!/bin/bash

# Optimized Docker build script for vault-autounseal-operator
# This script demonstrates best practices for fast, cached Docker builds

set -euo pipefail

# Configuration
REGISTRY="${REGISTRY:-ghcr.io}"
IMAGE_NAME="${IMAGE_NAME:-panteparak/vault-autounseal-operator}"
VERSION="${VERSION:-$(git describe --tags --always --dirty)}"
BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
GIT_COMMIT="${GIT_COMMIT:-$(git rev-parse HEAD)}"
PLATFORM="${PLATFORM:-linux/amd64,linux/arm64}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
    exit 1
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

# Check if required tools are available
check_prerequisites() {
    log "Checking prerequisites..."

    if ! command -v docker &> /dev/null; then
        error "Docker is not installed or not in PATH"
    fi

    if ! command -v git &> /dev/null; then
        error "Git is not installed or not in PATH"
    fi

    # Check if BuildKit is enabled
    if [[ "${DOCKER_BUILDKIT:-}" != "1" ]]; then
        warn "DOCKER_BUILDKIT is not enabled. Export DOCKER_BUILDKIT=1 for optimal caching"
        export DOCKER_BUILDKIT=1
    fi

    success "Prerequisites check completed"
}

# Build the Docker image with optimal caching
build_image() {
    local target="${1:-production}"
    local push="${2:-false}"

    log "Building ${target} image with version ${VERSION}..."
    log "Platform: ${PLATFORM}"
    log "Registry: ${REGISTRY}"

    # Prepare build arguments
    local build_args=(
        "--build-arg" "VERSION=${VERSION}"
        "--build-arg" "BUILD_TIME=${BUILD_TIME}"
        "--build-arg" "GIT_COMMIT=${GIT_COMMIT}"
        "--target" "${target}"
    )

    # Add platform specification for multi-platform builds
    if [[ "${target}" == "production" && "${push}" == "true" ]]; then
        build_args+=("--platform" "${PLATFORM}")
    fi

    # Add registry cache for better build performance
    if [[ "${push}" == "true" ]]; then
        build_args+=(
            "--cache-from" "type=registry,ref=${REGISTRY}/${IMAGE_NAME}:buildcache"
            "--cache-to" "type=registry,ref=${REGISTRY}/${IMAGE_NAME}:buildcache,mode=max"
        )
    fi

    # Tag configuration
    local tags=(
        "--tag" "${REGISTRY}/${IMAGE_NAME}:${VERSION}"
    )

    if [[ "${target}" == "production" ]]; then
        tags+=("--tag" "${REGISTRY}/${IMAGE_NAME}:latest")
    elif [[ "${target}" == "development" ]]; then
        tags+=("--tag" "${REGISTRY}/${IMAGE_NAME}:dev")
    fi

    # Build command
    local docker_cmd=(
        "docker" "buildx" "build"
        "${build_args[@]}"
        "${tags[@]}"
        "."
    )

    if [[ "${push}" == "true" ]]; then
        docker_cmd+=("--push")
    else
        docker_cmd+=("--load")
    fi

    log "Running: ${docker_cmd[*]}"

    # Execute build with timing
    local start_time=$(date +%s)
    "${docker_cmd[@]}"
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))

    success "Built ${target} image in ${duration} seconds"
}

# Test the built image
test_image() {
    local image_tag="${REGISTRY}/${IMAGE_NAME}:${VERSION}"

    log "Testing built image ${image_tag}..."

    # Basic smoke test
    if docker run --rm "${image_tag}" --version &>/dev/null; then
        success "Image smoke test passed"
    else
        warn "Image smoke test failed (this might be expected if --version is not implemented)"
    fi

    # Check image size
    local image_size
    image_size=$(docker image inspect "${image_tag}" --format='{{.Size}}' | numfmt --to=iec-i --suffix=B)
    log "Image size: ${image_size}"

    # Security scan (if available)
    if command -v docker &> /dev/null && docker version --format '{{.Server.Version}}' | grep -q '^2[0-9]'; then
        log "Running security scan..."
        docker scout cves "${image_tag}" 2>/dev/null || warn "Security scan not available"
    fi
}

# Clean up build cache and unused images
cleanup() {
    log "Cleaning up build cache..."

    # Remove dangling images
    docker image prune -f

    # Remove build cache (optional)
    if [[ "${CLEAN_CACHE:-false}" == "true" ]]; then
        docker buildx prune -f
        success "Build cache cleared"
    else
        log "Build cache preserved for faster subsequent builds"
    fi
}

# Show image information
show_info() {
    local image_tag="${REGISTRY}/${IMAGE_NAME}:${VERSION}"

    log "Image Information:"
    echo "  Repository: ${REGISTRY}/${IMAGE_NAME}"
    echo "  Version: ${VERSION}"
    echo "  Build Time: ${BUILD_TIME}"
    echo "  Git Commit: ${GIT_COMMIT}"
    echo "  Full Tag: ${image_tag}"

    if docker image inspect "${image_tag}" &>/dev/null; then
        local created
        local size
        created=$(docker image inspect "${image_tag}" --format='{{.Created}}')
        size=$(docker image inspect "${image_tag}" --format='{{.Size}}' | numfmt --to=iec-i --suffix=B)
        echo "  Created: ${created}"
        echo "  Size: ${size}"
    fi
}

# Main execution
main() {
    local target="production"
    local push=false
    local test=false
    local cleanup_after=false

    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --target)
                target="$2"
                shift 2
                ;;
            --push)
                push=true
                shift
                ;;
            --test)
                test=true
                shift
                ;;
            --cleanup)
                cleanup_after=true
                shift
                ;;
            --dev|--development)
                target="development"
                shift
                ;;
            --version)
                VERSION="$2"
                shift 2
                ;;
            --help|-h)
                echo "Usage: $0 [OPTIONS]"
                echo ""
                echo "Options:"
                echo "  --target TARGET     Build target (production, development, verify)"
                echo "  --push              Push image to registry"
                echo "  --test              Test built image"
                echo "  --cleanup           Clean up after build"
                echo "  --dev               Build development image"
                echo "  --version VERSION   Override version tag"
                echo "  --help              Show this help"
                echo ""
                echo "Environment Variables:"
                echo "  REGISTRY            Container registry (default: ghcr.io)"
                echo "  IMAGE_NAME          Image name (default: panteparak/vault-autounseal-operator)"
                echo "  VERSION             Image version (default: git describe)"
                echo "  PLATFORM            Target platforms (default: linux/amd64,linux/arm64)"
                echo "  CLEAN_CACHE         Clean build cache after build (default: false)"
                exit 0
                ;;
            *)
                error "Unknown option: $1"
                ;;
        esac
    done

    # Execute build process
    check_prerequisites
    build_image "${target}" "${push}"

    if [[ "${test}" == "true" ]]; then
        test_image
    fi

    if [[ "${cleanup_after}" == "true" ]]; then
        cleanup
    fi

    show_info
    success "Build process completed successfully!"
}

# Trap for cleanup on error
trap 'error "Build failed!"' ERR

# Run main function
main "$@"
