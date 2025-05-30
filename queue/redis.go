package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/DishankChauhan/ZipLine/config"
	"github.com/DishankChauhan/ZipLine/models"
)

const (
	// Redis key patterns
	queueKey         = "jqs:queue:%s"           // List for pending jobs
	processingKey    = "jqs:processing:%s"      // Set for jobs being processed
	delayedKey       = "jqs:delayed"            // Sorted set for delayed jobs
	failedKey        = "jqs:failed:%s"          // List for failed jobs
	deadLetterKey    = "jqs:dead_letter:%s"     // List for dead letter jobs
	jobKey           = "jqs:job:%s"             // Hash for job data
	statsKey         = "jqs:stats:%s"           // Hash for queue statistics
	queuesKey        = "jqs:queues"             // Set for tracking queue names
)

// RedisQueue implements the Queue interface using Redis
type RedisQueue struct {
	client *redis.Client
	config *config.RedisConfig
}

// NewRedisQueue creates a new Redis queue instance
func NewRedisQueue(cfg *config.RedisConfig) (*RedisQueue, error) {
	client := redis.NewClient(&redis.Options{
		Addr:        cfg.RedisAddr(),
		Password:    cfg.Password,
		DB:          cfg.Database,
		MaxRetries:  cfg.MaxRetries,
		PoolSize:    cfg.PoolSize,
		DialTimeout: cfg.DialTimeout,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisQueue{
		client: client,
		config: cfg,
	}, nil
}

// Enqueue adds a job to the queue
func (r *RedisQueue) Enqueue(ctx context.Context, queueName string, job *models.Job) error {
	// Serialize job
	jobData, err := job.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize job: %w", err)
	}

	pipe := r.client.TxPipeline()

	// Store job data
	pipe.HSet(ctx, fmt.Sprintf(jobKey, job.ID), map[string]interface{}{
		"data":       string(jobData),
		"queue":      queueName,
		"created_at": job.CreatedAt.Unix(),
		"status":     string(job.Status),
	})

	// Add to queue (priority-based using sorted set)
	queueKeyName := fmt.Sprintf(queueKey, queueName)
	score := float64(-job.Priority) // Negative for descending order (higher priority first)
	pipe.ZAdd(ctx, queueKeyName, &redis.Z{
		Score:  score,
		Member: job.ID,
	})

	// Track queue name
	pipe.SAdd(ctx, queuesKey, queueName)

	// Update stats
	pipe.HIncrBy(ctx, fmt.Sprintf(statsKey, queueName), "pending_jobs", 1)
	pipe.HSet(ctx, fmt.Sprintf(statsKey, queueName), "last_updated", time.Now().Unix())

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to enqueue job: %w", err)
	}

	log.Printf("Enqueued job %s to queue %s", job.ID, queueName)
	return nil
}

// EnqueueDelayed adds a job to be processed at a specific time
func (r *RedisQueue) EnqueueDelayed(ctx context.Context, queueName string, job *models.Job, executeAt time.Time) error {
	job.ExecuteAfter = &executeAt
	job.Status = models.StatusPending

	// Serialize job
	jobData, err := job.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize job: %w", err)
	}

	pipe := r.client.TxPipeline()

	// Store job data
	pipe.HSet(ctx, fmt.Sprintf(jobKey, job.ID), map[string]interface{}{
		"data":       string(jobData),
		"queue":      queueName,
		"created_at": job.CreatedAt.Unix(),
		"status":     string(job.Status),
		"execute_at": executeAt.Unix(),
	})

	// Add to delayed jobs sorted set
	pipe.ZAdd(ctx, delayedKey, &redis.Z{
		Score:  float64(executeAt.Unix()),
		Member: fmt.Sprintf("%s:%s", queueName, job.ID),
	})

	// Track queue name
	pipe.SAdd(ctx, queuesKey, queueName)

	// Update stats
	pipe.HIncrBy(ctx, fmt.Sprintf(statsKey, queueName), "delayed_jobs", 1)
	pipe.HSet(ctx, fmt.Sprintf(statsKey, queueName), "last_updated", time.Now().Unix())

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to enqueue delayed job: %w", err)
	}

	log.Printf("Enqueued delayed job %s to queue %s for execution at %v", job.ID, queueName, executeAt)
	return nil
}

// Dequeue retrieves the next job from the queue
func (r *RedisQueue) Dequeue(ctx context.Context, queueName string) (*models.Job, error) {
	queueKeyName := fmt.Sprintf(queueKey, queueName)
	processingKeyName := fmt.Sprintf(processingKey, queueName)

	// Use ZPOPMIN to get highest priority job (lowest score)
	result, err := r.client.ZPopMin(ctx, queueKeyName, 1).Result()
	if err == redis.Nil || len(result) == 0 {
		return nil, nil // No jobs available
	}
	if err != nil {
		return nil, fmt.Errorf("failed to dequeue job: %w", err)
	}

	jobID := result[0].Member.(string)

	// Move to processing set
	pipe := r.client.TxPipeline()
	pipe.SAdd(ctx, processingKeyName, jobID)
	pipe.HIncrBy(ctx, fmt.Sprintf(statsKey, queueName), "pending_jobs", -1)
	pipe.HIncrBy(ctx, fmt.Sprintf(statsKey, queueName), "processing_jobs", 1)
	pipe.HSet(ctx, fmt.Sprintf(statsKey, queueName), "last_updated", time.Now().Unix())

	_, err = pipe.Exec(ctx)
	if err != nil {
		// Re-add to queue if processing update failed
		r.client.ZAdd(ctx, queueKeyName, &redis.Z{Score: result[0].Score, Member: jobID})
		return nil, fmt.Errorf("failed to update job status: %w", err)
	}

	// Get job data
	job, err := r.GetJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job data: %w", err)
	}

	if job != nil {
		job.MarkProcessing()
		r.UpdateJob(ctx, job)
	}

	return job, nil
}

// DequeueBatch retrieves multiple jobs from the queue
func (r *RedisQueue) DequeueBatch(ctx context.Context, queueName string, batchSize int) ([]*models.Job, error) {
	var jobs []*models.Job

	for i := 0; i < batchSize; i++ {
		job, err := r.Dequeue(ctx, queueName)
		if err != nil {
			return jobs, err
		}
		if job == nil {
			break // No more jobs
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// Ack acknowledges that a job has been processed successfully
func (r *RedisQueue) Ack(ctx context.Context, queueName string, job *models.Job) error {
	processingKeyName := fmt.Sprintf(processingKey, queueName)

	job.MarkSuccess()

	pipe := r.client.TxPipeline()

	// Remove from processing set
	pipe.SRem(ctx, processingKeyName, job.ID)

	// Update job data
	jobData, _ := job.ToJSON()
	pipe.HSet(ctx, fmt.Sprintf(jobKey, job.ID), map[string]interface{}{
		"data":   string(jobData),
		"status": string(job.Status),
	})

	// Update stats
	pipe.HIncrBy(ctx, fmt.Sprintf(statsKey, queueName), "processing_jobs", -1)
	pipe.HIncrBy(ctx, fmt.Sprintf(statsKey, queueName), "completed_jobs", 1)
	pipe.HSet(ctx, fmt.Sprintf(statsKey, queueName), "last_updated", time.Now().Unix())

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to ack job: %w", err)
	}

	log.Printf("Acknowledged job %s from queue %s", job.ID, queueName)
	return nil
}

// Nack indicates that a job failed and should be retried or dead-lettered
func (r *RedisQueue) Nack(ctx context.Context, queueName string, job *models.Job, reason string) error {
	processingKeyName := fmt.Sprintf(processingKey, queueName)
	
	pipe := r.client.TxPipeline()

	// Remove from processing set
	pipe.SRem(ctx, processingKeyName, job.ID)

	if job.IsRetryable() {
		// Mark for retry
		job.MarkRetrying(reason)
		
		// Re-queue with delay
		nextRetry := job.NextRetryAt()
		pipe.ZAdd(ctx, delayedKey, &redis.Z{
			Score:  float64(nextRetry.Unix()),
			Member: fmt.Sprintf("%s:%s", queueName, job.ID),
		})

		// Update stats
		pipe.HIncrBy(ctx, fmt.Sprintf(statsKey, queueName), "processing_jobs", -1)
		pipe.HIncrBy(ctx, fmt.Sprintf(statsKey, queueName), "delayed_jobs", 1)
	} else {
		// Dead letter
		job.MarkDeadLetter()
		deadLetterKeyName := fmt.Sprintf(deadLetterKey, queueName)
		pipe.LPush(ctx, deadLetterKeyName, job.ID)

		// Update stats
		pipe.HIncrBy(ctx, fmt.Sprintf(statsKey, queueName), "processing_jobs", -1)
		pipe.HIncrBy(ctx, fmt.Sprintf(statsKey, queueName), "dead_letter_jobs", 1)
	}

	// Update job data
	jobData, _ := job.ToJSON()
	pipe.HSet(ctx, fmt.Sprintf(jobKey, job.ID), map[string]interface{}{
		"data":   string(jobData),
		"status": string(job.Status),
	})

	pipe.HSet(ctx, fmt.Sprintf(statsKey, queueName), "last_updated", time.Now().Unix())

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to nack job: %w", err)
	}

	log.Printf("Nacked job %s from queue %s (reason: %s)", job.ID, queueName, reason)
	return nil
}

// GetJob retrieves a specific job by ID
func (r *RedisQueue) GetJob(ctx context.Context, jobID string) (*models.Job, error) {
	result, err := r.client.HGetAll(ctx, fmt.Sprintf(jobKey, jobID)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	if len(result) == 0 {
		return nil, nil // Job not found
	}

	jobDataStr, exists := result["data"]
	if !exists {
		return nil, fmt.Errorf("job data not found for job %s", jobID)
	}

	var job models.Job
	if err := json.Unmarshal([]byte(jobDataStr), &job); err != nil {
		return nil, fmt.Errorf("failed to deserialize job: %w", err)
	}

	return &job, nil
}

// UpdateJob updates a job's status and metadata
func (r *RedisQueue) UpdateJob(ctx context.Context, job *models.Job) error {
	jobData, err := job.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize job: %w", err)
	}

	err = r.client.HSet(ctx, fmt.Sprintf(jobKey, job.ID), map[string]interface{}{
		"data":       string(jobData),
		"status":     string(job.Status),
		"updated_at": job.UpdatedAt.Unix(),
	}).Err()

	if err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	return nil
}

// DeleteJob removes a job from the queue
func (r *RedisQueue) DeleteJob(ctx context.Context, jobID string) error {
	return r.client.Del(ctx, fmt.Sprintf(jobKey, jobID)).Err()
}

// GetQueueStats returns statistics about a queue
func (r *RedisQueue) GetQueueStats(ctx context.Context, queueName string) (*QueueStats, error) {
	result, err := r.client.HGetAll(ctx, fmt.Sprintf(statsKey, queueName)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get queue stats: %w", err)
	}

	stats := &QueueStats{
		QueueName:   queueName,
		LastUpdated: time.Now(),
	}

	if val, exists := result["pending_jobs"]; exists {
		stats.PendingJobs, _ = strconv.ParseInt(val, 10, 64)
	}
	if val, exists := result["processing_jobs"]; exists {
		stats.ProcessingJobs, _ = strconv.ParseInt(val, 10, 64)
	}
	if val, exists := result["completed_jobs"]; exists {
		stats.CompletedJobs, _ = strconv.ParseInt(val, 10, 64)
	}
	if val, exists := result["failed_jobs"]; exists {
		stats.FailedJobs, _ = strconv.ParseInt(val, 10, 64)
	}
	if val, exists := result["delayed_jobs"]; exists {
		stats.DelayedJobs, _ = strconv.ParseInt(val, 10, 64)
	}
	if val, exists := result["dead_letter_jobs"]; exists {
		stats.DeadLetterJobs, _ = strconv.ParseInt(val, 10, 64)
	}
	if val, exists := result["last_updated"]; exists {
		if timestamp, err := strconv.ParseInt(val, 10, 64); err == nil {
			stats.LastUpdated = time.Unix(timestamp, 0)
		}
	}

	return stats, nil
}

// ListQueues returns all available queue names
func (r *RedisQueue) ListQueues(ctx context.Context) ([]string, error) {
	return r.client.SMembers(ctx, queuesKey).Result()
}

// GetDelayedJobs returns jobs scheduled for future execution
func (r *RedisQueue) GetDelayedJobs(ctx context.Context, limit int) ([]*models.Job, error) {
	now := time.Now().Unix()
	
	// Get jobs that are ready to be executed
	result, err := r.client.ZRangeByScoreWithScores(ctx, delayedKey, &redis.ZRangeBy{
		Min:   "-inf",
		Max:   strconv.FormatInt(now, 10),
		Count: int64(limit),
	}).Result()
	
	if err != nil {
		return nil, fmt.Errorf("failed to get delayed jobs: %w", err)
	}

	var jobs []*models.Job
	var keysToRemove []string

	for _, item := range result {
		member := item.Member.(string)
		parts := fmt.Sprintf("%s", member)
		// member format: "queueName:jobID"
		jobID := parts[len(parts)-36:] // UUID length is 36
		
		job, err := r.GetJob(ctx, jobID)
		if err != nil {
			log.Printf("Failed to get delayed job %s: %v", jobID, err)
			continue
		}
		
		if job != nil {
			jobs = append(jobs, job)
			keysToRemove = append(keysToRemove, member)
		}
	}

	// Remove processed delayed jobs
	if len(keysToRemove) > 0 {
		r.client.ZRem(ctx, delayedKey, keysToRemove)
	}

	return jobs, nil
}

// GetFailedJobs returns jobs that have failed
func (r *RedisQueue) GetFailedJobs(ctx context.Context, queueName string, limit int) ([]*models.Job, error) {
	deadLetterKeyName := fmt.Sprintf(deadLetterKey, queueName)
	
	jobIDs, err := r.client.LRange(ctx, deadLetterKeyName, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get failed jobs: %w", err)
	}

	var jobs []*models.Job
	for _, jobID := range jobIDs {
		job, err := r.GetJob(ctx, jobID)
		if err != nil {
			log.Printf("Failed to get failed job %s: %v", jobID, err)
			continue
		}
		if job != nil {
			jobs = append(jobs, job)
		}
	}

	return jobs, nil
}

// PurgeQueue removes all jobs from a queue
func (r *RedisQueue) PurgeQueue(ctx context.Context, queueName string) error {
	pipe := r.client.TxPipeline()
	
	pipe.Del(ctx, fmt.Sprintf(queueKey, queueName))
	pipe.Del(ctx, fmt.Sprintf(processingKey, queueName))
	pipe.Del(ctx, fmt.Sprintf(failedKey, queueName))
	pipe.Del(ctx, fmt.Sprintf(deadLetterKey, queueName))
	pipe.Del(ctx, fmt.Sprintf(statsKey, queueName))
	
	_, err := pipe.Exec(ctx)
	return err
}

// Close closes the queue connection
func (r *RedisQueue) Close() error {
	return r.client.Close()
}

// HealthCheck verifies the queue backend is healthy
func (r *RedisQueue) HealthCheck(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
} 