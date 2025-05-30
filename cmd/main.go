package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/DishankChauhan/ZipLine/api"
	"github.com/DishankChauhan/ZipLine/config"
	"github.com/DishankChauhan/ZipLine/models"
	"github.com/DishankChauhan/ZipLine/queue"
	"github.com/DishankChauhan/ZipLine/scheduler"
	"github.com/DishankChauhan/ZipLine/worker"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Starting ZipLine Job Queue System...")

	// Initialize Redis queue
	redisQueue, err := queue.NewRedisQueue(&cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to initialize Redis queue: %v", err)
	}
	defer redisQueue.Close()

	// Create job processor registry
	registry := queue.NewRegistry()
	
	// Register sample job processors
	registerDefaultProcessors(registry)

	// Initialize components
	workerPool := worker.NewPool(redisQueue, registry, &cfg.Worker)
	jobScheduler := scheduler.NewScheduler(redisQueue, &cfg.Scheduler)
	apiServer := api.NewServer(&cfg.Server, redisQueue, registry, workerPool, jobScheduler)

	// Start components
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	// Start scheduler
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := jobScheduler.Start(); err != nil {
			log.Printf("Failed to start scheduler: %v", err)
		}
		<-ctx.Done()
		jobScheduler.Stop()
	}()

	// Start worker pool
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := workerPool.Start(); err != nil {
			log.Printf("Failed to start worker pool: %v", err)
		}
		<-ctx.Done()
		workerPool.Stop()
	}()

	// Start API server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := apiServer.Start(); err != nil {
			log.Printf("API server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("ZipLine Job Queue System started successfully")
	log.Printf("API Server: http://%s", cfg.Server.ServerAddr())
	log.Printf("Health Check: http://%s/health", cfg.Server.ServerAddr())

	// Wait for shutdown signal
	<-sigChan
	log.Printf("Received shutdown signal, gracefully shutting down...")

	// Cancel context to signal shutdown
	cancel()

	// Wait for all components to stop
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Wait with timeout
	select {
	case <-done:
		log.Printf("All components stopped gracefully")
	case <-time.After(30 * time.Second):
		log.Printf("Shutdown timeout exceeded, forcing exit")
	}

	log.Printf("ZipLine Job Queue System shutdown complete")
}

// registerDefaultProcessors registers sample job processors for demonstration
func registerDefaultProcessors(registry *queue.Registry) {
	// Email processor
	registry.Register("send_email", &EmailProcessor{})
	
	// Webhook processor
	registry.Register("webhook", &WebhookProcessor{})
	
	// Data processing processor
	registry.Register("process_data", &DataProcessor{})
	
	log.Printf("Registered %d default job processors", len(registry.ListProcessors()))
}

// EmailProcessor handles email sending jobs
type EmailProcessor struct{}

func (p *EmailProcessor) ProcessJob(ctx context.Context, job *models.Job) error {
	log.Printf("Processing email job %s", job.ID)
	
	// Extract email data from payload
	email, ok := job.Payload["email"].(string)
	if !ok {
		return fmt.Errorf("email field is required")
	}
	
	subject, _ := job.Payload["subject"].(string)
	_, _ = job.Payload["body"].(string)
	
	// Simulate email sending
	time.Sleep(time.Duration(100+job.AttemptCount*50) * time.Millisecond)
	
	// Simulate occasional failures for testing retry logic
	if job.AttemptCount <= 1 && job.ID[len(job.ID)-1] < '5' {
		return fmt.Errorf("simulated email service timeout")
	}
	
	log.Printf("Email sent successfully to %s (subject: %s)", email, subject)
	return nil
}

func (p *EmailProcessor) CanProcess(jobType string) bool {
	return jobType == "send_email"
}

func (p *EmailProcessor) GetName() string {
	return "EmailProcessor"
}

// WebhookProcessor handles webhook calls
type WebhookProcessor struct{}

func (p *WebhookProcessor) ProcessJob(ctx context.Context, job *models.Job) error {
	log.Printf("Processing webhook job %s", job.ID)
	
	url, ok := job.Payload["url"].(string)
	if !ok {
		return fmt.Errorf("url field is required")
	}
	
	// Simulate webhook call
	time.Sleep(200 * time.Millisecond)
	
	log.Printf("Webhook called successfully: %s", url)
	return nil
}

func (p *WebhookProcessor) CanProcess(jobType string) bool {
	return jobType == "webhook"
}

func (p *WebhookProcessor) GetName() string {
	return "WebhookProcessor"
}

// DataProcessor handles data processing jobs
type DataProcessor struct{}

func (p *DataProcessor) ProcessJob(ctx context.Context, job *models.Job) error {
	log.Printf("Processing data job %s", job.ID)
	
	dataSize, ok := job.Payload["data_size"].(float64)
	if !ok {
		dataSize = 1000 // default size
	}
	
	// Simulate data processing time based on size
	processingTime := time.Duration(dataSize/10) * time.Millisecond
	if processingTime > 5*time.Second {
		processingTime = 5 * time.Second
	}
	
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(processingTime):
		// Processing complete
	}
	
	log.Printf("Data processing completed for job %s (size: %.0f)", job.ID, dataSize)
	return nil
}

func (p *DataProcessor) CanProcess(jobType string) bool {
	return jobType == "process_data"
}

func (p *DataProcessor) GetName() string {
	return "DataProcessor"
} 