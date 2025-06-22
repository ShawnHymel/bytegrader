// Implements the ByteGrader autograder API server. Provides HTTP endpoints for submitting code for
// grading and checking job status.
package main

import (
    "context"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "strings"
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
// Configuration Functions



//------------------------------------------------------------------------------
// HTTP Handlers

// Accept file uploads and queues them for processing
func submitHandler(w http.ResponseWriter, r *http.Request) {
    fmt.Printf("📥 Submit handler started\n")
    w.Header().Set("Content-Type", "application/json")

    if r.Method != "POST" {
        w.WriteHeader(http.StatusMethodNotAllowed)
        json.NewEncoder(w).Encode(ErrorResponse{Error: "Only POST method allowed"})
        return
    }

    // Parse the multipart form with configured max file size
    fmt.Printf("📝 Parsing multipart form (max size: %d bytes)\n", config.MaxFileSize)
    err := r.ParseMultipartForm(config.MaxFileSize)
    if err != nil {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(ErrorResponse{Error: "Unable to parse form - file may be too large"})
        return
    }

    // Get the file from form data
    fmt.Printf("📁 Getting file from form\n")
    file, header, err := r.FormFile("file")
    if err != nil {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(ErrorResponse{Error: "Unable to get file from form"})
        return
    }
    defer file.Close()
    fmt.Printf("✅ Got file: %s (size: %d bytes)\n", header.Filename, header.Size)

    // Check file size against configured limit
    if header.Size > config.MaxFileSize {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(ErrorResponse{
            Error: fmt.Sprintf("File too large. Maximum size: %d MB", config.MaxFileSize/(1024*1024)),
        })
        return
    }
    fmt.Printf("✅ File size OK\n")

    // Create job ID and workspace
    jobID := generateJobID()
    jobWorkspace := fmt.Sprintf("/workspace/jobs/%s", jobID)
    fmt.Printf("📋 Creating job %s with workspace %s\n", jobID, jobWorkspace)

    // Create job workspace directories
    submissionDir := filepath.Join(jobWorkspace, "submission")
    resultsDir := filepath.Join(jobWorkspace, "results")

    fmt.Printf("📁 Creating submission directory: %s\n", submissionDir)
    err = os.MkdirAll(submissionDir, 0755)
    if err != nil {
        fmt.Printf("❌ Failed to create submission directory: %v\n", err)
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Unable to create job workspace: %v", err)})
        return
    }
    fmt.Printf("✅ Created submission directory\n")

    fmt.Printf("📁 Creating results directory: %s\n", resultsDir)
    err = os.MkdirAll(resultsDir, 0755)
    if err != nil {
        fmt.Printf("❌ Failed to create results directory: %v\n", err)
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Unable to create results directory: %v", err)})
        return
    }
    fmt.Printf("✅ Created results directory\n")

    // Save directly to job workspace
    filePath := filepath.Join(jobWorkspace, "submission", "submission.zip")

    // Read and save file directly to volume
    fileContents, err := io.ReadAll(file)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(ErrorResponse{Error: "Unable to read file"})
        return
    }

    // Write file directly to volume workspace
    err = os.WriteFile(filePath, fileContents, 0644)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(ErrorResponse{Error: "Unable to save file to workspace"})
        return
    }

    fmt.Printf("📁 File saved directly to workspace: %s\n", filePath)

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

    // Start the worker only if not already running
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

    // Run Python grading script in a container
    return q.runContainerGrader(job, tempDir)
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
    cleanedWorkspaces := 0
    
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
            // Remove old upload file if it exists (from /tmp/uploads - if we still use that)
            if job.FilePath != "" && !strings.Contains(job.FilePath, "/workspace/") {
                err := os.Remove(job.FilePath)
                if err == nil {
                    cleanedFiles++
                    fmt.Printf(
                        "🗑️  Cleaned up old upload file: %s (Job: %s) - %s\n", 
                        job.FilePath, 
                        jobID, 
                        reason,
                    )
                }
            }
            
            // Remove job workspace from volume
            jobWorkspacePath := fmt.Sprintf("/workspace/jobs/%s", jobID)
            if _, err := os.Stat(jobWorkspacePath); err == nil {
                err := os.RemoveAll(jobWorkspacePath)
                if err == nil {
                    cleanedWorkspaces++
                    fmt.Printf("🗑️  Cleaned up job workspace: %s - %s\n", jobWorkspacePath, reason)
                } else {
                    fmt.Printf("⚠️  Failed to cleanup workspace %s: %v\n", jobWorkspacePath, err)
                }
            }
            
            // Remove job from memory
            delete(q.jobs, jobID)
            cleanedJobs++
        }
    }
    
    // Clean up orphaned workspaces (workspaces without corresponding jobs in memory)
    workspacePath := "/workspace/jobs"
    if _, err := os.Stat(workspacePath); err == nil {
        jobDirs, err := os.ReadDir(workspacePath)
        if err == nil {
            for _, dir := range jobDirs {
                if dir.IsDir() {
                    jobID := dir.Name()
                    
                    // If this workspace doesn't have a corresponding job in memory
                    if _, exists := q.jobs[jobID]; !exists {
                        jobWorkspacePath := filepath.Join(workspacePath, jobID)
                        
                        // Check if the workspace is old enough to clean up
                        if info, err := os.Stat(jobWorkspacePath); err == nil {
                            if info.ModTime().Before(now.Add(-config.OldFileTTL)) {
                                err := os.RemoveAll(jobWorkspacePath)
                                if err == nil {
                                    cleanedWorkspaces++
                                    fmt.Printf(
                                        "🗑️  Cleaned up orphaned workspace: %s (no job record)\n", 
                                        jobWorkspacePath,
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }
    }
    
    fmt.Printf(
        "🧹 Cleanup complete: %d upload files removed, %d workspaces removed, %d jobs removed\n", 
        cleanedFiles, 
        cleanedWorkspaces, 
        cleanedJobs,
    )
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
