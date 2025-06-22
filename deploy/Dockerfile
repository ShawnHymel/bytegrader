# This is a multi-stage Dockerfile for building a Go addition API app.
# Stage 1: Build the Go binary
# Stage 2: Create a minimal runtime image

# --- Stage 1: Build the Go binary ---

# Use a lightweight Go image for building the application
FROM golang:1.24-alpine AS builder
WORKDIR /app

# Copy Go module files
COPY server/go.mod server/go.sum ./
RUN go mod download

# Copy source code and version
COPY server/*.go ./
COPY VERSION ./

# Accept git commit as build argument
ARG GIT_COMMIT=unknown

# Build with version, time, and commit info
RUN VERSION=$(cat VERSION 2>/dev/null || echo "unknown") && \
    BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) && \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME -X main.GitCommit=$GIT_COMMIT" \
    -a -installsuffix cgo -o bytegrader-api .

# --- Stage 2: Create a minimal runtime image ---

# Runtime image
FROM python:3.12-slim

# Install system dependencies
RUN apt-get update && apt-get install -y \
        ca-certificates \
        libmagic1 \
        curl \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies for graders
RUN pip install --no-cache-dir python-magic

# Create app user with host-matching IDs
ARG USER_ID=1000
ARG GROUP_ID=988

# Create groups if they don't exist, then create user
RUN groupadd -f -g 999 docker \
    && groupadd -f -g ${GROUP_ID} bytegrader \
    && useradd -u ${USER_ID} -g bytegrader -G docker -m -s /bin/sh bytegrader

# Create directories with correct ownership
RUN mkdir -p /app /tmp/uploads /usr/local/bin/graders /workspace/jobs \
    && chown -R bytegrader:bytegrader /app /tmp/uploads /workspace \
    && chmod -R 755 /workspace

# Copy the Go binary from builder stage
COPY --from=builder /app/bytegrader-api /app/

# Copy grading scripts (will be mounted as volume in production)
COPY --chown=bytegrader:bytegrader graders/ /usr/local/bin/graders/

# Make graders executable
RUN chmod +x /usr/local/bin/graders/*.py 2>/dev/null || true

# Switch to non-root user
USER bytegrader
WORKDIR /app

EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

CMD ["./bytegrader-api"]