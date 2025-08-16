#!/bin/bash

# Integration Test Runner Script
# This script runs Go-based integration tests using Testcontainers

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
TEST_TIMEOUT=${TEST_TIMEOUT:-"30m"}
VERBOSE=${VERBOSE:-"false"}
PACKAGE=${PACKAGE:-"./pkg/vault/..."}
RACE_DETECTOR=${RACE_DETECTOR:-"-race"}
TAGS=${TAGS:-"integration"}
RUN_PATTERN=${RUN_PATTERN:-"TestVaultIntegrationSuite"}

# Print banner
print_banner() {
    echo -e "${CYAN}"
    echo "=============================================="
    echo "🧪 Go Integration Test Runner"
    echo "=============================================="
    echo -e "${NC}"
}

# Print usage
usage() {
    echo -e "${YELLOW}Usage: $0 [OPTIONS]${NC}"
    echo ""
    echo -e "${BLUE}Options:${NC}"
    echo "  -t, --timeout TIME     Set test timeout (default: 30m)"
    echo "  -v, --verbose          Enable verbose output"
    echo "  -p, --package PATTERN  Set package pattern (default: ./pkg/vault/...)"
    echo "  -r, --run PATTERN      Set test run pattern (default: TestVaultIntegrationSuite)"
    echo "  -s, --short            Run only short tests"
    echo "  -n, --no-race          Disable race detector"
    echo "  -c, --coverage         Generate coverage report"
    echo "  -h, --help             Show this help message"
    echo ""
    echo -e "${BLUE}Examples:${NC}"
    echo "  $0                                    # Run all integration tests"
    echo "  $0 -v -t 60m                         # Verbose with 60 minute timeout"
    echo "  $0 -r 'TestBasicVaultOperations'     # Run specific test"
    echo "  $0 -s                                # Run only short tests"
    echo "  $0 -c                                # Generate coverage report"
}

# Check dependencies
check_dependencies() {
    echo -e "${BLUE}🔍 Checking dependencies...${NC}"

    # Check Go
    if ! command -v go &> /dev/null; then
        echo -e "${RED}❌ Go is not installed or not in PATH${NC}"
        exit 1
    fi

    # Check Docker
    if ! command -v docker &> /dev/null; then
        echo -e "${RED}❌ Docker is not installed or not in PATH${NC}"
        exit 1
    fi

    # Check if Docker daemon is running
    if ! docker info &> /dev/null; then
        echo -e "${RED}❌ Docker daemon is not running${NC}"
        exit 1
    fi

    echo -e "${GREEN}✅ All dependencies are available${NC}"
}

# Download Go module dependencies
download_dependencies() {
    echo -e "${BLUE}📦 Downloading Go module dependencies...${NC}"

    go mod download
    go mod verify

    echo -e "${GREEN}✅ Dependencies downloaded${NC}"
}

# Run the tests
run_tests() {
    echo -e "${BLUE}🧪 Running integration tests...${NC}"
    echo -e "${BLUE}Package: ${PACKAGE}${NC}"
    echo -e "${BLUE}Timeout: ${TEST_TIMEOUT}${NC}"
    echo -e "${BLUE}Pattern: ${RUN_PATTERN}${NC}"

    # Build test command
    test_cmd="go test"

    # Add flags
    if [ "$VERBOSE" = "true" ]; then
        test_cmd="$test_cmd -v"
    fi

    if [ "$RACE_DETECTOR" = "-race" ]; then
        test_cmd="$test_cmd -race"
    fi

    if [ "$SHORT_TESTS" = "true" ]; then
        test_cmd="$test_cmd -short"
    fi

    if [ "$COVERAGE" = "true" ]; then
        test_cmd="$test_cmd -coverprofile=integration-coverage.out"
    fi

    test_cmd="$test_cmd -timeout=$TEST_TIMEOUT"
    test_cmd="$test_cmd -tags=$TAGS"
    test_cmd="$test_cmd -run=\"$RUN_PATTERN\""
    test_cmd="$test_cmd $PACKAGE"

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

    # Show coverage if generated
    if [ "$COVERAGE" = "true" ] && [ -f "integration-coverage.out" ]; then
        echo -e "${BLUE}📊 Coverage report generated: integration-coverage.out${NC}"
        coverage_percent=$(go tool cover -func=integration-coverage.out | grep total | awk '{print $3}')
        echo -e "${BLUE}📈 Total coverage: ${coverage_percent}${NC}"
    fi

    return $exit_code
}

# Parse command line arguments
SHORT_TESTS="false"
COVERAGE="false"

while [[ $# -gt 0 ]]; do
    case $1 in
        -t|--timeout)
            TEST_TIMEOUT="$2"
            shift 2
            ;;
        -v|--verbose)
            VERBOSE="true"
            shift
            ;;
        -p|--package)
            PACKAGE="$2"
            shift 2
            ;;
        -r|--run)
            RUN_PATTERN="$2"
            shift 2
            ;;
        -s|--short)
            SHORT_TESTS="true"
            shift
            ;;
        -n|--no-race)
            RACE_DETECTOR=""
            shift
            ;;
        -c|--coverage)
            COVERAGE="true"
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

# Check environment
check_dependencies
download_dependencies

# Run tests
if run_tests; then
    echo -e "${GREEN}🎉 Integration tests completed successfully!${NC}"
    exit_code=0
else
    echo -e "${RED}💥 Integration tests failed!${NC}"
    exit_code=1
fi

exit $exit_code
