package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the job queue system
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Worker    WorkerConfig    `mapstructure:"worker"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	Metrics   MetricsConfig   `mapstructure:"metrics"`
	Logging   LoggingConfig   `mapstructure:"logging"`
}

// ServerConfig holds API server configuration
type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	GRPCPort     int           `mapstructure:"grpc_port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Host        string        `mapstructure:"host"`
	Port        int           `mapstructure:"port"`
	Password    string        `mapstructure:"password"`
	Database    int           `mapstructure:"database"`
	MaxRetries  int           `mapstructure:"max_retries"`
	PoolSize    int           `mapstructure:"pool_size"`
	DialTimeout time.Duration `mapstructure:"dial_timeout"`
}

// WorkerConfig holds worker pool configuration
type WorkerConfig struct {
	Count               int           `mapstructure:"count"`
	MaxConcurrency      int           `mapstructure:"max_concurrency"`
	PollInterval        time.Duration `mapstructure:"poll_interval"`
	JobTimeout          time.Duration `mapstructure:"job_timeout"`
	GracefulShutdownTimeout time.Duration `mapstructure:"graceful_shutdown_timeout"`
	BatchSize           int           `mapstructure:"batch_size"`
}

// SchedulerConfig holds scheduler configuration
type SchedulerConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	PollInterval time.Duration `mapstructure:"poll_interval"`
	BatchSize    int           `mapstructure:"batch_size"`
}

// MetricsConfig holds metrics and monitoring configuration
type MetricsConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Port       int    `mapstructure:"port"`
	Path       string `mapstructure:"path"`
	Namespace  string `mapstructure:"namespace"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"` // json, text
}

// Load loads configuration from environment variables and config files
func Load() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/job-queue-system")

	// Set default values
	setDefaults()

	// Enable automatic environment variable binding
	viper.AutomaticEnv()
	viper.SetEnvPrefix("JQS") // Job Queue System

	// Bind environment variables explicitly for nested structures
	viper.BindEnv("redis.host", "JQS_REDIS_HOST")
	viper.BindEnv("redis.port", "JQS_REDIS_PORT")
	viper.BindEnv("redis.password", "JQS_REDIS_PASSWORD")
	viper.BindEnv("redis.database", "JQS_REDIS_DATABASE")
	viper.BindEnv("server.host", "JQS_SERVER_HOST")
	viper.BindEnv("server.port", "JQS_SERVER_PORT")
	viper.BindEnv("server.grpc_port", "JQS_SERVER_GRPC_PORT")
	viper.BindEnv("worker.count", "JQS_WORKER_COUNT")
	viper.BindEnv("worker.max_concurrency", "JQS_WORKER_MAX_CONCURRENCY")
	viper.BindEnv("worker.poll_interval", "JQS_WORKER_POLL_INTERVAL")
	viper.BindEnv("worker.job_timeout", "JQS_WORKER_JOB_TIMEOUT")
	viper.BindEnv("scheduler.enabled", "JQS_SCHEDULER_ENABLED")
	viper.BindEnv("scheduler.poll_interval", "JQS_SCHEDULER_POLL_INTERVAL")
	viper.BindEnv("scheduler.batch_size", "JQS_SCHEDULER_BATCH_SIZE")
	viper.BindEnv("metrics.enabled", "JQS_METRICS_ENABLED")
	viper.BindEnv("metrics.port", "JQS_METRICS_PORT")
	viper.BindEnv("logging.level", "JQS_LOGGING_LEVEL")
	viper.BindEnv("logging.format", "JQS_LOGGING_FORMAT")

	// Read config file (optional)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found, continue with defaults and env vars
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

// setDefaults sets default configuration values
func setDefaults() {
	// Server defaults
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.grpc_port", 9090)
	viper.SetDefault("server.read_timeout", 30*time.Second)
	viper.SetDefault("server.write_timeout", 30*time.Second)
	viper.SetDefault("server.idle_timeout", 60*time.Second)

	// Redis defaults
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.database", 0)
	viper.SetDefault("redis.max_retries", 3)
	viper.SetDefault("redis.pool_size", 10)
	viper.SetDefault("redis.dial_timeout", 5*time.Second)

	// Worker defaults
	viper.SetDefault("worker.count", 4)
	viper.SetDefault("worker.max_concurrency", 100)
	viper.SetDefault("worker.poll_interval", 1*time.Second)
	viper.SetDefault("worker.job_timeout", 5*time.Minute)
	viper.SetDefault("worker.graceful_shutdown_timeout", 30*time.Second)
	viper.SetDefault("worker.batch_size", 10)

	// Scheduler defaults
	viper.SetDefault("scheduler.enabled", true)
	viper.SetDefault("scheduler.poll_interval", 30*time.Second)
	viper.SetDefault("scheduler.batch_size", 100)

	// Metrics defaults
	viper.SetDefault("metrics.enabled", true)
	viper.SetDefault("metrics.port", 9091)
	viper.SetDefault("metrics.path", "/metrics")
	viper.SetDefault("metrics.namespace", "job_queue_system")

	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
}

// RedisAddr returns the formatted Redis address
func (c *RedisConfig) RedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// ServerAddr returns the formatted server address
func (c *ServerConfig) ServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// GRPCAddr returns the formatted gRPC server address
func (c *ServerConfig) GRPCAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.GRPCPort)
}

// MetricsAddr returns the formatted metrics server address
func (c *MetricsConfig) MetricsAddr() string {
	return fmt.Sprintf(":%d", c.Port)
} 