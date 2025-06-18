#!/bin/bash
# deploy/deploy.sh - Deploy ByteGrader application
# Run this from the deploy/ directory in the repository

set -e

echo "🚀 Deploying ByteGrader application..."

# Determine where we're running from and set up paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ -f "$SCRIPT_DIR/../main.go" ]; then
    # Running from deploy/ directory in repo
    REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
    APP_DIR="$(cd "$SCRIPT_DIR/../../app" && pwd)"
    echo "📁 Running from repository deploy/ directory"
elif [ -f "$SCRIPT_DIR/../bytegrader/main.go" ]; then
    # Running from app/ directory (legacy)
    REPO_DIR="$(cd "$SCRIPT_DIR/../bytegrader" && pwd)"
    APP_DIR="$(cd "$SCRIPT_DIR" && pwd)"
    echo "📁 Running from app/ directory"
else
    echo "❌ Error: Cannot find main.go"
    echo "💡 Current directory: $(pwd)"
    echo "💡 Script directory: $SCRIPT_DIR"
    echo "💡 Looking for main.go at:"
    echo "   - $SCRIPT_DIR/../main.go"
    echo "   - $SCRIPT_DIR/../bytegrader/main.go"
    exit 1
fi

echo "📂 Repository directory: $REPO_DIR"
echo "📂 App directory: $APP_DIR"

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
