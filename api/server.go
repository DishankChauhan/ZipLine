package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/DishankChauhan/ZipLine/config"
	"github.com/DishankChauhan/ZipLine/models"
	"github.com/DishankChauhan/ZipLine/queue"
	"github.com/DishankChauhan/ZipLine/scheduler"
	"github.com/DishankChauhan/ZipLine/worker"
)

// Server represents the API server
type Server struct {
	config    *config.ServerConfig
	queue     queue.Queue
	registry  *queue.Registry
	pool      *worker.Pool
	scheduler *scheduler.Scheduler
	router    *gin.Engine
}

// JobSubmissionRequest represents a job submission request
type JobSubmissionRequest struct {
	Name        string                 `json:"name" binding:"required"`
	Type        string                 `json:"type" binding:"required"`
	Payload     map[string]interface{} `json:"payload"`
	Priority    int                    `json:"priority"`
	Queue       string                 `json:"queue"`
	RetryPolicy *RetryPolicyRequest    `json:"retry"`
	ScheduleAt  *time.Time             `json:"schedule_at"`
	Tags        map[string]string      `json:"tags"`
}

// RetryPolicyRequest represents retry policy in API requests
type RetryPolicyRequest struct {
	MaxRetries   int                `json:"max_retries"`
	BackoffType  models.BackoffType `json:"backoff"`
	InitialDelay time.Duration      `json:"initial_delay"`
	MaxDelay     time.Duration      `json:"max_delay"`
}

// JobResponse represents a job in API responses
type JobResponse struct {
	*models.Job
	QueueName string `json:"queue_name,omitempty"`
}

// NewServer creates a new API server
func NewServer(cfg *config.ServerConfig, q queue.Queue, registry *queue.Registry, pool *worker.Pool, scheduler *scheduler.Scheduler) *Server {
	server := &Server{
		config:    cfg,
		queue:     q,
		registry:  registry,
		pool:      pool,
		scheduler: scheduler,
	}
	
	server.setupRoutes()
	return server
}

// setupRoutes configures the API routes
func (s *Server) setupRoutes() {
	if gin.Mode() == gin.ReleaseMode {
		gin.SetMode(gin.ReleaseMode)
	}
	
	s.router = gin.New()
	s.router.Use(gin.Logger())
	s.router.Use(gin.Recovery())
	s.router.Use(corsMiddleware())
	
	// Health check
	s.router.GET("/health", s.healthCheck)
	
	// API v1 routes
	v1 := s.router.Group("/api/v1")
	{
		// Job management
		jobs := v1.Group("/jobs")
		{
			jobs.POST("", s.submitJob)
			jobs.GET("/:id", s.getJob)
			jobs.DELETE("/:id", s.deleteJob)
			jobs.POST("/:id/cancel", s.cancelJob)
			jobs.POST("/:id/retry", s.retryJob)
		}
		
		// Queue management
		queues := v1.Group("/queues")
		{
			queues.GET("", s.listQueues)
			queues.GET("/:name/stats", s.getQueueStats)
			queues.GET("/:name/jobs", s.getQueueJobs)
			queues.DELETE("/:name", s.purgeQueue)
			queues.GET("/:name/failed", s.getFailedJobs)
		}
		
		// System stats
		v1.GET("/stats", s.getSystemStats)
		v1.GET("/workers/stats", s.getWorkerStats)
		v1.GET("/scheduler/stats", s.getSchedulerStats)
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := s.config.ServerAddr()
	log.Printf("Starting API server on %s", addr)
	
	server := &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}
	
	return server.ListenAndServe()
}

// healthCheck handles health check requests
func (s *Server) healthCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	health := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now(),
		"version":   "1.0.0",
	}
	
	// Check queue health
	if err := s.queue.HealthCheck(ctx); err != nil {
		health["status"] = "error"
		health["queue_error"] = err.Error()
		c.JSON(http.StatusServiceUnavailable, health)
		return
	}
	
	// Check worker pool
	poolStats := s.pool.GetStats()
	health["workers"] = map[string]interface{}{
		"running": poolStats.IsRunning,
		"count":   poolStats.WorkerCount,
	}
	
	c.JSON(http.StatusOK, health)
}

// submitJob handles job submission
func (s *Server) submitJob(c *gin.Context) {
	var req JobSubmissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Create job
	job := models.NewJob(req.Name, req.Type, req.Payload)
	job.Priority = req.Priority
	
	// Set queue name in tags
	queueName := req.Queue
	if queueName == "" {
		queueName = "default"
	}
	if job.Tags == nil {
		job.Tags = make(map[string]string)
	}
	job.Tags["queue"] = queueName
	
	// Apply custom retry policy if provided
	if req.RetryPolicy != nil {
		job.RetryPolicy = models.RetryPolicy{
			MaxRetries:   req.RetryPolicy.MaxRetries,
			BackoffType:  req.RetryPolicy.BackoffType,
			InitialDelay: req.RetryPolicy.InitialDelay,
			MaxDelay:     req.RetryPolicy.MaxDelay,
		}
	}
	
	// Apply custom tags
	if req.Tags != nil {
		for k, v := range req.Tags {
			job.Tags[k] = v
		}
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Submit job
	var err error
	if req.ScheduleAt != nil {
		err = s.queue.EnqueueDelayed(ctx, queueName, job, *req.ScheduleAt)
	} else {
		err = s.queue.Enqueue(ctx, queueName, job)
	}
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusCreated, &JobResponse{
		Job:       job,
		QueueName: queueName,
	})
}

// getJob handles job retrieval
func (s *Server) getJob(c *gin.Context) {
	jobID := c.Param("id")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	job, err := s.queue.GetJob(ctx, jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}
	
	c.JSON(http.StatusOK, &JobResponse{Job: job})
}

// deleteJob handles job deletion
func (s *Server) deleteJob(c *gin.Context) {
	jobID := c.Param("id")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := s.queue.DeleteJob(ctx, jobID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Job deleted successfully"})
}

// cancelJob handles job cancellation
func (s *Server) cancelJob(c *gin.Context) {
	jobID := c.Param("id")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	job, err := s.queue.GetJob(ctx, jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}
	
	if job.Status == models.StatusProcessing {
		c.JSON(http.StatusConflict, gin.H{"error": "Cannot cancel job that is currently processing"})
		return
	}
	
	job.MarkCancelled()
	if err := s.queue.UpdateJob(ctx, job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Job cancelled successfully"})
}

// retryJob handles job retry
func (s *Server) retryJob(c *gin.Context) {
	jobID := c.Param("id")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	job, err := s.queue.GetJob(ctx, jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}
	
	// Reset job for retry
	job.Status = models.StatusPending
	job.AttemptCount = 0
	job.LastError = ""
	job.UpdatedAt = time.Now()
	
	queueName := "default"
	if job.Tags != nil {
		if queue, exists := job.Tags["queue"]; exists {
			queueName = queue
		}
	}
	
	if err := s.queue.Enqueue(ctx, queueName, job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": "Job queued for retry"})
}

// listQueues handles queue listing
func (s *Server) listQueues(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	queues, err := s.queue.ListQueues(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"queues": queues})
}

// getQueueStats handles queue statistics
func (s *Server) getQueueStats(c *gin.Context) {
	queueName := c.Param("name")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	stats, err := s.queue.GetQueueStats(ctx, queueName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, stats)
}

// getQueueJobs handles getting jobs from a queue
func (s *Server) getQueueJobs(c *gin.Context) {
	queueName := c.Param("name")
	limitStr := c.DefaultQuery("limit", "10")
	limit, _ := strconv.Atoi(limitStr)
	
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// For now, we'll return failed jobs as an example
	jobs, err := s.queue.GetFailedJobs(ctx, queueName, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"queue": queueName,
		"jobs":  jobs,
		"count": len(jobs),
	})
}

// purgeQueue handles queue purging
func (s *Server) purgeQueue(c *gin.Context) {
	queueName := c.Param("name")
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := s.queue.PurgeQueue(ctx, queueName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Queue %s purged successfully", queueName)})
}

// getFailedJobs handles failed job retrieval
func (s *Server) getFailedJobs(c *gin.Context) {
	queueName := c.Param("name")
	limitStr := c.DefaultQuery("limit", "10")
	limit, _ := strconv.Atoi(limitStr)
	
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	jobs, err := s.queue.GetFailedJobs(ctx, queueName, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"queue":       queueName,
		"failed_jobs": jobs,
		"count":       len(jobs),
	})
}

// getSystemStats handles system statistics
func (s *Server) getSystemStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	queues, _ := s.queue.ListQueues(ctx)
	
	totalStats := map[string]interface{}{
		"total_queues": len(queues),
		"queues":      make(map[string]interface{}),
	}
	
	for _, queueName := range queues {
		if stats, err := s.queue.GetQueueStats(ctx, queueName); err == nil {
			totalStats["queues"].(map[string]interface{})[queueName] = stats
		}
	}
	
	c.JSON(http.StatusOK, totalStats)
}

// getWorkerStats handles worker statistics
func (s *Server) getWorkerStats(c *gin.Context) {
	stats := s.pool.GetStats()
	c.JSON(http.StatusOK, stats)
}

// getSchedulerStats handles scheduler statistics
func (s *Server) getSchedulerStats(c *gin.Context) {
	stats := s.scheduler.GetStats()
	c.JSON(http.StatusOK, stats)
}

// corsMiddleware handles CORS
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Header("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
} 