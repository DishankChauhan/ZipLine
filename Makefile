.PHONY: build run test clean docker-build docker-run deps fmt lint help

# Variables
BINARY_NAME=zipline
DOCKER_IMAGE=zipline
VERSION?=latest

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
build: ## Build the application binary
	@echo "Building $(BINARY_NAME)..."
	go build -o bin/$(BINARY_NAME) cmd/main.go

build-linux: ## Build for Linux
	@echo "Building $(BINARY_NAME) for Linux..."
	GOOS=linux GOARCH=amd64 go build -o bin/$(BINARY_NAME)-linux cmd/main.go

# Run targets
run: ## Run the application locally
	@echo "Running $(BINARY_NAME)..."
	go run cmd/main.go

run-redis: ## Start Redis using Docker
	@echo "Starting Redis..."
	docker run -d --name redis-jqs -p 6379:6379 redis:7-alpine

stop-redis: ## Stop Redis container
	@echo "Stopping Redis..."
	docker stop redis-jqs || true
	docker rm redis-jqs || true

# Development targets
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

fmt: ## Format code
	@echo "Formatting code..."
	go fmt ./...

lint: ## Run linter
	@echo "Running linter..."
	golangci-lint run ./...

test: ## Run tests
	@echo "Running tests..."
	go test -v ./...

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Docker targets
docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(VERSION) .

docker-run: ## Run application using Docker Compose
	@echo "Starting application with Docker Compose..."
	docker-compose up -d

docker-stop: ## Stop Docker Compose services
	@echo "Stopping Docker Compose services..."
	docker-compose down

docker-logs: ## Show Docker Compose logs
	@echo "Showing logs..."
	docker-compose logs -f

# Cleanup targets
clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html

clean-docker: ## Clean Docker images and containers
	@echo "Cleaning Docker..."
	docker-compose down --rmi all --volumes --remove-orphans

# API testing targets
test-api: ## Test API endpoints
	@echo "Testing API endpoints..."
	@echo "Submitting test job..."
	curl -X POST http://localhost:8080/api/v1/jobs \
	  -H "Content-Type: application/json" \
	  -d '{"name":"test_job","type":"send_email","payload":{"email":"test@example.com","subject":"Test","body":"Test email"}}'

test-health: ## Test health endpoint
	@echo "Testing health endpoint..."
	curl http://localhost:8080/health

test-stats: ## Test stats endpoints
	@echo "Testing stats endpoints..."
	curl http://localhost:8080/api/v1/stats

# Load testing
load-test: ## Run load test with 100 jobs
	@echo "Running load test..."
	@for i in $$(seq 1 100); do \
		curl -s -X POST http://localhost:8080/api/v1/jobs \
		  -H "Content-Type: application/json" \
		  -d "{\"name\":\"load_test_$$i\",\"type\":\"send_email\",\"payload\":{\"email\":\"test$$i@example.com\",\"subject\":\"Load Test $$i\"}}" \
		  > /dev/null; \
	done
	@echo "Submitted 100 test jobs"

# Development workflow
dev: deps fmt build run ## Complete development workflow

# Production targets
release: clean fmt lint test build-linux ## Prepare release build

# Initialize project
init: ## Initialize development environment
	@echo "Initializing development environment..."
	$(MAKE) deps
	$(MAKE) run-redis
	@echo "Development environment ready!"
	@echo "Run 'make run' to start the application" 