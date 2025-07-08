# ByteGrader API Endpoints

## Public Endpoints

*These endpoints do not require authentication and are not subject to rate limiting.*

- **`GET /health`** - Health check endpoint
  - Always returns `{"status": "ok"}` if server is running
  - Used by load balancers and monitoring systems
  - No authentication required

## Admin Endpoints

*These endpoints require API key and are subject to IP whitelist and rate limiting.*

- **`GET /config`** - View current server configuration
  - Returns server settings like timeouts, file size limits, and security status
  - Useful for debugging and system monitoring
  - Does not expose sensitive information like API keys

- **`GET /version`** - Get version and build information
  - Returns API version, build time, and git commit hash
  - Useful for debugging and deployment verification
  - Example response: `{"version": "1.0.0", "build_time": "2025-01-20T15:30:45Z", "git_commit": "a3b7c9d"}`

## Protected Endpoints

*These endpoints require API key authentication (if enabled), username (arbitrary), and are subject to IP whitelist and rate limiting.*

- **`GET /queue`** - Get overall queue status and statistics
  - Returns queue length, total jobs, active jobs, and worker status
  - Useful for monitoring system load and performance
  - Example response: `{"queue_length": 3, "active_jobs": 1, "total_jobs": 15}`

- **`GET /status/{job_id}`** - Check the status of a grading job
  - Returns job information including status, score, and feedback
  - Status values: "queued", "processing", "completed", "failed"
  - Example: `GET /status/abc123def456`

- **`POST /submit`** - Submit a file for grading
  - Accepts multipart form data with a `file` field (ZIP archive)
  - Requires `assignment` query parameter or `assignment_id` form field
  - Returns job ID for tracking grading status
  - Example: `POST /submit?assignment=test-stub`

## HTTP Methods Supported

- **POST**: `/submit` only
- **GET**: All other endpoints
- **OPTIONS**: All endpoints (for CORS preflight requests)

## Response Format

All endpoints return JSON responses with appropriate HTTP status codes:
- `200` - Success
- `400` - Bad Request (invalid input)
- `401` - Unauthorized (invalid/missing API key)
- `403` - Forbidden (IP not whitelisted)
- `404` - Not Found (invalid job ID)
- `405` - Method Not Allowed
- `429` - Too Many Requests (rate limit exceeded)
- `500` - Internal Server Error