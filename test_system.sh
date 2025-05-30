#!/bin/bash

# Test script for Job Queue System
echo "=== Job Queue System Test Script ==="
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓${NC} $2"
    else
        echo -e "${RED}✗${NC} $2"
    fi
}

print_info() {
    echo -e "${YELLOW}ℹ${NC} $1"
}

# Check if Go is installed
echo "1. Checking Go installation..."
if command -v go &> /dev/null; then
    GO_VERSION=$(go version)
    print_status 0 "Go is installed: $GO_VERSION"
else
    print_status 1 "Go is not installed. Please install Go 1.21+ from https://golang.org/dl/"
    echo
    print_info "To install Go on macOS: brew install go"
    print_info "To install Go on Ubuntu: sudo apt install golang-go"
    echo
fi

# Check if Docker is installed
echo "2. Checking Docker installation..."
if command -v docker &> /dev/null; then
    DOCKER_VERSION=$(docker --version)
    print_status 0 "Docker is installed: $DOCKER_VERSION"
    
    # Check if Docker is running
    if docker ps &> /dev/null; then
        print_status 0 "Docker daemon is running"
    else
        print_status 1 "Docker daemon is not running. Please start Docker."
    fi
else
    print_status 1 "Docker is not installed. Please install Docker from https://docker.com/"
fi

# Check if Docker Compose is installed
echo "3. Checking Docker Compose installation..."
if command -v docker-compose &> /dev/null; then
    COMPOSE_VERSION=$(docker-compose --version)
    print_status 0 "Docker Compose is installed: $COMPOSE_VERSION"
elif docker compose version &> /dev/null; then
    COMPOSE_VERSION=$(docker compose version)
    print_status 0 "Docker Compose (plugin) is installed: $COMPOSE_VERSION"
else
    print_status 1 "Docker Compose is not installed"
fi

# Check project structure
echo "4. Checking project structure..."
REQUIRED_FILES=(
    "go.mod"
    "cmd/main.go"
    "api/server.go"
    "queue/interface.go"
    "queue/redis.go"
    "worker/pool.go"
    "scheduler/scheduler.go"
    "models/job.go"
    "config/config.go"
    "Dockerfile"
    "docker-compose.yml"
    "README.md"
)

for file in "${REQUIRED_FILES[@]}"; do
    if [ -f "$file" ]; then
        print_status 0 "Found $file"
    else
        print_status 1 "Missing $file"
    fi
done

echo
echo "=== Test Commands ==="
print_info "If all checks pass, you can run:"
echo "  • make help           - See all available commands"
echo "  • make docker-run     - Start with Docker Compose"
echo "  • make run-redis      - Start only Redis"
echo "  • make run            - Run locally (requires Go and Redis)"
echo "  • make test-api       - Test API endpoints"
echo

print_info "Example API calls:"
echo "  • Health check: curl http://localhost:8080/health"
echo "  • Submit job: curl -X POST http://localhost:8080/api/v1/jobs -H 'Content-Type: application/json' -d '{\"name\":\"test\",\"type\":\"send_email\",\"payload\":{\"email\":\"test@example.com\"}}'"
echo "  • Get stats: curl http://localhost:8080/api/v1/stats"
echo

echo "=== Installation Instructions ==="
print_info "If you don't have Go installed:"
echo "  macOS: brew install go"
echo "  Ubuntu: sudo apt install golang-go"
echo "  Other: https://golang.org/dl/"
echo

print_info "If you don't have Docker installed:"
echo "  macOS: brew install --cask docker"
echo "  Ubuntu: sudo apt install docker.io docker-compose"
echo "  Other: https://docs.docker.com/get-docker/"
echo

echo "=== Next Steps ==="
print_info "1. Install missing dependencies (Go, Docker)"
print_info "2. Run 'make docker-run' to start the system"
print_info "3. Test with 'curl http://localhost:8080/health'"
print_info "4. Submit jobs using the API"
print_info "5. Monitor with 'make docker-logs'" 