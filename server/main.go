// Implements the ByteGrader autograder API server. Provides HTTP endpoints for submitting code for
// grading and checking job status.
package main

import (
    "context"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/docker/docker/client"
    "golang.org/x/time/rate"
    "gopkg.in/yaml.v3"
)

//------------------------------------------------------------------------------
// Structs and Types

// Configuration for each assignment
type AssignmentConfig struct {
    Image           string `yaml:"image"`
    Description     string `yaml:"description"`
    TimeoutMinutes  int    `yaml:"timeout_minutes"`
    Enabled         bool   `yaml:"enabled"`
    Resources       ResourceConfig `yaml:"resources,omitempty"`
}

type ResourceConfig struct {
    MemoryMB       int     `yaml:"memory_mb,omitempty"`
    CPULimit       float64 `yaml:"cpu_limit,omitempty"`  // CPU cores (e.g., 0.5 = 50%)
    PidsLimit      int     `yaml:"pids_limit,omitempty"` // Max processes
}

// Configuration for the grader registry
type GraderRegistry struct {
    Assignments map[string]AssignmentConfig `yaml:"assignments"`
}



// Rate limiter storage
type RateLimitManager struct {
    limiters map[string]*rate.Limiter
    mutex    sync.RWMutex
}

// Global variables
var (
    config           *Config
    jobQueue         *JobQueue
    rateLimitManager *RateLimitManager
)

//------------------------------------------------------------------------------
// Rate Limiting Functions

// Initialize rate limit manager
func newRateLimitManager() *RateLimitManager {
    return &RateLimitManager{
        limiters: make(map[string]*rate.Limiter),
    }
}

// Get or create rate limiter for IP
func (rlm *RateLimitManager) getLimiter(ip string) *rate.Limiter {
    rlm.mutex.Lock()
    defer rlm.mutex.Unlock()
    
    limiter, exists := rlm.limiters[ip]
    if !exists {
        // Create new limiter with burst = maxRequests and refill rate
        // Rate: requests per window converted to requests per second
        requestsPerSecond := float64(config.RateLimitRequests) / config.RateLimitWindow.Seconds()
        limiter = rate.NewLimiter(rate.Limit(requestsPerSecond), config.RateLimitRequests)
        rlm.limiters[ip] = limiter
        
        fmt.Printf("🚦 Created rate limiter for IP %s: %.4f req/sec, burst %d\n", 
            ip, requestsPerSecond, config.RateLimitRequests)
    }
    
    return limiter
}

// Clean up old limiters periodically
func (rlm *RateLimitManager) cleanup() {
    ticker := time.NewTicker(time.Hour) // Clean up every hour
    defer ticker.Stop()
    
    // Clean up unused limiters
    for {
        select {
        case <-ticker.C:
            rlm.mutex.Lock()
            
            // Remove limiters that haven't been used recently
            for ip, limiter := range rlm.limiters {

                // If limiter has full tokens, it hasn't been used recently
                if limiter.Tokens() >= float64(config.RateLimitRequests) {
                    delete(rlm.limiters, ip)
                }
            }
            
            rlm.mutex.Unlock()
            fmt.Printf("🧹 Cleaned up unused rate limiters\n")
        }
    }
}

// Rate limiting middleware
func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        fmt.Printf("🚦 Rate limiting middleware called for %s %s\n", r.Method, r.URL.Path)
        
        if !config.RateLimitEnabled {
            fmt.Printf("🚦 Rate limiting is DISABLED\n")
            next(w, r)
            return
        }
        
        // Get limiter information
        clientIP := getClientIP(r)
        limiter := rateLimitManager.getLimiter(clientIP)
        
        // Show if rate limit exceeded for IP address
        if !limiter.Allow() {
            fmt.Printf("❌ Rate limit exceeded for IP: %s\n", clientIP)
            w.WriteHeader(http.StatusTooManyRequests)
            json.NewEncoder(w).Encode(ErrorResponse{
                Error: fmt.Sprintf("Rate limit exceeded. Maximum %d requests per %v allowed.", 
                    config.RateLimitRequests, config.RateLimitWindow),
            })
            return
        }
        
        next(w, r)
    }
}

//------------------------------------------------------------------------------
// Job Queue Functions



// Load grader registry from YAML file
func loadGraderRegistry() (*GraderRegistry, error) {
    data, err := os.ReadFile(config.GraderRegistryPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read registry file: %v", err)
    }
    
    var registry GraderRegistry
    err = yaml.Unmarshal(data, &registry)
    if err != nil {
        return nil, fmt.Errorf("failed to parse registry YAML: %v", err)
    }
    
    return &registry, nil
}

// Get assignment configuration and validate
func getAssignmentConfig(assignmentID string) (*AssignmentConfig, error) {

    // Load the grader registry
    registry, err := loadGraderRegistry()
    if err != nil {
        return nil, err
    }
    
    // Check if assignment exists in the registry
    assignment, exists := registry.Assignments[assignmentID]
    if !exists {
        return nil, fmt.Errorf("assignment '%s' not found in registry", assignmentID)
    }
    
    // Validate assignment configuration
    if !assignment.Enabled {
        return nil, fmt.Errorf("assignment '%s' is disabled", assignmentID)
    }
    
    return &assignment, nil
}



//------------------------------------------------------------------------------
// Utility Functions

// Generate UUID7-based job ID
func generateJobID() string {

    // Try UUID7 first (best option)
    if u, err := uuid.NewV7(); err == nil {

        // Encode as Base64 for shorter representation (22 chars vs 36)
        return base64.RawURLEncoding.EncodeToString(u[:])
    }
    
    // Fallback to UUID4 if UUID7 fails
    u := uuid.New()

    return base64.RawURLEncoding.EncodeToString(u[:])
}

// Extract assignment ID from request (form, query, or header)
func getAssignmentID(r *http.Request) string {

    // Try form field first
    if assignmentID := r.FormValue("assignment_id"); assignmentID != "" {
        return assignmentID
    }
    
    // Try query parameter
    if assignmentID := r.URL.Query().Get("assignment"); assignmentID != "" {
        return assignmentID
    }
    
    // Try header
    if assignmentID := r.Header.Get("X-Assignment-ID"); assignmentID != "" {
        return assignmentID
    }
    
    return ""
}

// Validate assignment ID to prevent path traversal and injection
func isValidAssignmentID(assignmentID string) bool {

    // Must be alphanumeric with dashes and underscores only
    // Length between 1 and 50 characters
    if len(assignmentID) == 0 || len(assignmentID) > 50 {
        return false
    }
    
    for _, char := range assignmentID {
        if !((char >= 'a' && char <= 'z') || 
             (char >= 'A' && char <= 'Z') || 
             (char >= '0' && char <= '9') || 
             char == '-' || char == '_') {
            return false
        }
    }
    
    // Check if assignment exists and is enabled in registry
    _, err := getAssignmentConfig(assignmentID)

    return err == nil
}

//------------------------------------------------------------------------------
// Main entry point

// Initializes the server, loads configuration, and starts the API
func main() {

    // Load configuration (from environment variables or defaults)
    config = loadConfig()
    
    // Print configuration on startup
    fmt.Printf("⚙️ ByteGrader API starting with configuration:\n")
    fmt.Printf("   Port: %s\n", config.Port)
    fmt.Printf("   Max file size: %d MB\n", config.MaxFileSize/(1024*1024))
    fmt.Printf("   Grading timeout: %v\n", config.GradingTimeout)
    fmt.Printf("   Cleanup interval: %v\n", config.CleanupInterval)
    fmt.Printf("   Completed job TTL: %v\n", config.CompletedJobTTL)
    fmt.Printf("   Failed job TTL: %v\n", config.FailedJobTTL)
    fmt.Printf("   Old file TTL: %v\n", config.OldFileTTL)
    fmt.Printf("   Queue buffer size: %d\n", config.QueueBufferSize)
    fmt.Printf("   Max concurrent jobs: %d\n", config.MaxConcurrentJobs)
    fmt.Printf("   Max Queue Size: %d\n", config.MaxQueueSize)
	fmt.Printf("   Grading registry path: %s\n", config.GraderRegistryPath)
    fmt.Println("")

    // Test Docker availability using Docker SDK
    ctx := context.Background()
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil {
        log.Fatalf("❌ Failed to create Docker client: %v", err)
    }
    defer cli.Close()

    // Check if Docker is running and accessible
    info, err := cli.Info(ctx)
    if err != nil {
        log.Fatalf("❌ Failed to connect to Docker: %v", err)
    }
    fmt.Printf("🐳 Connected to Docker: %s (API %s)\n", info.Name, info.ServerVersion)
    
    // Print security configuration
    fmt.Printf("🔐 Security configuration:\n")

    // Print API key configuration
    fmt.Printf("   API Key Required: %v\n", config.RequireAPIKey)
    if config.RequireAPIKey {
        fmt.Printf("   Valid API Keys: %d configured\n", len(config.ValidAPIKeys))
        for i, key := range config.ValidAPIKeys {
            if len(key) > 3 {
                fmt.Printf("     Key %d: %s... (%d chars)\n", i+1, key[:3], len(key))
            } else {
                fmt.Printf("     Key %d: %s (%d chars)\n", i+1, key, len(key))
            }
        }
    }

    // Print note about API key usage
    if config.RequireAPIKey {
        fmt.Println("   API key required for all endpoints except /health")
        fmt.Println("   Send API key in 'X-API-Key' header or 'Authorization: Bearer {key}'")
    } else {
        fmt.Println("   ⚠️  WARNING: API key authentication is DISABLED")
        fmt.Println("   Set REQUIRE_API_KEY=true for production use")
    }

    // Print IP whitelist configuration
    if len(config.AllowedIPs) == 0 {
        fmt.Printf("   IP Whitelist: DISABLED (allow all IPs) - ⚠️  DEVELOPMENT ONLY\n")
    } else {
        fmt.Printf("   IP Whitelist: %d IP(s) configured\n", len(config.AllowedIPs))
        for _, ip := range config.AllowedIPs {
            fmt.Printf("     - %s\n", ip)
        }
    }

    // Print CORS information
    fmt.Printf("   Note: CORS is permissive because IP whitelist provides primary security\n")

    // Print rate limiting configuration
    if config.RateLimitEnabled {
        fmt.Printf("   Rate Limiting: ENABLED (%d requests per %v)\n", config.RateLimitRequests, config.RateLimitWindow)
    } else {
        fmt.Printf("   Rate Limiting: DISABLED (no limits on requests) - ⚠️  DEVELOPMENT ONLY\n")
    }
    
    // Security level assessment
    if !config.RequireAPIKey && len(config.AllowedIPs) == 0 {
        fmt.Printf("   ⚠️  SECURITY LEVEL: NONE (No protection) - DEVELOPMENT ONLY\n")
    } else if !config.RequireAPIKey {
        fmt.Printf("   🛡️  SECURITY LEVEL: BASIC (IP whitelist only)\n")
    } else if len(config.AllowedIPs) == 0 {
        fmt.Printf("   🛡️  SECURITY LEVEL: MODERATE (API key only)\n")
    } else {
        fmt.Printf("   🔒 SECURITY LEVEL: MAXIMUM (API key + IP whitelist)\n")
    }
    fmt.Println("")

    // Initialize queue with configured buffer size
    jobQueue = &JobQueue{
        jobs:  make(map[string]*Job),
        queue: make(chan string, config.QueueBufferSize),
    }

    // Initialize rate limit manager
    rateLimitManager = newRateLimitManager()

    // Start background services
    go jobQueue.startWorker()
    go jobQueue.startCleanup()
    go rateLimitManager.cleanup() // Clean up old rate limiters

    // Create a custom mux to handle CORS globally
    mux := http.NewServeMux()
    
    // API endpoints with security and rate limiting
    mux.HandleFunc("/submit", protectedEndpoint(submitHandler))
    mux.HandleFunc("/status/", protectedEndpoint(statusHandler))
    mux.HandleFunc("/queue", protectedEndpoint(queueStatusHandler))
    mux.HandleFunc("/config", protectedEndpoint(configHandler))
    
    // Health endpoint without security (for load balancer checks)
    mux.HandleFunc("/health", healthHandler)

    // Print API startup information
    fmt.Printf("🚀 ByteGrader API running on port %s\n", config.Port)
    fmt.Println("📋 Endpoints:")
    fmt.Println("   POST /submit - Submit file for grading (returns job_id)")
    fmt.Println("   GET  /status/{job_id} - Check job status")
    fmt.Println("   GET  /queue - View queue status")
    fmt.Println("   GET  /config - View current configuration")
    fmt.Println("   GET  /health - Health check (no auth required)")
    fmt.Println("")

    // List available assignments from registry
    fmt.Println("📂 Available assignments:")
    registry, err := loadGraderRegistry()
    if err != nil {
        fmt.Printf("   ❌ Error reading grader registry: %v\n", err)
        fmt.Printf("   Expected registry file: %s\n", config.GraderRegistryPath)
    } else {
        if len(registry.Assignments) == 0 {
            fmt.Println("   ❌ No assignments found in registry")
        } else {
            fmt.Println("   Use one of the following assignment IDs:")
            for assignmentID, assignment := range registry.Assignments {
                status := "✅ enabled"
                if !assignment.Enabled {
                    status = "❌ disabled"
                }
                fmt.Printf("     - %s (%s) -> %s\n", assignmentID, status, assignment.Image)
            }
        }
    }
    
    // Start the server
    log.Fatal(http.ListenAndServe(":"+config.Port, mux))
}
