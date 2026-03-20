package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "time"

    "github.com/docker/docker/api/types/container"
    "github.com/docker/docker/api/types/mount"
    "github.com/docker/docker/client"
)

// Get UID and GID from image configuration
func getImageUserIDs(ctx context.Context, cli *client.Client, imageName string) (int, int, error) {

    // Inspect the image to get the user
    imageInfo, err := cli.ImageInspect(ctx, imageName)
    if err != nil {
        return 0, 0, fmt.Errorf("failed to inspect image: %v", err)
    }

    imageUser := imageInfo.Config.User
    fmt.Printf("🔍 Image user: '%s'\n", imageUser)

    // If no user specified, default to root (0:0)
    if imageUser == "" {
        fmt.Printf("⚠️  No USER specified in image, using root (0:0)\n")
        return 0, 0, nil
    }

    // Parse "uid:gid" format (e.g. "1000:1000")
    if strings.Contains(imageUser, ":") {
        parts := strings.Split(imageUser, ":")
        uid, err := strconv.Atoi(parts[0])
        if err != nil {
            return 0, 0, fmt.Errorf("failed to parse UID from '%s': %v", imageUser, err)
        }
        gid, err := strconv.Atoi(parts[1])
        if err != nil {
            return 0, 0, fmt.Errorf("failed to parse GID from '%s': %v", imageUser, err)
        }
        return uid, gid, nil
    }

    // If just a username (e.g. "grader"), run a quick container to resolve UID/GID
    fmt.Printf("🔍 Resolving UID/GID for user '%s'...\n", imageUser)
    result, err := cli.ContainerCreate(ctx,
        &container.Config{
            Image: imageName,
            Cmd:   []string{"/bin/sh", "-c", "id -u && id -g"},
            User:  imageUser,
        },
        nil, nil, nil, "",
    )
    if err != nil {
        return 0, 0, fmt.Errorf("failed to create id lookup container: %v", err)
    }
    defer cli.ContainerRemove(ctx, result.ID, container.RemoveOptions{Force: true})

    if err := cli.ContainerStart(ctx, result.ID, container.StartOptions{}); err != nil {
        return 0, 0, fmt.Errorf("failed to start id lookup container: %v", err)
    }

    statusCh, errCh := cli.ContainerWait(ctx, result.ID, container.WaitConditionNotRunning)
    select {
    case err := <-errCh:
        return 0, 0, fmt.Errorf("error waiting for id lookup container: %v", err)
    case <-statusCh:
    }

    logs, err := cli.ContainerLogs(ctx, result.ID, container.LogsOptions{ShowStdout: true})
    if err != nil {
        return 0, 0, fmt.Errorf("failed to get id lookup container logs: %v", err)
    }
    defer logs.Close()

    logData, err := io.ReadAll(logs)
    if err != nil {
        return 0, 0, fmt.Errorf("failed to read id lookup logs: %v", err)
    }

    // Parse UID and GID from output
    // Docker log lines have an 8-byte header, strip them
    lines := strings.Split(strings.TrimSpace(string(logData)), "\n")
    if len(lines) < 2 {
        return 0, 0, fmt.Errorf("unexpected id output: %s", string(logData))
    }

    // Strip 8-byte Docker log header from each line
    uidStr := strings.TrimSpace(lines[0])
    gidStr := strings.TrimSpace(lines[1])
    if len(uidStr) > 8 {
        uidStr = strings.TrimSpace(uidStr[8:])
    }
    if len(gidStr) > 8 {
        gidStr = strings.TrimSpace(gidStr[8:])
    }

    uid, err := strconv.Atoi(uidStr)
    if err != nil {
        return 0, 0, fmt.Errorf("failed to parse UID '%s': %v", uidStr, err)
    }
    gid, err := strconv.Atoi(gidStr)
    if err != nil {
        return 0, 0, fmt.Errorf("failed to parse GID '%s': %v", gidStr, err)
    }

    fmt.Printf("✅ Resolved user '%s' to %d:%d\n", imageUser, uid, gid)
    return uid, gid, nil
}

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

    // Get UID/GID from image and fix workspace ownership
    uid, gid, err := getImageUserIDs(ctx, cli, assignmentConfig.Image)
    if err != nil {
        return &JobResult{Error: fmt.Sprintf("Failed to get image user IDs: %v", err)}
    }
    fmt.Printf("👤 Setting workspace ownership to %d:%d\n", uid, gid)

    // Fix ownership of job workspace directories
    submissionDir := filepath.Join(jobWorkspace, "submission")
    resultsDir := filepath.Join(jobWorkspace, "results")
    submissionZip := filepath.Join(submissionDir, "submission.zip")

    for _, path := range []string{jobWorkspace, submissionDir, resultsDir, submissionZip} {
        if err := os.Chown(path, uid, gid); err != nil {
            fmt.Printf("⚠️  Failed to chown %s: %v\n", path, err)
        }
    }
    
    // Create grader container with volume mount and environment detection
    resp, err := cli.ContainerCreate(
        ctx, 
        &container.Config{
            Image: assignmentConfig.Image,
            WorkingDir: "/workspace",  // Simplified working directory
            Env: buildEnvironmentVariables(job.ID, assignmentConfig),
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