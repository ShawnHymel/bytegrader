package main

import (
    "encoding/json"
	"fmt"
    "net"
    "net/http"
    "strings"
)

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

// Extract username from request headers
func getUsername(r *http.Request) string {
    username := r.Header.Get("X-Username")
    return username
}

// Validate that username is present in request
func validateUsername(r *http.Request) bool {
    username := getUsername(r)
    return username != ""
}

// Apply security checks for admin endpoints (API key + IP, but no username required)
func adminSecurityMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        clientIP := getClientIP(r)
        
        // Set CORS headers for browser compatibility
        setCORSHeaders(w)
        
        // Handle preflight requests
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        
        // Check IP whitelist (primary security)
        if !validateSourceIP(r) {
            fmt.Printf("❌ IP validation failed for admin endpoint %s %s from %s\n", r.Method, r.URL.Path, clientIP)
            w.WriteHeader(http.StatusForbidden)
            json.NewEncoder(w).Encode(ErrorResponse{Error: "IP address not allowed"})
            return
        }
        
        // Check API key (authentication)
        if !authenticateRequest(r) {
            fmt.Printf("❌ Authentication failed for admin endpoint %s %s\n", r.Method, r.URL.Path)
            w.WriteHeader(http.StatusUnauthorized)
            json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid or missing API key"})
            return
        }
        
        // Log successful security checks (no username required for admin endpoints)
        fmt.Printf("✅ Admin security checks passed for %s %s from %s\n", r.Method, r.URL.Path, clientIP)
        
        // All security checks passed, proceed to handler
        next(w, r)
    }
}

// Admin endpoint wrapper (API key required, but no username or rate limiting)
func adminEndpoint(handler http.HandlerFunc) http.HandlerFunc {
    return adminSecurityMiddleware(handler)
}

// Apply security checks to requests
func securityMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        clientIP := getClientIP(r)
        
        // Set CORS headers for browser compatibility
        setCORSHeaders(w)
        
        // Handle preflight requests
        if r.Method == "OPTIONS" {
            w.WriteHeader(http.StatusOK)
            return
        }
        
        // Check IP whitelist (primary security)
        if !validateSourceIP(r) {
            fmt.Printf("❌ IP validation failed for %s %s from %s\n", r.Method, r.URL.Path, clientIP)
            w.WriteHeader(http.StatusForbidden)
            json.NewEncoder(w).Encode(ErrorResponse{Error: "IP address not allowed"})
            return
        }
        
        // Check API key (authentication)
        if !authenticateRequest(r) {
            fmt.Printf("❌ Authentication failed for %s %s\n", r.Method, r.URL.Path)
            w.WriteHeader(http.StatusUnauthorized)
            json.NewEncoder(w).Encode(ErrorResponse{Error: "Invalid or missing API key"})
            return
        }

        // Check username (required for rate limiting)
        if !validateUsername(r) {
            fmt.Printf("❌ Username validation failed for %s %s from %s\n", r.Method, r.URL.Path, clientIP)
            w.WriteHeader(http.StatusBadRequest)
            json.NewEncoder(w).Encode(ErrorResponse{Error: "Username required (X-Username header)"})
            return
        }
        
        // Log successful security checks
        username := getUsername(r)
        fmt.Printf("✅ All security checks passed for %s %s from %s (user: %s)\n", r.Method, r.URL.Path, clientIP, username)
        
        // All security checks passed, proceed to handler
        next(w, r)
    }
}

// Combined middleware that applies rate limiting and security
func protectedEndpoint(handler http.HandlerFunc) http.HandlerFunc {
    return securityMiddleware(rateLimitMiddleware(handler))
}
