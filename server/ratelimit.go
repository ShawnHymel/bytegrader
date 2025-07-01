package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "time"

    "golang.org/x/time/rate"
)

// Rate limiter storage
type RateLimitManager struct {
    limiters map[string]*rate.Limiter
    mutex    sync.RWMutex
}

// Initialize rate limit manager
func newRateLimitManager() *RateLimitManager {
    return &RateLimitManager{
        limiters: make(map[string]*rate.Limiter),
    }
}

// Get or create rate limiter for IP
func (rlm *RateLimitManager) getLimiter(ip, username string) *rate.Limiter {
    rlm.mutex.Lock()
    defer rlm.mutex.Unlock()

    // Create composite key from IP and username
    key := fmt.Sprintf("%s:%s", ip, username)
    
    limiter, exists := rlm.limiters[key]
    if !exists {
        // Create new limiter with burst = maxRequests and refill rate
        // Rate: requests per window converted to requests per second
        requestsPerSecond := float64(config.RateLimitRequests) / config.RateLimitWindow.Seconds()
        limiter = rate.NewLimiter(rate.Limit(requestsPerSecond), config.RateLimitRequests)
        rlm.limiters[key] = limiter

        fmt.Printf("🚦 Created rate limiter for IP %s, user %s: %.4f req/sec, burst %d\n",
            ip, username, requestsPerSecond, config.RateLimitRequests)
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
        username := getUsername(r)
        limiter := rateLimitManager.getLimiter(clientIP, username)
        
        // Show if rate limit exceeded for IP address
        if !limiter.Allow() {
            fmt.Printf("❌ Rate limit exceeded for IP: %s, user: %s\n", clientIP, username)
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
