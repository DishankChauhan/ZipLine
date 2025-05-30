package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/DishankChauhan/ZipLine/config"
	"github.com/DishankChauhan/ZipLine/queue"
)

// Scheduler handles delayed job execution
type Scheduler struct {
	queue     queue.Queue
	config    *config.SchedulerConfig
	
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	
	isRunning bool
	mu        sync.RWMutex
	
	// Metrics
	processedDelayedJobs int64
	errorCount          int64
}

// NewScheduler creates a new scheduler instance
func NewScheduler(q queue.Queue, cfg *config.SchedulerConfig) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Scheduler{
		queue:  q,
		config: cfg,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start starts the scheduler
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if !s.config.Enabled {
		log.Printf("Scheduler is disabled in configuration")
		return nil
	}
	
	if s.isRunning {
		return nil
	}
	
	log.Printf("Starting scheduler with poll interval: %v", s.config.PollInterval)
	
	s.wg.Add(1)
	go s.run()
	
	s.isRunning = true
	log.Printf("Scheduler started successfully")
	
	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if !s.isRunning {
		return nil
	}
	
	log.Printf("Stopping scheduler...")
	
	s.cancel()
	s.wg.Wait()
	
	s.isRunning = false
	log.Printf("Scheduler stopped")
	
	return nil
}

// GetStats returns scheduler statistics
func (s *Scheduler) GetStats() SchedulerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	return SchedulerStats{
		IsRunning:            s.isRunning,
		ProcessedDelayedJobs: s.processedDelayedJobs,
		ErrorCount:          s.errorCount,
		PollInterval:        s.config.PollInterval,
	}
}

// SchedulerStats holds scheduler statistics
type SchedulerStats struct {
	IsRunning            bool          `json:"is_running"`
	ProcessedDelayedJobs int64         `json:"processed_delayed_jobs"`
	ErrorCount          int64         `json:"error_count"`
	PollInterval        time.Duration `json:"poll_interval"`
}

// run is the main scheduler loop
func (s *Scheduler) run() {
	defer s.wg.Done()
	
	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.processDelayedJobs()
		}
	}
}

// processDelayedJobs processes jobs that are ready for execution
func (s *Scheduler) processDelayedJobs() {
	// Get delayed jobs that are ready to be executed
	jobs, err := s.queue.GetDelayedJobs(s.ctx, s.config.BatchSize)
	if err != nil {
		log.Printf("Error getting delayed jobs: %v", err)
		s.mu.Lock()
		s.errorCount++
		s.mu.Unlock()
		return
	}
	
	if len(jobs) == 0 {
		return
	}
	
	log.Printf("Processing %d delayed jobs", len(jobs))
	
	successCount := 0
	for _, job := range jobs {
		// Extract queue name from job metadata or use default
		queueName := "default"
		if job.Tags != nil {
			if queue, exists := job.Tags["queue"]; exists {
				queueName = queue
			}
		}
		
		// Move job to active queue
		if err := s.queue.Enqueue(s.ctx, queueName, job); err != nil {
			log.Printf("Error enqueuing delayed job %s: %v", job.ID, err)
			s.mu.Lock()
			s.errorCount++
			s.mu.Unlock()
		} else {
			successCount++
		}
	}
	
	if successCount > 0 {
		s.mu.Lock()
		s.processedDelayedJobs += int64(successCount)
		s.mu.Unlock()
		log.Printf("Successfully moved %d delayed jobs to active queues", successCount)
	}
}

// IsRunning returns whether the scheduler is running
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
} 