package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// JobStatus represents the current state of a job
type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusSuccess    JobStatus = "success"
	StatusFailed     JobStatus = "failed"
	StatusRetrying   JobStatus = "retrying"
	StatusDeadLetter JobStatus = "dead_letter"
	StatusCancelled  JobStatus = "cancelled"
)

// BackoffType defines the retry backoff strategy
type BackoffType string

const (
	BackoffLinear      BackoffType = "linear"
	BackoffExponential BackoffType = "exponential"
	BackoffFixed       BackoffType = "fixed"
)

// RetryPolicy defines how job retries should be handled
type RetryPolicy struct {
	MaxRetries   int         `json:"max_retries"`
	BackoffType  BackoffType `json:"backoff_type"`
	InitialDelay time.Duration `json:"initial_delay"`
	MaxDelay     time.Duration `json:"max_delay"`
}

// Job represents a single unit of work in the queue
type Job struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Payload     map[string]interface{} `json:"payload"`
	Status      JobStatus              `json:"status"`
	Priority    int                    `json:"priority"` // Higher number = higher priority
	RetryPolicy RetryPolicy            `json:"retry_policy"`
	
	// Execution metadata
	AttemptCount    int        `json:"attempt_count"`
	LastAttemptAt   *time.Time `json:"last_attempt_at,omitempty"`
	ProcessedAt     *time.Time `json:"processed_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	FailedAt        *time.Time `json:"failed_at,omitempty"`
	
	// Scheduling
	ScheduledAt   *time.Time `json:"scheduled_at,omitempty"`
	ExecuteAfter  *time.Time `json:"execute_after,omitempty"`
	
	// Error tracking
	LastError     string   `json:"last_error,omitempty"`
	ErrorHistory  []string `json:"error_history,omitempty"`
	
	// Metadata
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	CreatedBy     string                 `json:"created_by,omitempty"`
	Tags          map[string]string      `json:"tags,omitempty"`
}

// NewJob creates a new job with default values
func NewJob(name, jobType string, payload map[string]interface{}) *Job {
	now := time.Now()
	return &Job{
		ID:          uuid.New().String(),
		Name:        name,
		Type:        jobType,
		Payload:     payload,
		Status:      StatusPending,
		Priority:    0,
		RetryPolicy: RetryPolicy{
			MaxRetries:   3,
			BackoffType:  BackoffExponential,
			InitialDelay: 5 * time.Second,
			MaxDelay:     5 * time.Minute,
		},
		AttemptCount: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
		Tags:         make(map[string]string),
	}
}

// ToJSON serializes the job to JSON
func (j *Job) ToJSON() ([]byte, error) {
	return json.Marshal(j)
}

// FromJSON deserializes a job from JSON
func (j *Job) FromJSON(data []byte) error {
	return json.Unmarshal(data, j)
}

// IsRetryable checks if the job can be retried
func (j *Job) IsRetryable() bool {
	return j.AttemptCount < j.RetryPolicy.MaxRetries && 
		   j.Status != StatusSuccess && 
		   j.Status != StatusCancelled &&
		   j.Status != StatusDeadLetter
}

// NextRetryAt calculates when the job should be retried next
func (j *Job) NextRetryAt() time.Time {
	delay := j.RetryPolicy.InitialDelay
	
	switch j.RetryPolicy.BackoffType {
	case BackoffExponential:
		for i := 0; i < j.AttemptCount; i++ {
			delay *= 2
		}
	case BackoffLinear:
		delay = time.Duration(j.AttemptCount+1) * j.RetryPolicy.InitialDelay
	case BackoffFixed:
		// delay remains the same
	}
	
	if delay > j.RetryPolicy.MaxDelay {
		delay = j.RetryPolicy.MaxDelay
	}
	
	return time.Now().Add(delay)
}

// MarkProcessing marks the job as currently being processed
func (j *Job) MarkProcessing() {
	j.Status = StatusProcessing
	now := time.Now()
	j.ProcessedAt = &now
	j.UpdatedAt = now
	j.AttemptCount++
}

// MarkSuccess marks the job as successfully completed
func (j *Job) MarkSuccess() {
	j.Status = StatusSuccess
	now := time.Now()
	j.CompletedAt = &now
	j.UpdatedAt = now
}

// MarkFailed marks the job as failed with an error message
func (j *Job) MarkFailed(errorMsg string) {
	j.Status = StatusFailed
	j.LastError = errorMsg
	j.ErrorHistory = append(j.ErrorHistory, errorMsg)
	now := time.Now()
	j.FailedAt = &now
	j.LastAttemptAt = &now
	j.UpdatedAt = now
}

// MarkRetrying marks the job for retry
func (j *Job) MarkRetrying(errorMsg string) {
	j.Status = StatusRetrying
	j.LastError = errorMsg
	j.ErrorHistory = append(j.ErrorHistory, errorMsg)
	now := time.Now()
	j.LastAttemptAt = &now
	j.UpdatedAt = now
}

// MarkDeadLetter marks the job as permanently failed
func (j *Job) MarkDeadLetter() {
	j.Status = StatusDeadLetter
	j.UpdatedAt = time.Now()
}

// MarkCancelled marks the job as cancelled
func (j *Job) MarkCancelled() {
	j.Status = StatusCancelled
	j.UpdatedAt = time.Now()
} 