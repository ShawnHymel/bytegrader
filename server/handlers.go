package main

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strconv"
    "time"
)

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

// Version response structure
type VersionResponse struct {
    Version   string `json:"version"`
    BuildTime string `json:"build_time"`
    GitCommit string `json:"git_commit,omitempty"`
}

// Accept file uploads and queues them for processing
func submitHandler(w http.ResponseWriter, r *http.Request) {
    fmt.Printf("📥 Submit handler started\n")
    w.Header().Set("Content-Type", "application/json")

    // Check if method is POST
    if r.Method != "POST" {
        w.WriteHeader(http.StatusMethodNotAllowed)
        json.NewEncoder(w).Encode(ErrorResponse{Error: "Only POST method allowed"})
        return
    }

    // Pre-check Content-Length header if present
    if contentLength := r.Header.Get("Content-Length"); contentLength != "" {
        if length, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
            if length > config.MaxFileSize * 2 { // Allow 2x for multipart overhead
                w.WriteHeader(http.StatusBadRequest)
                json.NewEncoder(w).Encode(ErrorResponse{
                    Error: fmt.Sprintf("Request too large. Content-Length: %d bytes, Maximum file size: %d MB", 
                        length, config.MaxFileSize/(1024*1024)),
                })
                return
            }
        }
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
            Error: fmt.Sprintf("File too large. File size: %.2f MB, Maximum allowed: %d MB", 
                float64(header.Size)/(1024*1024), config.MaxFileSize/(1024*1024)),
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

// Return version information
func versionHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    
    response := VersionResponse{
        Version:   Version,
        BuildTime: BuildTime,
        GitCommit: GitCommit,
    }
    
    json.NewEncoder(w).Encode(response)
}
