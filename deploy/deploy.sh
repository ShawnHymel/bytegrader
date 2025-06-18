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

# Copy deployment files (Dockerfile, docker-compose.yml) using absolute paths
cp "$REPO_DIR/deploy/Dockerfile" .
cp "$REPO_DIR/deploy/docker-compose.yml" .

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
    echo "🧪 Test locally:"
    echo "   curl http://localhost:8080/health"
    echo ""
    echo "📊 View status:"
    echo "   docker compose ps"
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
    exit 1
fi
