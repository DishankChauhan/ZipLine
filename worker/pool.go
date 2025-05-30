package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/DishankChauhan/ZipLine/config"
	"github.com/DishankChauhan/ZipLine/models"
	"github.com/DishankChauhan/ZipLine/queue"
)

// Pool manages a pool of workers for processing jobs
type Pool struct {
	queue     queue.Queue
	registry  *queue.Registry
	config    *config.WorkerConfig
	
	workers   []*Worker
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	
	// Channels for communication
	jobChan     chan *JobRequest
	resultChan  chan *JobResult
	
	// Pool state
	isRunning   bool
	mu          sync.RWMutex
	
	// Metrics
	processedJobs int64
	failedJobs    int64
	activeJobs    int64
}

// JobRequest represents a job to be processed
type JobRequest struct {
	Job       *models.Job
	QueueName string
}

// JobResult represents the result of job processing
type JobResult struct {
	Job       *models.Job
	QueueName string
	Success   bool
	Error     error
	Duration  time.Duration
}

// Worker represents a single worker in the pool
type Worker struct {
	id        int
	pool      *Pool
	jobChan   <-chan *JobRequest
	resultChan chan<- *JobResult
	ctx       context.Context
}

// NewPool creates a new worker pool
func NewPool(q queue.Queue, registry *queue.Registry, cfg *config.WorkerConfig) *Pool {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Pool{
		queue:     q,
		registry:  registry,
		config:    cfg,
		ctx:       ctx,
		cancel:    cancel,
		jobChan:   make(chan *JobRequest, cfg.MaxConcurrency),
		resultChan: make(chan *JobResult, cfg.MaxConcurrency),
	}
}

// Start starts the worker pool
func (p *Pool) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.isRunning {
		return fmt.Errorf("worker pool is already running")
	}
	
	log.Printf("Starting worker pool with %d workers", p.config.Count)
	
	// Create workers
	p.workers = make([]*Worker, p.config.Count)
	for i := 0; i < p.config.Count; i++ {
		p.workers[i] = &Worker{
			id:         i,
			pool:       p,
			jobChan:    p.jobChan,
			resultChan: p.resultChan,
			ctx:        p.ctx,
		}
	}
	
	// Start workers
	for _, worker := range p.workers {
		p.wg.Add(1)
		go worker.start()
	}
	
	// Start result processor
	p.wg.Add(1)
	go p.processResults()
	
	// Start job fetcher
	p.wg.Add(1)
	go p.fetchJobs()
	
	p.isRunning = true
	log.Printf("Worker pool started successfully")
	
	return nil
}

// Stop gracefully stops the worker pool
func (p *Pool) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if !p.isRunning {
		return fmt.Errorf("worker pool is not running")
	}
	
	log.Printf("Stopping worker pool...")
	
	// Cancel context to signal workers to stop
	p.cancel()
	
	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		log.Printf("Worker pool stopped gracefully")
	case <-time.After(p.config.GracefulShutdownTimeout):
		log.Printf("Worker pool stop timed out, some workers may still be running")
	}
	
	// Close channels
	close(p.jobChan)
	close(p.resultChan)
	
	p.isRunning = false
	return nil
}

// GetStats returns pool statistics
func (p *Pool) GetStats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	return PoolStats{
		WorkerCount:   len(p.workers),
		ProcessedJobs: p.processedJobs,
		FailedJobs:    p.failedJobs,
		ActiveJobs:    p.activeJobs,
		IsRunning:     p.isRunning,
		QueueDepth:    int64(len(p.jobChan)),
	}
}

// PoolStats holds worker pool statistics
type PoolStats struct {
	WorkerCount   int   `json:"worker_count"`
	ProcessedJobs int64 `json:"processed_jobs"`
	FailedJobs    int64 `json:"failed_jobs"`
	ActiveJobs    int64 `json:"active_jobs"`
	IsRunning     bool  `json:"is_running"`
	QueueDepth    int64 `json:"queue_depth"`
}

// fetchJobs continuously fetches jobs from queues and sends them to workers
func (p *Pool) fetchJobs() {
	defer p.wg.Done()
	
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()
	
	// List of queues to process (can be made configurable)
	queues := []string{"default", "high", "low"} // Default queue names
	
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.fetchJobsFromQueues(queues)
		}
	}
}

// fetchJobsFromQueues fetches jobs from specified queues
func (p *Pool) fetchJobsFromQueues(queueNames []string) {
	for _, queueName := range queueNames {
		// Try to get updated queue list from backend
		if availableQueues, err := p.queue.ListQueues(p.ctx); err == nil && len(availableQueues) > 0 {
			queueNames = availableQueues
		}
		
		select {
		case <-p.ctx.Done():
			return
		default:
		}
		
		// Fetch batch of jobs
		jobs, err := p.queue.DequeueBatch(p.ctx, queueName, p.config.BatchSize)
		if err != nil {
			log.Printf("Error fetching jobs from queue %s: %v", queueName, err)
			continue
		}
		
		// Send jobs to workers
		for _, job := range jobs {
			select {
			case p.jobChan <- &JobRequest{Job: job, QueueName: queueName}:
				p.mu.Lock()
				p.activeJobs++
				p.mu.Unlock()
			case <-p.ctx.Done():
				// Return job to queue if we're shutting down
				if err := p.queue.Nack(p.ctx, queueName, job, "Worker pool shutting down"); err != nil {
					log.Printf("Error returning job to queue during shutdown: %v", err)
				}
				return
			default:
				// Job channel is full, return job to queue
				if err := p.queue.Nack(p.ctx, queueName, job, "Worker pool busy"); err != nil {
					log.Printf("Error returning job to queue: %v", err)
				}
			}
		}
	}
}

// processResults processes job results from workers
func (p *Pool) processResults() {
	defer p.wg.Done()
	
	for {
		select {
		case <-p.ctx.Done():
			return
		case result := <-p.resultChan:
			p.handleJobResult(result)
		}
	}
}

// handleJobResult handles the result of job processing
func (p *Pool) handleJobResult(result *JobResult) {
	p.mu.Lock()
	p.activeJobs--
	p.mu.Unlock()
	
	if result.Success {
		// Acknowledge successful job
		if err := p.queue.Ack(p.ctx, result.QueueName, result.Job); err != nil {
			log.Printf("Error acknowledging job %s: %v", result.Job.ID, err)
		} else {
			p.mu.Lock()
			p.processedJobs++
			p.mu.Unlock()
			log.Printf("Job %s completed successfully in %v", result.Job.ID, result.Duration)
		}
	} else {
		// Handle failed job
		errorMsg := "Unknown error"
		if result.Error != nil {
			errorMsg = result.Error.Error()
		}
		
		if err := p.queue.Nack(p.ctx, result.QueueName, result.Job, errorMsg); err != nil {
			log.Printf("Error nacking job %s: %v", result.Job.ID, err)
		} else {
			p.mu.Lock()
			p.failedJobs++
			p.mu.Unlock()
			log.Printf("Job %s failed: %s", result.Job.ID, errorMsg)
		}
	}
}

// start starts a single worker
func (w *Worker) start() {
	defer w.pool.wg.Done()
	
	log.Printf("Worker %d started", w.id)
	defer log.Printf("Worker %d stopped", w.id)
	
	for {
		select {
		case <-w.ctx.Done():
			return
		case jobReq := <-w.jobChan:
			w.processJob(jobReq)
		}
	}
}

// processJob processes a single job
func (w *Worker) processJob(jobReq *JobRequest) {
	start := time.Now()
	
	// Create job-specific context with timeout
	jobCtx, cancel := context.WithTimeout(w.ctx, w.pool.config.JobTimeout)
	defer cancel()
	
	result := &JobResult{
		Job:       jobReq.Job,
		QueueName: jobReq.QueueName,
		Success:   false,
	}
	
	// Get processor for job type
	processor, exists := w.pool.registry.GetProcessor(jobReq.Job.Type)
	if !exists {
		result.Error = fmt.Errorf("no processor found for job type: %s", jobReq.Job.Type)
		result.Duration = time.Since(start)
		
		select {
		case w.resultChan <- result:
		case <-w.ctx.Done():
		}
		return
	}
	
	// Process the job
	log.Printf("Worker %d processing job %s (type: %s)", w.id, jobReq.Job.ID, jobReq.Job.Type)
	
	err := processor.ProcessJob(jobCtx, jobReq.Job)
	
	result.Duration = time.Since(start)
	result.Success = err == nil
	result.Error = err
	
	// Send result
	select {
	case w.resultChan <- result:
	case <-w.ctx.Done():
	}
} 