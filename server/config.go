package main

import (
	"os"
    "strconv"
    "strings"
    "time"
)

// Configuration struct to hold all configurable parameters
type Config struct {

    // Server configuration
    Port                string
    MaxFileSize         int64         // MB
    GradingTimeout      time.Duration // minutes
    CleanupInterval     time.Duration // hours
    CompletedJobTTL     time.Duration // hours
    FailedJobTTL        time.Duration // hours
    OldFileTTL          time.Duration // hours
    QueueBufferSize     int
    GraderRegistryPath  string        // Path to grader registry file

    // Security configuration
    RequireAPIKey       bool          // Enable API key authentication
    RequireUsername     bool          // Require username header (always true for rate limiting)
    ValidAPIKeys        []string      // Valid API keys
    AllowedIPs          []string      // IP whitelist for maximum security

    // Rate limiting configuration
    RateLimitEnabled    bool          // Enable rate limiting
    RateLimitRequests   int           // Requests per window
    RateLimitWindow     time.Duration // Time window for rate limiting
    
    // Resource limits
    MaxConcurrentJobs   int           // Maximum concurrent grading jobs
    MaxQueueSize        int           // Maximum queued jobs
}

// Load server configuration settings
func loadConfig() *Config {

    // Load configuration from environment variables with defaults
    config := &Config{

        // Server configuration
        Port:                getEnv("PORT", "8080"),
        MaxFileSize:         getEnvInt64("MAX_FILE_SIZE_MB", 50),
        GradingTimeout:      time.Duration(getEnvInt("GRADING_TIMEOUT_MIN", 5)) * time.Minute,
        CleanupInterval:     time.Duration(getEnvInt("CLEANUP_INTERVAL_HOURS", 1)) * time.Hour,
        CompletedJobTTL:     time.Duration(getEnvInt("COMPLETED_JOB_TTL_HOURS", 24)) * time.Hour,
        FailedJobTTL:        time.Duration(getEnvInt("FAILED_JOB_TTL_HOURS", 24)) * time.Hour,
        OldFileTTL:          time.Duration(getEnvInt("OLD_FILE_TTL_HOURS", 48)) * time.Hour,
        QueueBufferSize:     getEnvInt("QUEUE_BUFFER_SIZE", 100),
        GraderRegistryPath: getEnv("GRADER_REGISTRY_PATH", "/usr/local/bin/graders/registry.yaml"),
        
        // Security configuration
        RequireAPIKey       bool          // Enable API key authentication
        RequireUsername:    true,         // Always require username for proper rate limiting
        ValidAPIKeys        []string      // Valid API keys
        AllowedIPs          []string      // IP whitelist for maximum security
        
        // Rate limiting configuration
        RateLimitEnabled:    getEnvBool("RATE_LIMIT_ENABLED", true),
        RateLimitRequests:   getEnvInt("RATE_LIMIT_REQUESTS", 10),
        RateLimitWindow:     time.Duration(getEnvInt("RATE_LIMIT_WINDOW_MIN", 5)) * time.Minute,
        
        // Resource limits
        MaxConcurrentJobs:   getEnvInt("MAX_CONCURRENT_JOBS", 3),
        MaxQueueSize:        getEnvInt("MAX_QUEUE_SIZE", 50),
    }
    
    // Convert MB to bytes for file size
    config.MaxFileSize = config.MaxFileSize * 1024 * 1024
    
    return config
}

// Helper functions to get environment variables with defaults
func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

// Helper functions to get environment variables as integers
func getEnvInt(key string, defaultValue int) int {
    if value := os.Getenv(key); value != "" {
        if intValue, err := strconv.Atoi(value); err == nil {
            return intValue
        }
    }
    return defaultValue
}

// Helper functions to get environment variables as int64
func getEnvInt64(key string, defaultValue int64) int64 {
    if value := os.Getenv(key); value != "" {
        if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
            return intValue
        }
    }
    return defaultValue
}

// Helper function to get environment variables as boolean
func getEnvBool(key string, defaultValue bool) bool {
    if value := os.Getenv(key); value != "" {
        if boolValue, err := strconv.ParseBool(value); err == nil {
            return boolValue
        }
    }
    return defaultValue
}

// parseAPIKeys parses comma-separated API keys
func parseAPIKeys(keys string) []string {
    if keys == "" {
        return []string{}
    }
    
    var apiKeys []string
    for _, key := range strings.Split(keys, ",") {
        key = strings.TrimSpace(key)
        if key != "" {
            apiKeys = append(apiKeys, key)
        }
    }
    
    return apiKeys
}

// Parse comma-separated IP addresses and CIDR blocks
func parseAllowedIPs(ips string) []string {
    if ips == "" {
        return []string{}
    }
    
    var allowedIPs []string
    for _, ip := range strings.Split(ips, ",") {
        ip = strings.TrimSpace(ip)
        if ip != "" {
            allowedIPs = append(allowedIPs, ip)
        }
    }
    
    return allowedIPs
}
