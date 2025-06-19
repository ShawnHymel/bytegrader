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
if [ ! -f "$REPO_DIR/main.go" ]; then
    echo "❌ Error: main.go not found in auto-detected repository: $REPO_DIR/main.go"
    echo "💡 Make sure you're running this script from the ByteGrader repository"
    echo "💡 Script location: $SCRIPT_DIR"
    exit 1
fi
if [ ! -f "$REPO_DIR/deploy/docker-compose.yml" ]; then
    echo "❌ Error: docker-compose.yml not found: $REPO_DIR/deploy/docker-compose.yml"
    echo "💡 Make sure you're using a complete ByteGrader repository"
    exit 1
fi

# Ensure app directory exists
mkdir -p "$APP_DIR"
cd "$APP_DIR"

echo "📂 Copying source files from repository..."
# Copy source files using absolute paths
cp "$REPO_DIR/main.go" .
cp "$REPO_DIR/go.mod" .
cp "$REPO_DIR/go.sum" .

# Get current user and docker group IDs for container permissions
export DOCKER_USER_ID=$(id -u)
export DOCKER_GROUP_ID=$(getent group docker | cut -d: -f3)

echo "🔧 Setting container permissions:"
echo "   User ID: $DOCKER_USER_ID"
echo "   Docker Group ID: $DOCKER_GROUP_ID"

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

# Copy and process docker-compose.yml with environment variables
envsubst < "$REPO_DIR/deploy/docker-compose.yml" > docker-compose.yml

# Copy Dockerfile
cp "$REPO_DIR/deploy/Dockerfile" .

# Copy graders directory
mkdir -p graders
if [ -f "$REPO_DIR/test.py" ]; then
    cp "$REPO_DIR/test.py" graders/
    echo "✅ Copied test.py to graders/"
fi

# Copy any grader scripts from graders directory
if [ -d "$REPO_DIR/graders" ]; then
    cp "$REPO_DIR/graders"/*.py graders/ 2>/dev/null || true
    echo "✅ Copied grader scripts"
fi

# Copy registry file (ADD THIS)
if [ -f "$REPO_DIR/graders/registry.yaml" ]; then
    cp "$REPO_DIR/graders/registry.yaml" graders/
    echo "✅ Copied registry.yaml"
fi

# Create other necessary directories
mkdir -p uploads logs

# Make graders executable
chmod +x graders/*.py 2>/dev/null || true

echo "🐳 Building and starting ByteGrader..."
# Stop any existing containers
docker compose down 2>/dev/null || true

# Build with no cache to ensure latest code
docker compose build --no-cache

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