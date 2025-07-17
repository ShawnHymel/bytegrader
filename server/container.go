package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "time"

    "github.com/docker/docker/api/types/container"
    "github.com/docker/docker/api/types/mount"
    "github.com/docker/docker/client"
)

// Run the grading process inside a Docker container
func (q *JobQueue) runContainerGrader(job *Job, tempDir string) *JobResult {

    // Get assignment configuration from registry
    assignmentConfig, err := getAssignmentConfig(job.AssignmentID)
    if err != nil {
        return &JobResult{Error: fmt.Sprintf("Assignment configuration error: %v", err)}
    }
    
    fmt.Printf("🐳 Starting container grading for assignment '%s' with image: %s\n", 
        job.AssignmentID, 
        assignmentConfig.Image,
    )
    
    // Create job-specific directory in shared volume
    jobWorkspace := fmt.Sprintf("/workspace/jobs/%s", job.ID)
    
    // Set up timeout context
    timeout := time.Duration(assignmentConfig.TimeoutMinutes) * time.Minute
    if timeout == 0 {
        timeout = config.GradingTimeout
    }
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    
    // Create Docker client
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil {
        return &JobResult{Error: fmt.Sprintf("Failed to create Docker client: %v", err)}
    }
    defer cli.Close()
    
    // Create grader container with volume mount and environment detection
    resp, err := cli.ContainerCreate(
        ctx, 
        &container.Config{
            Image: assignmentConfig.Image,
            WorkingDir: "/workspace",  // Simplified working directory
            Env: buildEnvironmentVariables(job.ID, assignmentConfig),
            User: fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
        },
        &container.HostConfig{
            Mounts: []mount.Mount{
                {
                    Type:   mount.TypeVolume,
                    Source: "bytegrader-workspace",
                    Target: "/workspace",
                },
            },
            AutoRemove: true, // Automatically remove container after exit
            Resources: container.Resources{
                Memory:   int64(assignmentConfig.Resources.MemoryMB) * 1024 * 1024,
                NanoCPUs: int64(assignmentConfig.Resources.CPULimit * 1e9),
                PidsLimit: func() *int64 {
                    if assignmentConfig.Resources.PidsLimit > 0 {
                        limit := int64(assignmentConfig.Resources.PidsLimit)
                        return &limit
                    }
                    return nil
                }(),
            },
        }, 
        nil, 
        nil, 
        "",
    )
    
    // Check for errors in container creation
    if err != nil {
        return &JobResult{Error: fmt.Sprintf("Failed to create grader container: %v", err)}
    }
    
    // Log the container ID
    containerID := resp.ID
    fmt.Printf("🚀 Launching grading container %s for job %s (assignment: %s, image: %s)...\n", 
        containerID[:12], job.ID, job.AssignmentID, assignmentConfig.Image)

    // Log GRADER_ASSIGNMENT environment variables (if passed in)
    env := buildEnvironmentVariables(job.ID, assignmentConfig)
    for _, envVar := range env {
        if strings.HasPrefix(envVar, "GRADER_ASSIGNMENT=") {
            fmt.Printf("📋 Environment: %s\n", envVar)
            break
        }
    }
    
    // Start the container
    if err := cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
        return &JobResult{Error: fmt.Sprintf("Failed to start grader container: %v", err)}
    }
    
    // Wait for completion using polling
    fmt.Printf("⏳ Waiting for grading (timeout: %v)...\n", timeout)
    
    // Wait for container to complete with timeout (blocking)
    exitCode, err := q.waitForContainerCompletion(ctx, cli, containerID, timeout)
    if err != nil {
        // Stop the container on timeout/error
        cli.ContainerStop(ctx, containerID, container.StopOptions{})
        return &JobResult{Error: fmt.Sprintf("Container failed: %v", err)}
    }

    // Always try to read results first, regardless of exit code
    result := q.readResultsFromSharedVolume(jobWorkspace)

    // Check if the container exited with an error code
    if exitCode != 0 {
        fmt.Printf("⚠️  Container %s exited with code %d\n", containerID[:12], exitCode)
        
        // If we got valid results from output.json, use those (even on non-zero exit)
        if result.Error != "" && result.Error == "No output.json found in results directory" {

            // No valid results file, fall back to container logs
            logs, _ := cli.ContainerLogs(ctx, containerID, container.LogsOptions{
                ShowStdout: true,
                ShowStderr: true,
            })
            if logs != nil {
                logData, _ := io.ReadAll(logs)
                logs.Close()
                return &JobResult{Error: fmt.Sprintf("Grader exited with code %d: %s", exitCode, string(logData))}
            }

            return &JobResult{Error: fmt.Sprintf("Grader exited with code %d", exitCode)}
        }
        
        // We have valid results from output.json, use them even though exit code was non-zero
        fmt.Printf("📋 Using results from output.json despite non-zero exit code\n")
    }

    return result
}

// Wait for container to complete with timeout and status updates (blocking)
func (q *JobQueue) waitForContainerCompletion(
    ctx context.Context, 
    cli *client.Client, 
    containerID string, 
    timeout time.Duration,
) (int64, error) {
    
    fmt.Printf("⏳ Waiting for container %s to complete (timeout: %v)...\n", containerID[:12], timeout)
    
    // Use Docker SDK's ContainerWait
    statusCh, errCh := cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
    
    // Create a ticker for periodic status updates
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    // Use a select loop to handle multiple channels
    for {
        select {
        case err := <-errCh:
            if err != nil {
                return -1, fmt.Errorf("error waiting for container: %v", err)
            }
            return -1, fmt.Errorf("container wait channel closed unexpectedly")
            
        case status := <-statusCh:
            fmt.Printf("✅ Container %s completed with exit code: %d\n", containerID[:12], status.StatusCode)
            return status.StatusCode, nil
            
        case <-ticker.C:
            // Log periodic status updates
            fmt.Printf("⏳ Container %s still running...\n", containerID[:12])
            
        case <-ctx.Done():
            fmt.Printf("⏰ Container %s timed out after %v\n", containerID[:12], timeout)
            return -1, fmt.Errorf("container execution timed out after %v", timeout)
        }
    }
}

// Read results from shared volume
func (q *JobQueue) readResultsFromSharedVolume(jobWorkspace string) *JobResult {

    fmt.Printf("📖 Reading results from shared volume at %s...\n", jobWorkspace)
    
    // Construct the results file path
    resultsFile := filepath.Join(jobWorkspace, "results", "output.json")
    
    // Check if results file exists
    if _, err := os.Stat(resultsFile); os.IsNotExist(err) {
        return &JobResult{Error: "No output.json found in results directory"}
    }
    
    // Read results file
    resultData, err := os.ReadFile(resultsFile)
    if err != nil {
        return &JobResult{Error: fmt.Sprintf("Failed to read results file: %v", err)}
    }
    
    // Parse the JSON result
    var result JobResult
    err = json.Unmarshal(resultData, &result)
    if err != nil {
        return &JobResult{Error: fmt.Sprintf("Invalid results JSON: %s", string(resultData))}
    }
    
    // Validate score
    if result.Error != "" {
        return &result
    }
    
    fmt.Printf("✅ Container grading complete: Score %.1f\n", result.Score)
    return &result
}

// Create the environment variable slice for containers
func buildEnvironmentVariables(jobID string, config *AssignmentConfig) []string {
    env := []string{
        "BYTEGRADER_VOLUME_MODE=true",
        fmt.Sprintf("BYTEGRADER_JOB_ID=%s", jobID),
    }
    
    // Add assignment-specific environment variables
    for key, value := range config.Environment {
        env = append(env, fmt.Sprintf("%s=%s", key, value))
    }
    
    return env
}