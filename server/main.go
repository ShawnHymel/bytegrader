// Implements the ByteGrader autograder API server. Provides HTTP endpoints for submitting code for
// grading and checking job status.
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"

    "github.com/docker/docker/client"
)

// Version information (injected at build time)
var (
    Version   = "dev"      // Overridden by -ldflags during build
    BuildTime = "unknown"  // Overridden by -ldflags during build
    GitCommit = "unknown"  // Overridden by -ldflags during build
)

// Global variables
var (
    config           *Config
    jobQueue         *JobQueue
    rateLimitManager *RateLimitManager
)

// Initializes the server, loads configuration, and starts the API
func main() {

    // Print version information on startup
    fmt.Printf("🚀 ByteGrader API v%s starting...\n", Version)
    fmt.Printf("   Built: %s\n", BuildTime)
    if GitCommit != "unknown" {
        fmt.Printf("   Commit: %s\n", GitCommit)
    }
    fmt.Println("")

    // Load configuration (from environment variables or defaults)
    config = loadConfig()
    
    // Print configuration on startup
    fmt.Printf("⚙️ Configuration:\n")
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

    // Print username requirement
    fmt.Println("   Username required for all protected endpoints")
    fmt.Println("   Send username in 'X-Username' header")

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
    
    // Public endpoints (no auth required)
    mux.HandleFunc("/health", healthHandler)
    mux.HandleFunc("/version", versionHandler)

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
