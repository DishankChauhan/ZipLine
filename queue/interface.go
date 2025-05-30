package queue

import (
	"context"
	"time"

	"github.com/DishankChauhan/ZipLine/models"
)

// Queue defines the interface for job queue backends
type Queue interface {
	// Enqueue adds a job to the queue
	Enqueue(ctx context.Context, queueName string, job *models.Job) error
	
	// EnqueueDelayed adds a job to be processed at a specific time
	EnqueueDelayed(ctx context.Context, queueName string, job *models.Job, executeAt time.Time) error
	
	// Dequeue retrieves the next job from the queue
	Dequeue(ctx context.Context, queueName string) (*models.Job, error)
	
	// DequeueBatch retrieves multiple jobs from the queue
	DequeueBatch(ctx context.Context, queueName string, batchSize int) ([]*models.Job, error)
	
	// Ack acknowledges that a job has been processed successfully
	Ack(ctx context.Context, queueName string, job *models.Job) error
	
	// Nack indicates that a job failed and should be retried or dead-lettered
	Nack(ctx context.Context, queueName string, job *models.Job, reason string) error
	
	// GetJob retrieves a specific job by ID
	GetJob(ctx context.Context, jobID string) (*models.Job, error)
	
	// UpdateJob updates a job's status and metadata
	UpdateJob(ctx context.Context, job *models.Job) error
	
	// DeleteJob removes a job from the queue
	DeleteJob(ctx context.Context, jobID string) error
	
	// GetQueueStats returns statistics about a queue
	GetQueueStats(ctx context.Context, queueName string) (*QueueStats, error)
	
	// ListQueues returns all available queue names
	ListQueues(ctx context.Context) ([]string, error)
	
	// GetDelayedJobs returns jobs scheduled for future execution
	GetDelayedJobs(ctx context.Context, limit int) ([]*models.Job, error)
	
	// GetFailedJobs returns jobs that have failed
	GetFailedJobs(ctx context.Context, queueName string, limit int) ([]*models.Job, error)
	
	// PurgeQueue removes all jobs from a queue
	PurgeQueue(ctx context.Context, queueName string) error
	
	// Close closes the queue connection
	Close() error
	
	// HealthCheck verifies the queue backend is healthy
	HealthCheck(ctx context.Context) error
}

// QueueStats holds statistics about a queue
type QueueStats struct {
	QueueName    string    `json:"queue_name"`
	PendingJobs  int64     `json:"pending_jobs"`
	ProcessingJobs int64   `json:"processing_jobs"`
	CompletedJobs int64    `json:"completed_jobs"`
	FailedJobs   int64     `json:"failed_jobs"`
	DelayedJobs  int64     `json:"delayed_jobs"`
	DeadLetterJobs int64   `json:"dead_letter_jobs"`
	LastUpdated  time.Time `json:"last_updated"`
}

// JobProcessor defines how jobs should be processed
type JobProcessor interface {
	// ProcessJob processes a single job
	ProcessJob(ctx context.Context, job *models.Job) error
	
	// CanProcess checks if this processor can handle the job type
	CanProcess(jobType string) bool
	
	// GetName returns the processor name
	GetName() string
}

// Registry holds job processors
type Registry struct {
	processors map[string]JobProcessor
}

// NewRegistry creates a new processor registry
func NewRegistry() *Registry {
	return &Registry{
		processors: make(map[string]JobProcessor),
	}
}

// Register registers a job processor for a specific job type
func (r *Registry) Register(jobType string, processor JobProcessor) {
	r.processors[jobType] = processor
}

// GetProcessor returns a processor for the given job type
func (r *Registry) GetProcessor(jobType string) (JobProcessor, bool) {
	processor, exists := r.processors[jobType]
	return processor, exists
}

// ListProcessors returns all registered processors
func (r *Registry) ListProcessors() map[string]JobProcessor {
	result := make(map[string]JobProcessor)
	for k, v := range r.processors {
		result[k] = v
	}
	return result
} 