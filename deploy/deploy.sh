#!/bin/bash
# deploy/deploy.sh - Deploy ByteGrader application
# Run this as the bytegrader user in the app directory

set -e

echo "🚀 Deploying ByteGrader application..."

# Check if we're in the right directory and have the right files
if [ ! -f "docker-compose.yml" ]; then
    echo "❌ Error: docker-compose.yml not found in current directory"
    echo "💡 Make sure you're in the /home/bytegrader/app directory"
    exit 1
fi

if [ ! -f "../../../main.go" ]; then
    echo "❌ Error: main.go not found in repository root"
    echo "💡 Make sure the ByteGrader repository is cloned in /home/bytegrader"
    echo "📋 Expected structure:"
    echo "   /home/bytegrader/"
    echo "   ├── bytegrader/          # Git repository"
    echo "   │   ├── main.go"
    echo "   │   ├── go.mod"
    echo "   │   └── ..."
    echo "   └── app/                 # Deployment directory"
    echo "       ├── docker-compose.yml"
    echo "       └── deploy.sh"
    exit 1
fi

echo "📂 Copying source files from repository..."
# Copy source files from the repository to the deployment directory
cp ../../../main.go .
cp ../../../go.mod .
cp ../../../go.sum .

# Copy graders directory
mkdir -p graders
if [ -f "../../../test.py" ]; then
    cp ../../../test.py graders/
    echo "✅ Copied test.py to graders/"
fi

# Copy any other grader scripts
cp ../../../graders/*.py graders/ 2>/dev/null || true

# Create other necessary directories
mkdir -p uploads logs

# Make graders executable
chmod +x graders/*.py 2>/dev/null || true

echo "🐳 Building and starting ByteGrader..."
# Stop any existing containers
docker-compose down 2>/dev/null || true

# Build with no cache to ensure latest code
docker-compose build --no-cache

# Start the services
docker-compose up -d

echo "⏳ Waiting for service to start..."
sleep 15

# Check if the service is running
if docker-compose ps | grep -q "Up"; then
    echo "✅ ByteGrader is running!"
    echo ""
    echo "🧪 Test locally:"
    echo "   curl http://localhost:8080/health"
    echo ""
    echo "📊 View status:"
    echo "   docker-compose ps"
    echo ""
    echo "📋 View logs:"
    echo "   docker-compose logs -f"
    echo ""
    echo "🔄 To restart:"
    echo "   docker-compose restart"
    echo ""
    echo "🛑 To stop:"
    echo "   docker-compose down"
else
    echo "❌ Failed to start ByteGrader"
    echo "📋 Check logs:"
    docker-compose logs
    echo ""
    echo "🔍 Debug steps:"
    echo "1. Check if port 8080 is available: netstat -tulpn | grep :8080"
    echo "2. Check Docker status: docker ps -a"
    echo "3. Check system resources: df -h && free -h"
    exit 1
fi