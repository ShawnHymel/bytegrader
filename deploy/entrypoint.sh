#!/bin/sh
set -e

# Set up workspace with shared group ownership and setgid bit.
# The setgid bit ensures new subdirectories inherit the 'graders' group,
# so grader containers (whose users are also in the 'graders' group)
# can write to results/ directories created by the API server.
mkdir -p /workspace/jobs
chgrp graders /workspace /workspace/jobs 2>/dev/null || true
chmod 2775 /workspace /workspace/jobs

# Set umask so new files/dirs are group-writable (0775 dirs, 0664 files)
umask 002

# Drop to app user and exec the command
exec gosu bytegrader "$@"
