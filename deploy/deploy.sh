#!/bin/bash
# deploy/deploy.sh - Deploy ByteGrader application
# Run this from the deploy/ directory in the repository

set -e

echo "🚀 Deploying ByteGrader application..."

# Check that app directory argument is provided
if [ $# -ne 1 ]; then
    echo "Usage: $0 <app_dir>"
    echo ""
    echo "Examples:"
    echo "  $0 ~/app"
    echo "  $0 /home/bytegrader/app"
    echo "  $0 /tmp/test-deployment"
    echo ""
    echo "The app directory will be created if it doesn't exist."
    exit 1
fi

APP_DIR="$1"

# Auto-detect repository directory from script location
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "📂 Repository directory: $REPO_DIR (auto-detected)"
echo "📂 App directory: $APP_DIR (specified)"

# Load environment variables
if [ -f ~/.bytegrader_env ]; then
    source ~/.bytegrader_env
    echo "✅ Loaded environment variables"
    echo "📋 Course: $BYTEGRADER_COURSE_DOMAIN"
else
    echo "❌ Environment file not found: ~/.bytegrader_env"
    echo "💡 Run server setup first: sudo ./deploy/setup-server.sh <domain> <subdomain> <email> [security_level]"
    exit 1
fi

# Validate repository directory
if [ ! -f "$REPO_DIR/server/main.go" ]; then
    echo "❌ Error: main.go not found in auto-detected repository: $REPO_DIR/server/main.go"
    echo "💡 Make sure you're running this script from the ByteGrader repository"
    echo "💡 Script location: $SCRIPT_DIR"
    exit 1
fi
if [ ! -f "$REPO_DIR/deploy/docker-compose.yaml" ]; then
    echo "❌ Error: docker-compose.yaml not found: $REPO_DIR/deploy/docker-compose.yaml"
    echo "💡 Make sure you're using a complete ByteGrader repository"
    exit 1
fi

# Ensure app directory exists
mkdir -p "$APP_DIR"
cd "$APP_DIR"

echo "📂 Copying source files from repository..."

# Copy source files using absolute paths
mkdir -p server
cp "$REPO_DIR/server"/*.go server/
cp "$REPO_DIR/server/go.mod" server/
cp "$REPO_DIR/server/go.sum" server/
cp "$REPO_DIR/VERSION" .

# Calculate git info
GIT_COMMIT=$(cd "$REPO_DIR" && git rev-parse --short HEAD 2>/dev/null || echo "unknown")
echo "📦 Using Git commit: $GIT_COMMIT"

# Get current user and docker group IDs for container permissions
if [ -n "$SUDO_USER" ]; then
    # Script was run with sudo, get the real user's ID
    export DOCKER_USER_ID=$(id -u "$SUDO_USER")
else
    # Script was run directly
    export DOCKER_USER_ID=$(id -u)
fi
export DOCKER_GROUP_ID=$(getent group docker | cut -d: -f3)

echo "🔧 Using User ID: $DOCKER_USER_ID ($(id -un $DOCKER_USER_ID 2>/dev/null || echo "unknown"))"
echo "🔧 Using Docker Group ID: $DOCKER_GROUP_ID"

echo "📂 Copying and configuring deployment files..."

# Set variables for envsubst (it expects different names than our env file)
export COURSE_SUBDOMAIN="$BYTEGRADER_COURSE_SUBDOMAIN"

# Export security settings for docker-compose
export BYTEGRADER_REQUIRE_API_KEY
export BYTEGRADER_VALID_API_KEYS
export BYTEGRADER_ALLOWED_IPS
export BYTEGRADER_MAX_FILE_SIZE_MB
export BYTEGRADER_GRADING_TIMEOUT_MIN
export BYTEGRADER_MAX_CONCURRENT_JOBS
export BYTEGRADER_RATE_LIMIT_ENABLED
export BYTEGRADER_RATE_LIMIT_REQUESTS
export BYTEGRADER_RATE_LIMIT_WINDOW_MIN
export BYTEGRADER_CLEANUP_INTERVAL_HOURS
export BYTEGRADER_COMPLETED_JOB_TTL_HOURS
export BYTEGRADER_FAILED_JOB_TTL_HOURS
export BYTEGRADER_OLD_FILE_TTL_HOURS
export DOCKER_USER_ID
export DOCKER_GROUP_ID

# Build with correct user/group IDs
echo "🔧 Building with User ID: $DOCKER_USER_ID, Docker Group ID: $DOCKER_GROUP_ID"

# Copy and process docker-compose.yaml with environment variables
envsubst < "$REPO_DIR/deploy/docker-compose.yaml" > docker-compose.yaml

# Copy Dockerfile
cp "$REPO_DIR/deploy/Dockerfile" .

# Create other necessary directories
mkdir -p uploads logs workspace

# Set correct permissions for workspace
chown -R "$DOCKER_USER_ID:$DOCKER_GROUP_ID" workspace || true

# Build grader images
echo "🔨 Building grader images..."
if [ -d "$REPO_DIR/graders" ]; then

    # Copy graders directory
    cp -r "$REPO_DIR/graders" .
    echo "✅ Copied graders directory"
    
    # Build grader images from registry
    cd graders
    if [ -f "build.sh" ]; then
        chmod +x build.sh
        ./build.sh
        echo "✅ Built grader images"
    else
        echo "⚠️ build.sh not found, skipping grader builds"
    fi
    cd ..
else
    echo "⚠️  No graders directory found"
fi

echo "🐳 Building Docker image..."

# Stop any existing containers
docker compose down 2>/dev/null || true

# Build with no cache to ensure latest code
docker compose build --no-cache \
  --build-arg USER_ID="$DOCKER_USER_ID" \
  --build-arg GROUP_ID="$DOCKER_GROUP_ID" \
  --build-arg DOCKER_GID="$DOCKER_GROUP_ID" \
  --build-arg GIT_COMMIT="$GIT_COMMIT"

# Fix volume ownership to match container user
echo "🔧 Fixing volume permissions..."
docker volume create bytegrader-workspace 2>/dev/null || true
docker run --rm -v bytegrader-workspace:/workspace alpine sh -c "
  chown -R $DOCKER_USER_ID:$DOCKER_GROUP_ID /workspace &&
  chmod -R 755 /workspace &&
  echo 'Volume ownership fixed to $DOCKER_USER_ID:$DOCKER_GROUP_ID'
"

# Copy health check script
echo "📋 Copying health check script..."
cp "$REPO_DIR/deploy/health-check.sh" .
chmod +x health-check.sh

# Start the services
docker compose up -d

echo "⏳ Waiting for service to start..."
sleep 15

# Check if the service is running
if docker compose ps | grep -q "Up"; then
    echo "✅ ByteGrader is running!"
    echo ""
    
    # Display security configuration summary
    echo "🔐 Security Configuration:"
    if [ "$BYTEGRADER_REQUIRE_API_KEY" = "true" ]; then
        echo "   🔑 API Key: Required"
    else
        echo "   🔓 API Key: Not required"
    fi
    
    if [ -n "$BYTEGRADER_ALLOWED_IPS" ]; then
        echo "   🛡️  IP Whitelist: $BYTEGRADER_ALLOWED_IPS"
    else
        echo "   🌐 IP Access: Open (no restrictions)"
    fi
    echo ""
    
    echo "🧪 Test locally:"
    echo "   curl http://localhost:8080/health"
    
    # Show appropriate test commands based on security level
    if [ "$BYTEGRADER_REQUIRE_API_KEY" = "true" ]; then
        FIRST_API_KEY="$BYTEGRADER_VALID_API_KEYS"
        echo "   curl -H \"X-API-Key: $FIRST_API_KEY\" http://localhost:8080/queue"
    else
        echo "   curl http://localhost:8080/queue"
    fi
    echo ""
    
    echo "🌐 Test via domain (after DNS propagates):"
    echo "   curl http://$BYTEGRADER_COURSE_DOMAIN/health"
    
    if [ "$BYTEGRADER_REQUIRE_API_KEY" = "true" ]; then
        echo "   curl -H \"X-API-Key: $FIRST_API_KEY\" https://$BYTEGRADER_COURSE_DOMAIN/queue"
    else
        echo "   curl https://$BYTEGRADER_COURSE_DOMAIN/queue"
    fi
    echo ""
    
    if [ "$BYTEGRADER_REQUIRE_API_KEY" = "true" ]; then
        echo "🔑 API Key:"
        echo "   $BYTEGRADER_VALID_API_KEYS"
        echo ""
        echo "📝 API Usage Examples:"
        echo "   Header: X-API-Key: $FIRST_API_KEY"
        echo "   Bearer: Authorization: Bearer $FIRST_API_KEY"
        echo ""
    fi
    
    echo "📊 Monitor:"
    echo "   cd $APP_DIR"
    echo "   docker compose ps           # Service status"
    echo "   docker compose logs -f      # View logs"
    echo ""
    echo "🔧 Manage:"
    echo "   cd $APP_DIR"
    echo "   docker compose restart      # Restart services"
    echo "   docker compose down         # Stop services"
    echo ""
    
    if [ -z "$(docker compose ps -q)" ]; then
        echo "📋 Next steps:"
        echo "1. Test DNS: nslookup $BYTEGRADER_COURSE_DOMAIN"
        echo "2. Enable HTTPS: sudo /root/setup_ssl.sh"
        echo "3. Test HTTPS: curl https://$BYTEGRADER_COURSE_DOMAIN/health"
    fi
    echo ""
    echo "📋 View logs:"
    echo "   docker compose logs -f"
    echo ""
    echo "🔄 To restart:"
    echo "   docker compose restart"
    echo ""
    echo "🛑 To stop:"
    echo "   docker compose down"
else
    echo "❌ Failed to start ByteGrader"
    echo "📋 Check logs:"
    docker compose logs
    echo ""
    echo "🔍 Debug steps:"
    echo "1. Check if port 8080 is available: netstat -tulpn | grep :8080"
    echo "2. Check Docker status: docker ps -a"
    echo "3. Check system resources: df -h && free -h"
    echo "4. Check environment variables: cat ~/.bytegrader_env"
    exit 1
fi