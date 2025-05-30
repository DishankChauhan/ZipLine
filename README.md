# ZipLine - High-Performance Job Queue System

A scalable, reliable, and fast job queue system built in Go by **Dishank Chauhan**. ZipLine is similar to Sidekiq (Ruby) or Bull (Node.js) and is designed as a developer tool or infrastructure component that can be embedded into modern microservices.

## 🚀 Features

- **High Performance**: Built with Go's goroutines for concurrent job processing
- **Redis Backend**: Uses Redis for reliable job storage and queuing
- **Retry Logic**: Configurable retry policies with exponential backoff
- **Delayed Jobs**: Schedule jobs for future execution
- **Priority Queues**: Support for job prioritization
- **REST API**: Complete RESTful API for job management
- **Metrics**: Built-in Prometheus metrics for monitoring
- **Graceful Shutdown**: Proper cleanup and graceful shutdown handling
- **Docker Ready**: Containerized for easy deployment

## 📋 Requirements

- Go 1.21+
- Redis 6.0+
- Docker & Docker Compose (optional)

## 🛠️ Quick Start

### Using Docker Compose (Recommended)

1. Clone the repository:
```bash
git clone https://github.com/DishankChauhan/ZipLine.git
cd ZipLine
```

2. Start the system:
```bash
docker-compose up -d
```

3. Verify the system is running:
```bash
curl http://localhost:8080/health
```

### Manual Installation

1. Install dependencies:
```bash
go mod download
```

2. Start Redis:
```bash
# Using Docker
docker run -d -p 6379:6379 redis:7-alpine

# Or install Redis locally
# macOS: brew install redis && redis-server
# Ubuntu: sudo apt install redis-server && redis-server
```

3. Run the application:
```bash
go run cmd/main.go
```

## 📖 API Documentation

### Submit a Job

**POST** `/api/v1/jobs`

```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "send_welcome_email",
    "type": "send_email",
    "payload": {
      "email": "user@example.com",
      "subject": "Welcome to ZipLine!",
      "body": "Welcome to our high-performance job queue system!"
    },
    "priority": 5,
    "queue": "default",
    "retry": {
      "max_retries": 3,
      "backoff": "exponential",
      "initial_delay": "5s",
      "max_delay": "5m"
    }
  }'
```

### Submit a Delayed Job

```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "reminder_email",
    "type": "send_email",
    "payload": {
      "email": "user@example.com",
      "subject": "ZipLine Reminder",
      "body": "This is your scheduled reminder from ZipLine!"
    },
    "schedule_at": "2024-12-25T10:00:00Z"
  }'
```

### Get Job Status

**GET** `/api/v1/jobs/{job_id}`

```bash
curl http://localhost:8080/api/v1/jobs/{job_id}
```

### Cancel a Job

**POST** `/api/v1/jobs/{job_id}/cancel`

```bash
curl -X POST http://localhost:8080/api/v1/jobs/{job_id}/cancel
```

### Retry a Failed Job

**POST** `/api/v1/jobs/{job_id}/retry`

```bash
curl -X POST http://localhost:8080/api/v1/jobs/{job_id}/retry
```

### Get Queue Statistics

**GET** `/api/v1/queues/{queue_name}/stats`

```bash
curl http://localhost:8080/api/v1/queues/default/stats
```

### Get System Statistics

**GET** `/api/v1/stats`

```bash
curl http://localhost:8080/api/v1/stats
```

### Get Worker Statistics

**GET** `/api/v1/workers/stats`

```bash
curl http://localhost:8080/api/v1/workers/stats
```

## 🔧 Configuration

The system can be configured via environment variables or a YAML configuration file.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `JQS_REDIS_HOST` | `localhost` | Redis host |
| `JQS_REDIS_PORT` | `6379` | Redis port |
| `JQS_REDIS_PASSWORD` | `` | Redis password |
| `JQS_REDIS_DATABASE` | `0` | Redis database |
| `JQS_SERVER_HOST` | `0.0.0.0` | API server host |
| `JQS_SERVER_PORT` | `8080` | API server port |
| `JQS_WORKER_COUNT` | `4` | Number of worker goroutines |
| `JQS_WORKER_MAX_CONCURRENCY` | `100` | Max concurrent jobs |
| `JQS_WORKER_POLL_INTERVAL` | `1s` | Job polling interval |
| `JQS_SCHEDULER_ENABLED` | `true` | Enable job scheduler |
| `JQS_SCHEDULER_POLL_INTERVAL` | `30s` | Scheduler polling interval |
| `JQS_METRICS_ENABLED` | `true` | Enable metrics |
| `JQS_METRICS_PORT` | `9091` | Metrics server port |

### Configuration File

Create a `config.yaml` file:

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  grpc_port: 9090

redis:
  host: "localhost"
  port: 6379
  password: ""
  database: 0

worker:
  count: 4
  max_concurrency: 100
  poll_interval: "1s"
  job_timeout: "5m"

scheduler:
  enabled: true
  poll_interval: "30s"
  batch_size: 100

metrics:
  enabled: true
  port: 9091
  path: "/metrics"

logging:
  level: "info"
  format: "json"
```

## 🔌 Creating Custom Job Processors

To add custom job types, implement the `JobProcessor` interface:

```go
package main

import (
    "context"
    "job-queue-system/models"
    "job-queue-system/queue"
)

type CustomProcessor struct{}

func (p *CustomProcessor) ProcessJob(ctx context.Context, job *models.Job) error {
    // Your custom job processing logic here
    return nil
}

func (p *CustomProcessor) CanProcess(jobType string) bool {
    return jobType == "custom_job"
}

func (p *CustomProcessor) GetName() string {
    return "CustomProcessor"
}

// Register the processor
func main() {
    registry := queue.NewRegistry()
    registry.Register("custom_job", &CustomProcessor{})
    // ... rest of setup
}
```

## 📊 Built-in Job Types

The system comes with three example job processors:

### Email Processor (`send_email`)
```json
{
  "type": "send_email",
  "payload": {
    "email": "user@example.com",
    "subject": "Subject",
    "body": "Email body"
  }
}
```

### Webhook Processor (`webhook`)
```json
{
  "type": "webhook",
  "payload": {
    "url": "https://api.example.com/webhook",
    "method": "POST",
    "data": {"key": "value"}
  }
}
```

### Data Processor (`process_data`)
```json
{
  "type": "process_data",
  "payload": {
    "data_size": 1000,
    "operation": "transform"
  }
}
```

## 📈 Monitoring

### Prometheus Metrics

The system exposes Prometheus metrics at `http://localhost:9091/metrics`:

- `job_queue_processed_total` - Total processed jobs
- `job_queue_failed_total` - Total failed jobs
- `job_queue_duration_seconds` - Job processing duration
- `job_queue_active_jobs` - Currently active jobs
- `job_queue_pending_jobs` - Pending jobs per queue

### Health Check

Check system health at `http://localhost:8080/health`:

```json
{
  "status": "ok",
  "timestamp": "2024-01-15T10:00:00Z",
  "version": "1.0.0",
  "workers": {
    "running": true,
    "count": 4
  }
}
```

## 🧪 Testing

### Unit Tests
```bash
go test ./...
```

### Load Testing
```bash
# Submit 1000 jobs for testing
for i in {1..1000}; do
  curl -X POST http://localhost:8080/api/v1/jobs \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"test_job_$i\",\"type\":\"send_email\",\"payload\":{\"email\":\"test$i@example.com\"}}"
done
```

## 🚀 Deployment

### Production Deployment

1. **Environment Variables**: Set production environment variables
2. **Redis Cluster**: Use Redis Cluster for high availability
3. **Load Balancer**: Put multiple instances behind a load balancer
4. **Monitoring**: Set up Prometheus and Grafana for monitoring
5. **Logging**: Configure structured logging and log aggregation

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: job-queue-system
spec:
  replicas: 3
  selector:
    matchLabels:
      app: job-queue-system
  template:
    metadata:
      labels:
        app: job-queue-system
    spec:
      containers:
      - name: job-queue-system
        image: job-queue-system:latest
        ports:
        - containerPort: 8080
        env:
        - name: JQS_REDIS_HOST
          value: "redis-service"
        - name: JQS_WORKER_COUNT
          value: "8"
```

## 🔧 Development

### Project Structure
```
job-queue-system/
├── cmd/                 # Application entry points
├── api/                 # REST API handlers
├── queue/               # Queue interface and implementations
├── worker/              # Worker pool implementation
├── scheduler/           # Job scheduler
├── models/              # Data models
├── config/              # Configuration management
├── metrics/             # Metrics collection
├── Dockerfile           # Container definition
├── docker-compose.yml   # Local development setup
└── README.md           # This file
```

### Building
```bash
# Build binary
go build -o bin/job-queue-system cmd/main.go

# Build Docker image
docker build -t job-queue-system .
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## 📝 License

This project is licensed under the MIT License. See the LICENSE file for details.

## 🆘 Support

For support and questions:
- Create an issue on GitHub
- Check the documentation
- Review the examples in this README

## 🔮 Roadmap

- [ ] Kafka backend support
- [ ] gRPC API implementation
- [ ] Web dashboard for monitoring
- [ ] CRON-style recurring jobs
- [ ] PostgreSQL persistence layer
- [ ] Job result storage
- [ ] Rate limiting
- [ ] Job dependencies
- [ ] Batch job processing
- [ ] Multi-tenant support