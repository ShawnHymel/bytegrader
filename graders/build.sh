#!/bin/bash
# Build all enabled grader images from registry

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "🔨 Building grader images from registry..."

if [ ! -f "registry.yaml" ]; then
    echo "❌ registry.yaml not found"
    exit 1
fi

# Simple approach: check each directory and see if it's enabled in registry
for dir in */; do
    if [ -d "$dir" ]; then
        assignment=$(basename "$dir")
        echo ""
        echo "🔍 Checking directory: $assignment"
        
        # Check if this assignment exists in registry and is enabled
        if grep -A10 "^  $assignment:" registry.yaml | grep -q "enabled: true"; then
            # Get image name - simplified extraction
            image_line=$(grep -A10 "^  $assignment:" registry.yaml | grep "image:")
            # Remove everything up to 'image:', then remove quotes and whitespace
            image_name=$(echo "$image_line" | sed 's/.*image:[[:space:]]*//' | sed 's/[\"'\'']//g' | xargs)
            
            echo "📦 Image name: '$image_name'"
            
            if [ -n "$image_name" ]; then
                echo "✅ Building $assignment -> $image_name"
                
                docker build -t "$image_name" -f "$assignment/Dockerfile" .
                echo "✅ Built $image_name successfully"
            else
                echo "❌ Could not extract image name for $assignment"
            fi
        else
            echo "⏭️  Skipping $assignment (not enabled in registry)"
        fi
    fi
done

echo ""
echo "Done building grader images from registry!"
