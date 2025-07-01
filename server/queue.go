package main

import (
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"
)

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
    Username     string `json:"username,omitempty"`      // User who submitted this job
}

// JobResult represents the grading result
type JobResult struct {
    Score    float64 `json:"score"`
    Feedback string  `json:"feedback"`
    Error    string  `json:"error,omitempty"`
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

// Add a job to the queue and map it to its job ID
func (q *JobQueue) addJob(job *Job, username string) {
    q.mutex.Lock()
    defer q.mutex.Unlock()

    // Generate a unique job ID
    job.Username = username
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
