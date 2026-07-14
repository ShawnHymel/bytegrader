package main

import (
    "fmt"
    "net/http"
    "os"

    "gopkg.in/yaml.v3"
)

// Configuration for each assignment
type AssignmentConfig struct {
    Image           string `yaml:"image"`
    Description     string `yaml:"description"`
    TimeoutMinutes  int    `yaml:"timeout_minutes"`
    Enabled         bool   `yaml:"enabled"`
    Environment     map[string]string `yaml:"environment,omitempty"`
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
