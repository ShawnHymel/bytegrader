package main

import (
    "encoding/base64"

    "github.com/google/uuid"
)

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
