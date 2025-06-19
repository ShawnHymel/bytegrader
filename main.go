package main

import (
    "context"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net"
    "net/http"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/google/uuid"
    "github.com/testcontainers/testcontainers-go"
    "golang.org/x/time/rate"
    "gopkg.in/yaml.v3"
)

//------------------------------------------------------------------------------
// Structs and Types

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
    UploadsDir          string
    GraderRegistryPath  string        // Path to grader registry file

    // Security configuration
    RequireAPIKey       bool          // Enable API key authentication
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

// Configuration for each assignment
type AssignmentConfig struct {
    Image           string `yaml:"image"`
    Description     string `yaml:"description"`
    TimeoutMinutes  int    `yaml:"timeout_minutes"`
    Enabled         bool   `yaml:"enabled"`
}

// Configuration for the grader registry
type GraderRegistry struct {
    Assignments map[string]AssignmentConfig `yaml:"assignments"`
}

// Job represents a grading job
type Job struct {
    ID       string    `json:"id"`
    Filename string    `json:"filename"`
    FilePath string    `json:"-"` // Don't expose file path in JSON
    Size     int64     `json:"size"`
    Status   string    `json:"status"` // "queued", "processing", "completed", "failed"
    Result   *JobResult `json:"result,omitempty"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    AssignmentID string `json:"assignment_id,omitempty"` // Which assignment this is for
}

// JobResult represents the grading result
type JobResult struct {
    Score    float64 `json:"score"`
    Feedback string  `json:"feedback"`
    Error    string  `json:"error,omitempty"`
}

// Queue responses
type SubmitResponse struct {
    JobID   string `json:"job_id"`
    Status  string `json:"status"`
    Message string `json:"message"`
}

// Job status response
type StatusResponse struct {
    Job *Job `json:"job"`
}

// Job error response
type ErrorResponse struct {
    Error string `json:"error"`
}

// Simple in-memory queue
type JobQueue struct {
    jobs            map[string]*Job
    queue           chan string
    mutex           sync.RWMutex
    isRunning       bool
    activeJobs      int           // Current number of processing jobs
    activeJobsMutex sync.Mutex    // Mutex for activeJobs counter
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
        
        clientIP := getClientIP(r)
        limiter := rateLimitManager.getLimiter(clientIP)
        
        // Debug: Show current limiter state
        fmt.Printf("🚦 Rate check for IP %s: tokens=%.2f, limit=%.4f\n", 
            clientIP, limiter.Tokens(), float64(limiter.Limit()))
        
        if !limiter.Allow() {
            fmt.Printf("❌ Rate limit exceeded for IP: %s\n", clientIP)
            w.WriteHeader(http.StatusTooManyRequests)
            json.NewEncoder(w).Encode(ErrorResponse{
                Error: fmt.Sprintf("Rate limit exceeded. Maximum %d requests per %v allowed.", 
                    config.RateLimitRequests, config.RateLimitWindow),
            })
            return
        }
        
        fmt.Printf("✅ Rate limit passed for IP: %s (tokens remaining: %.2f)\n", 
            clientIP, limiter.Tokens())
        
        next(w, r)
    }
}

//------------------------------------------------------------------------------
// Security Functions

// Set permissive CORS headers for browser compatibility (as we use IP whitelisting)
func setCORSHeaders(w http.ResponseWriter) {
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Access-Control-Allow-Credentials", "true")
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, X-API-Key")
    w.Header().Set("Access-Control-Max-Age", "86400")
    w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type")
}

// Validate the API key if authentication is required
func authenticateRequest(r *http.Request) bool {
    if !config.RequireAPIKey {
        return true // Skip auth if disabled
    }
    
    // Check for API key in header
    apiKey := r.Header.Get("X-API-Key")
    if apiKey == "" {
        // Also check Authorization header as Bearer token
        authHeader := r.Header.Get("Authorization")
        if strings.HasPrefix(authHeader, "Bearer ") {
            apiKey = strings.TrimPrefix(authHeader, "Bearer ")
        }
    }
    
    // Validate against configured API keys
    for _, validKey := range config.ValidAPIKeys {
        if apiKey == validKey {
            return true
        }
    }
    
    return false
}

// Extract client IP from request headers
func getClientIP(r *http.Request) string {
    
    // Check X-Forwarded-For header (most common for proxies/load balancers)
    xff := r.Header.Get("X-Forwarded-For")
    if xff != "" {

        // X-Forwarded-For can contain multiple IPs, take the first one
        ips := strings.Split(xff, ",")
        return strings.TrimSpace(ips[0])
    }
    
    // Check X-Real-IP header (used by some proxies)
    realIP := r.Header.Get("X-Real-IP")
    if realIP != "" {
        return realIP
    }
    
    // Check CF-Connecting-IP header (Cloudflare)
    cfIP := r.Header.Get("CF-Connecting-IP")
    if cfIP != "" {
        return cfIP
    }
    
    // Fall back to RemoteAddr (direct connection)
    ip, _, err := net.SplitHostPort(r.RemoteAddr)
    if err != nil {
        return r.RemoteAddr // Return as-is if parsing fails
    }
    
    return ip
}

// Check if the request comes from an allowed IP
func validateSourceIP(r *http.Request) bool {
    // If no IP whitelist configured, allow all IPs
    if len(config.AllowedIPs) == 0 {
        return true
    }
    
    clientIP := getClientIP(r)
    
    // Special case: allow localhost for development
    if clientIP == "127.0.0.1" || clientIP == "::1" || clientIP == "localhost" {
        
        // Only allow localhost if explicitly configured
        for _, allowedIP := range config.AllowedIPs {
            if allowedIP == "127.0.0.1" || allowedIP == "localhost" {
                return true
            }
        }
    }
    
    // Check against whitelist
    for _, allowedIP := range config.AllowedIPs {
        if clientIP == allowedIP {
            return true
        }
        
        // Check if it's a CIDR block (e.g., 192.168.1.0/24)
        if strings.Contains(allowedIP, "/") {
            _, ipNet, err := net.ParseCIDR(allowedIP)
            if err == nil && ipNet.Contains(net.ParseIP(clientIP)) {
                return true
            }
        }
    }
    
    return false
}

// securityMiddleware applies all security checks to requests
func securityMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Debug logging
        clientIP := getClientIP(r)
        fmt.Printf("🔍 Security check for %s %s from IP: %s\n", r.Method, r.URL.Path, clientIP)
        fmt.Printf("   API Key Required: %v\n", config.RequireAPIKey)
        fmt.Printf("   X-API-Key header: '%s'\n", r.Header.Get("X-API-Key"))
        
        // 1. Set CORS headers for browser compatibility
        setCORSHeaders(w)
        
        // Handle preflight requests
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        
        // 2. Check IP whitelist (primary security)
        if !validateSourceIP(r) {
            fmt.Printf("❌ IP validation failed for %s %s from %s\n", r.Method, r.URL.Path, clientIP)
            w.WriteHeader(http.StatusForbidden)
            json.NewEncoder(w).Encode(ErrorResponse{Error: "IP address not allowed"})
            return
        }
        
        // 3. Check API key (authentication)
        if !authenticateRequest(r) {
            fmt.Printf("❌ Authentication failed for %s %s\n", r.Method, r.URL.Path)
            w.WriteHeader(http.StatusUnauthorized)
            json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid or missing API key"})
            return
        }
        
        fmt.Printf("✅ All security checks passed for %s %s from %s\n", r.Method, r.URL.Path, clientIP)
        
        // All security checks passed, proceed to handler
        next(w, r)
    }
}

// Combined middleware that applies rate limiting and security
func protectedEndpoint(handler http.HandlerFunc) http.HandlerFunc {
    return securityMiddleware(rateLimitMiddleware(handler))
}

//------------------------------------------------------------------------------
// Configuration Functions

//  Load server configuration settings
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
        UploadsDir:          getEnv("UPLOADS_DIR", "/tmp/uploads"),
        GraderRegistryPath: getEnv("GRADER_REGISTRY_PATH", "/usr/local/bin/graders/registry.yaml"),
        
        // Security configuration
        RequireAPIKey:       getEnvBool("REQUIRE_API_KEY", false),
        ValidAPIKeys:        parseAPIKeys(getEnv("VALID_API_KEYS", "")),
        AllowedIPs:          parseAllowedIPs(getEnv("ALLOWED_IPS", "")),
        
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

//------------------------------------------------------------------------------
// HTTP Handlers

// Accept file uploads and queues them for processing
func submitHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")

    if r.Method != "POST" {
        w.WriteHeader(http.StatusMethodNotAllowed)
        json.NewEncoder(w).Encode(ErrorResponse{Error: "Only POST method allowed"})
        return
    }

    // Parse the multipart form with configured max file size
    err := r.ParseMultipartForm(config.MaxFileSize)
    if err != nil {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(ErrorResponse{Error: "Unable to parse form - file may be too large"})
        return
    }

    // Get the file from form data
    file, header, err := r.FormFile("file")
    if err != nil {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(ErrorResponse{Error: "Unable to get file from form"})
        return
    }
    defer file.Close()

    // Check file size against configured limit
    if header.Size > config.MaxFileSize {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(ErrorResponse{
            Error: fmt.Sprintf("File too large. Maximum size: %d MB", config.MaxFileSize/(1024*1024)),
        })
        return
    }

    // Create job ID and file path
    jobID := generateJobID()
    
    // Create uploads directory if it doesn't exist
    os.MkdirAll(config.UploadsDir, 0755)
    
    // Construct the file path for saving
    filePath := fmt.Sprintf("%s/%s_%s", config.UploadsDir, jobID, header.Filename)
    
    // Read and save file
    fileContents, err := io.ReadAll(file)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(ErrorResponse{Error: "Unable to read file"})
        return
    }
    
    // Write file to disk
    err = os.WriteFile(filePath, fileContents, 0644)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(ErrorResponse{Error: "Unable to save file"})
        return
    }

	// Get assignment ID from form data, query param, or header
	assignmentID := getAssignmentID(r)
	if assignmentID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Assignment ID required (form field, query param, or X-Assignment-ID header)"})
		return
	}

	// Validate assignment ID (prevent path traversal)
	if !isValidAssignmentID(assignmentID) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid assignment ID format"})
		return
	}

    // Create job (no file contents in RAM)
    job := &Job{
        ID:        jobID,
        Filename:  header.Filename,
        Size:      header.Size,
        Status:    "queued",
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
        FilePath:  filePath,
		AssignmentID: assignmentID,
    }

    // Add to queue
    jobQueue.addJob(job)

    fmt.Printf("📁 File saved: %s (Job: %s)\n", filePath, jobID)

    // Return job ID immediately
    response := SubmitResponse{
        JobID:   jobID,
        Status:  "queued",
        Message: "File submitted for grading. Use job_id to check status.",
    }

    json.NewEncoder(w).Encode(response)
}

// Return the status of a specific job
func statusHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")

    // Extract job ID from URL path
    jobID := r.URL.Path[len("/status/"):]
    if jobID == "" {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(ErrorResponse{Error: "Job ID required"})
        return
    }

    // Validate job ID format
    job := jobQueue.getJob(jobID)
    if job == nil {
        w.WriteHeader(http.StatusNotFound)
        json.NewEncoder(w).Encode(ErrorResponse{Error: "Job not found"})
        return
    }

    // Check if job is still processing
    response := StatusResponse{Job: job}
    json.NewEncoder(w).Encode(response)
}

// Return overall queue information
func queueStatusHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    
    queueLength := len(jobQueue.queue)
    totalJobs := len(jobQueue.jobs)
    
    jobQueue.activeJobsMutex.Lock()
    activeJobs := jobQueue.activeJobs
    jobQueue.activeJobsMutex.Unlock()
    
    response := map[string]interface{}{
        "queue_length":    queueLength,
        "total_jobs":      totalJobs,
        "active_jobs":     activeJobs,
        "max_queue_size":  config.MaxQueueSize,
        "max_concurrent":  config.MaxConcurrentJobs,
        "worker_running":  jobQueue.isRunning,
    }
    
    json.NewEncoder(w).Encode(response)
}

// Perform a health check
func healthHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Return current configuration (for debugging/monitoring)
func configHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    
    configInfo := map[string]interface{}{
        "max_file_size_mb":        config.MaxFileSize / (1024 * 1024),
        "grading_timeout_minutes": int(config.GradingTimeout.Minutes()),
        "cleanup_interval_hours":  int(config.CleanupInterval.Hours()),
        "completed_job_ttl_hours": int(config.CompletedJobTTL.Hours()),
        "failed_job_ttl_hours":    int(config.FailedJobTTL.Hours()),
        "old_file_ttl_hours":      int(config.OldFileTTL.Hours()),
        "queue_buffer_size":       config.QueueBufferSize,
        "uploads_dir":             config.UploadsDir,
        "grader_registry_path":    config.GraderRegistryPath,
        "require_api_key":         config.RequireAPIKey,
        "ip_whitelist_enabled":    len(config.AllowedIPs) > 0,
        "allowed_ips_count":       len(config.AllowedIPs),
        "api_keys_configured":     len(config.ValidAPIKeys),
        "rate_limit_enabled":      config.RateLimitEnabled,
        "rate_limit_requests":     config.RateLimitRequests,
        "rate_limit_window_min":   int(config.RateLimitWindow.Minutes()),
        "max_concurrent_jobs":     config.MaxConcurrentJobs,
        "max_queue_size":          config.MaxQueueSize,
    }
    
    json.NewEncoder(w).Encode(configInfo)
}

//------------------------------------------------------------------------------
// Job Queue Functions

// Add a job to the queue and map it to its job ID
func (q *JobQueue) addJob(job *Job) {
    q.mutex.Lock()
    defer q.mutex.Unlock()
    
    q.jobs[job.ID] = job
    q.queue <- job.ID
    
    fmt.Printf("Job %s queued (%s)\n", job.ID, job.Filename)
}

// Get a job by ID from the queue
func (q *JobQueue) getJob(jobID string) *Job {
    q.mutex.RLock()
    defer q.mutex.RUnlock()
    
    return q.jobs[jobID]
}

// Update job status and result in the queue
func (q *JobQueue) updateJob(jobID string, status string, result *JobResult) {
    q.mutex.Lock()
    defer q.mutex.Unlock()
    
    if job, exists := q.jobs[jobID]; exists {
        job.Status = status
        job.Result = result
        job.UpdatedAt = time.Now()
    }
}

// Worker that processes jobs one by one
func (q *JobQueue) startWorker() {
    q.isRunning = true
    fmt.Printf("🔄 Worker started - processing jobs (max concurrent: %d)...\n", config.MaxConcurrentJobs)
    
    // Create a semaphore to limit concurrent jobs
    semaphore := make(chan struct{}, config.MaxConcurrentJobs)
    
    // Process jobs from the queue
    for jobID := range q.queue {

        // Wait for available slot
        semaphore <- struct{}{}
        
        // Increment active jobs counter
        q.activeJobsMutex.Lock()
        q.activeJobs++
        q.activeJobsMutex.Unlock()
        
        go func(jobID string) {
            defer func() {
                // Release semaphore slot
                <-semaphore
                
                // Decrement active jobs counter
                q.activeJobsMutex.Lock()
                q.activeJobs--
                q.activeJobsMutex.Unlock()
            }()
            
            fmt.Printf("⚡ Processing job %s... (active: %d/%d)\n", jobID, q.activeJobs, config.MaxConcurrentJobs)
            
            // Update status to processing
            q.updateJob(jobID, "processing", nil)
            
            // Process the job
            result := q.processJob(jobID)
            
            // Update with result and cleanup file if failed
            if result.Error != "" {
                q.updateJob(jobID, "failed", result)
                fmt.Printf("❌ Job %s failed: %s\n", jobID, result.Error)
                
                // Clean up file for failed jobs
                job := q.getJob(jobID)
                if job != nil {
                    q.cleanupFile(job.FilePath, jobID, "job failed")
                }
            } else {
                q.updateJob(jobID, "completed", result)
                fmt.Printf("✅ Job %s completed (Score: %.1f)\n", jobID, result.Score)
            }
        }(jobID)
    }
}

// Calls the Python script for grading
func (q *JobQueue) processJob(jobID string) *JobResult {
    job := q.getJob(jobID)
    if job == nil {
        return &JobResult{Error: "Job not found"}
    }
    
    // Create isolated grading directory
    tempDir := fmt.Sprintf("/tmp/grading_%s", jobID)
    err := os.MkdirAll(tempDir, 0755)
    if err != nil {
        return &JobResult{Error: "Failed to create grading directory"}
    }
    defer os.RemoveAll(tempDir) // Always cleanup

    // Log the grading start
    fmt.Printf("🔬 Starting grading in %s\n", tempDir)

    // Copy student submission to grading directory
    submissionPath := filepath.Join(tempDir, "submission.zip")
    err = q.copyFile(job.FilePath, submissionPath)
    if err != nil {
        return &JobResult{Error: "Failed to copy submission for grading"}
    }

    // Run Python grading script with timeout
    return q.runPythonGrader(tempDir, submissionPath, job.Filename)
}

// Copy a file from src to dst
func (q *JobQueue) copyFile(src, dst string) error {
    sourceFile, err := os.Open(src)
    if err != nil {
        return err
    }
    defer sourceFile.Close()

    destFile, err := os.Create(dst)
    if err != nil {
        return err
    }
    defer destFile.Close()

    _, err = io.Copy(destFile, sourceFile)
    return err
}

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

// Execute the Python grading script
func (q *JobQueue) runPythonGrader(tempDir, submissionPath, originalFilename string) *JobResult {
    
	// TODO: Replace this entire function with container spawning
    return &JobResult{
        Error: "Container grading not yet implemented - coming soon!",
    }
}

// Convert Python script JSON output to JobResult
func (q *JobQueue) parseGradingOutput(stdout, stderr string) *JobResult {
    fmt.Printf("🔍 Debug - Python stdout: %s\n", stdout)
    fmt.Printf("🔍 Debug - Python stderr: %s\n", stderr)
    
    // Try to parse JSON output from Python script
    var gradingResult struct {
        Score       float64 `json:"score"`
        MaxScore    float64 `json:"max_score"`
        Feedback    string  `json:"feedback"`
        TestResults []struct {
            Name    string  `json:"name"`
            Passed  bool    `json:"passed"`
            Message string  `json:"message"`
            Points  float64 `json:"points"`
        } `json:"test_results"`
        Error string `json:"error"`
    }
    
    // Clean up stdout and try to find JSON
    stdout = strings.TrimSpace(stdout)
    
    // Try to parse the entire stdout as JSON first
    err := json.Unmarshal([]byte(stdout), &gradingResult)
    if err == nil {
        fmt.Printf("✅ Successfully parsed JSON directly\n")
        if gradingResult.Error != "" {
            return &JobResult{Error: gradingResult.Error}
        }
        
        return &JobResult{
            Score:    gradingResult.Score,
            Feedback: gradingResult.Feedback,
        }
    }
    
    // If parsing the entire stdout fails, log the error
    fmt.Printf("❌ Failed to parse stdout as JSON directly: %v\n", err)
    
    // Split stdout into lines and try to find valid JSON
    lines := strings.Split(stdout, "\n")
    for i, line := range lines {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
            fmt.Printf("🔍 Trying to parse line %d: %s\n", i, line)
            err := json.Unmarshal([]byte(line), &gradingResult)
            if err == nil {
                fmt.Printf("✅ Successfully parsed JSON from line %d\n", i)
                if gradingResult.Error != "" {
                    return &JobResult{Error: gradingResult.Error}
                }
                
                return &JobResult{
                    Score:    gradingResult.Score,
                    Feedback: gradingResult.Feedback,
                }
            } else {
                fmt.Printf("❌ Failed to parse line %d: %v\n", i, err)
            }
        }
    }
    
    // Fallback: if no valid JSON found, return error with output
    return &JobResult{
        Error: fmt.Sprintf("Python grader did not return valid JSON.\nStdout: %s\nStderr: %s", stdout, stderr),
    }
}

// Remove a file and log the action
func (q *JobQueue) cleanupFile(filePath, jobID, reason string) {
    if filePath == "" {
        return
    }
    
    err := os.Remove(filePath)
    if err != nil {
        fmt.Printf("⚠️  Failed to cleanup file %s (Job: %s): %v\n", filePath, jobID, err)
    } else {
        fmt.Printf("🗑️  Cleaned up file %s (Job: %s) - %s\n", filePath, jobID, reason)
    }
}

// Run periodic cleanup of old files and jobs
func (q *JobQueue) startCleanup() {
    fmt.Printf("🧹 Cleanup service started - checking every %v...\n", config.CleanupInterval)
    
    ticker := time.NewTicker(config.CleanupInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            q.performCleanup()
        }
    }
}

// Remove old files and stale job records
func (q *JobQueue) performCleanup() {
    fmt.Println("🧹 Starting cleanup routine...")
    
    q.mutex.Lock()
    defer q.mutex.Unlock()
    
    now := time.Now()
    cleanedFiles := 0
    cleanedJobs := 0
    
    // Check all jobs for cleanup candidates
    for jobID, job := range q.jobs {
        shouldCleanup := false
        reason := ""
        
        // Cleanup criteria using configured TTLs
        if job.CreatedAt.Before(now.Add(-config.OldFileTTL)) {
            shouldCleanup = true
            reason = fmt.Sprintf("older than %v", config.OldFileTTL)
        } else if job.Status == "failed" && job.UpdatedAt.Before(now.Add(-config.FailedJobTTL)) {
            shouldCleanup = true
            reason = fmt.Sprintf("failed job older than %v", config.FailedJobTTL)
        } else if job.Status == "completed" && job.UpdatedAt.Before(now.Add(-config.CompletedJobTTL)) {
            shouldCleanup = true
            reason = fmt.Sprintf("completed job older than %v", config.CompletedJobTTL)
        }
        
        if shouldCleanup {
            // Remove file if it exists
            if job.FilePath != "" {
                err := os.Remove(job.FilePath)
                if err == nil {
                    cleanedFiles++
                    fmt.Printf("🗑️  Cleaned up old file: %s (Job: %s) - %s\n", job.FilePath, jobID, reason)
                }
            }
            
            // Remove job from memory
            delete(q.jobs, jobID)
            cleanedJobs++
        }
    }
    
    fmt.Printf("🧹 Cleanup complete: %d files removed, %d jobs removed\n", cleanedFiles, cleanedJobs)
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
    fmt.Printf("   Uploads directory: %s\n", config.UploadsDir)
    fmt.Printf("   Max concurrent jobs: %d\n", config.MaxConcurrentJobs)
    fmt.Printf("   Max Queue Size: %d\n", config.MaxQueueSize)
	fmt.Printf("   Grading registry path: %s\n", config.GraderRegistryPath)
    fmt.Printf("   Uploads Directory: %s (files will be stored here)\n", config.UploadsDir)
    fmt.Println("")

    // Test Docker availability using testcontainers
    ctx := context.Background()
    provider, err := testcontainers.NewDockerProvider()
    if err != nil {
        log.Fatalf("❌ Failed to create Docker provider: %v", err)
    }
    defer provider.Close()

    // Check if Docker is running and accessible
    info, err := provider.DaemonHost(ctx)
    if err != nil {
        log.Fatalf("❌ Failed to connect to Docker: %v", err)
    }
    fmt.Printf("🐳 Connected to Docker: %s\n", info)
    
    // Print security configuration
    fmt.Printf("🔐 Security configuration:\n")

    // Print API key configuration
    fmt.Printf("   API Key Required: %v\n", config.RequireAPIKey)
    if config.RequireAPIKey {
        fmt.Printf("   Valid API Keys: %d configured\n", len(config.ValidAPIKeys))
        // Debug: Print first few characters of each key for verification
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

    // Create uploads directory
    os.MkdirAll(config.UploadsDir, 0755)

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
