#!/bin/bash

echo "=== ByteGrader Health Check ==="
echo "Time: $(date)"
echo ""

echo "=== Service Status ==="
cd /home/bytegrader/app && docker compose ps
echo ""

echo "=== System Resources ==="
echo "Memory Usage:"
free -h
echo ""
echo "CPU Usage:"
top -bn1 | grep "Cpu(s)" | awk '{print $1 $2}' | sed 's/%Cpu(s):/CPU: /'
echo ""

echo "=== Container Resources ==="
docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}" | head -10
echo ""

echo "=== Disk Usage ==="
df -h | grep -E "(Filesystem|/dev/)"
echo ""

echo "=== Workspace Usage ==="
# Try to check from inside a container instead
docker run --rm -v bytegrader-workspace:/workspace alpine du -sh /workspace 2>/dev/null || echo "Unable to check workspace (run as root to check Docker volumes)"
echo ""

echo "=== Recent Errors ==="
cd /home/bytegrader/app && docker compose logs --since 30m | grep -i error | tail -5
echo ""
