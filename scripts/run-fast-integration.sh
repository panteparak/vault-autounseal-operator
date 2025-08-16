#!/bin/bash

# Fast Integration Test Runner Script
# This script provides various options for running fast-failing integration tests

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Default configuration
DEBUG_LEVEL=${INTEGRATION_DEBUG:-"BASIC"}
TIMEOUT=${GO_TEST_TIMEOUT:-"60s"}
FOCUS=${GINKGO_FOCUS:-""}
DOCKER_COMPOSE_FILE="test/environments/ci/docker-compose.fast-integration.yml"
USE_DOCKER=${USE_DOCKER:-"false"}
LOG_FILE=${INTEGRATION_DEBUG_LOG:-"integration-debug.log"}

# Print banner
print_banner() {
    echo -e "${CYAN}"
    echo "=================================================="
    echo "🚀 Fast-Failing Integration Test Runner"
    echo "=================================================="
    echo -e "${NC}"
}

# Print usage
usage() {
    echo -e "${YELLOW}Usage: $0 [OPTIONS]${NC}"
    echo ""
    echo -e "${BLUE}Options:${NC}"
    echo "  -d, --debug LEVEL    Set debug level (QUIET, BASIC, VERBOSE, TRACE)"
    echo "  -t, --timeout TIME   Set test timeout (default: 60s)"
    echo "  -f, --focus PATTERN  Focus on specific tests (Ginkgo pattern)"
    echo "  -D, --docker         Use Docker Compose for vault services"
    echo "  -l, --log FILE       Debug log file (default: integration-debug.log)"
    echo "  -c, --clean          Clean up Docker containers after tests"
    echo "  -v, --verbose        Enable verbose output"
    echo "  -h, --help           Show this help message"
    echo ""
    echo -e "${BLUE}Examples:${NC}"
    echo "  $0                           # Run with default settings"
    echo "  $0 -d VERBOSE -t 120s        # Verbose debug, 2 minute timeout"
    echo "  $0 -f 'Circuit Breaker'      # Focus on circuit breaker tests"
    echo "  $0 -D -c                     # Use Docker, clean up after"
    echo "  $0 -d TRACE -l trace.log     # Full tracing to custom log file"
}

# Setup Docker environment
setup_docker() {
    echo -e "${BLUE}🐳 Setting up Docker environment...${NC}"

    if ! command -v docker-compose &> /dev/null; then
        echo -e "${RED}❌ docker-compose not found. Please install Docker Compose.${NC}"
        exit 1
    fi

    # Start services
    echo -e "${BLUE}Starting Vault services...${NC}"
    docker-compose -f "$DOCKER_COMPOSE_FILE" up -d vault-dev vault-sealed

    # Wait for services to be healthy
    echo -e "${BLUE}Waiting for services to be ready...${NC}"
    timeout 30s bash -c '
        while ! docker-compose -f '"$DOCKER_COMPOSE_FILE"' ps vault-dev | grep -q "healthy"; do
            echo "Waiting for vault-dev..."
            sleep 2
        done
    '

    timeout 30s bash -c '
        while ! docker-compose -f '"$DOCKER_COMPOSE_FILE"' ps vault-sealed | grep -q "healthy"; do
            echo "Waiting for vault-sealed..."
            sleep 2
        done
    '

    echo -e "${GREEN}✅ Docker services are ready${NC}"
}

# Cleanup Docker environment
cleanup_docker() {
    if [ "$USE_DOCKER" = "true" ]; then
        echo -e "${BLUE}🧹 Cleaning up Docker environment...${NC}"
        docker-compose -f "$DOCKER_COMPOSE_FILE" down -v
        echo -e "${GREEN}✅ Cleanup complete${NC}"
    fi
}

# Run the tests
run_tests() {
    echo -e "${BLUE}🧪 Running fast integration tests...${NC}"
    echo -e "${BLUE}Debug Level: ${DEBUG_LEVEL}${NC}"
    echo -e "${BLUE}Timeout: ${TIMEOUT}${NC}"
    echo -e "${BLUE}Log File: ${LOG_FILE}${NC}"

    # Set environment variables
    export INTEGRATION_DEBUG="$DEBUG_LEVEL"
    export INTEGRATION_DEBUG_LOG="$LOG_FILE"
    export GO_TEST_TIMEOUT="$TIMEOUT"

    # Build test command
    test_cmd="go test -v -tags=integration -timeout=$TIMEOUT"

    # Add focus if specified
    if [ -n "$FOCUS" ]; then
        test_cmd="$test_cmd -ginkgo.focus=\"$FOCUS\""
    fi

    # Add test files
    test_cmd="$test_cmd ./pkg/vault/fast_integration_test.go ./pkg/vault/integration_framework.go ./pkg/vault/integration_debug.go ./pkg/vault/modular_test.go"

    echo -e "${BLUE}Running: $test_cmd${NC}"
    echo ""

    # Run tests and capture exit code
    start_time=$(date +%s)
    if eval "$test_cmd"; then
        exit_code=0
        echo -e "${GREEN}✅ All tests passed!${NC}"
    else
        exit_code=$?
        echo -e "${RED}❌ Tests failed with exit code $exit_code${NC}"
    fi
    end_time=$(date +%s)

    # Calculate duration
    duration=$((end_time - start_time))
    echo -e "${BLUE}⏱️  Total test duration: ${duration}s${NC}"

    # Show debug log info if available
    if [ -f "$LOG_FILE" ]; then
        log_size=$(wc -l < "$LOG_FILE")
        echo -e "${BLUE}📋 Debug log: $LOG_FILE ($log_size lines)${NC}"

        if [ "$DEBUG_LEVEL" = "BASIC" ] || [ "$DEBUG_LEVEL" = "VERBOSE" ] || [ "$DEBUG_LEVEL" = "TRACE" ]; then
            echo -e "${BLUE}📊 Last 10 debug entries:${NC}"
            tail -10 "$LOG_FILE" | while read line; do
                echo -e "${CYAN}  $line${NC}"
            done
        fi
    fi

    return $exit_code
}

# Trap to ensure cleanup
trap cleanup_docker EXIT

# Parse command line arguments
CLEAN_DOCKER="false"
VERBOSE="false"

while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--debug)
            DEBUG_LEVEL="$2"
            shift 2
            ;;
        -t|--timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        -f|--focus)
            FOCUS="$2"
            shift 2
            ;;
        -D|--docker)
            USE_DOCKER="true"
            shift
            ;;
        -l|--log)
            LOG_FILE="$2"
            shift 2
            ;;
        -c|--clean)
            CLEAN_DOCKER="true"
            shift
            ;;
        -v|--verbose)
            VERBOSE="true"
            DEBUG_LEVEL="VERBOSE"
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            usage
            exit 1
            ;;
    esac
done

# Main execution
print_banner

# Validate debug level
case "$DEBUG_LEVEL" in
    QUIET|BASIC|VERBOSE|TRACE)
        ;;
    *)
        echo -e "${RED}❌ Invalid debug level: $DEBUG_LEVEL${NC}"
        echo -e "${YELLOW}Valid levels: QUIET, BASIC, VERBOSE, TRACE${NC}"
        exit 1
        ;;
esac

# Setup Docker if requested
if [ "$USE_DOCKER" = "true" ]; then
    setup_docker
fi

# Run the tests
if run_tests; then
    echo -e "${GREEN}🎉 Integration tests completed successfully!${NC}"
    exit_code=0
else
    echo -e "${RED}💥 Integration tests failed!${NC}"
    exit_code=1
fi

# Clean up Docker if requested
if [ "$CLEAN_DOCKER" = "true" ]; then
    cleanup_docker
fi

exit $exit_code
