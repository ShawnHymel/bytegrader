FROM python:3.12-slim

# Settings
ARG GRADER_DIR=test-delay
ENV MAX_UNZIP_SIZE_MB=10

# Dependency versions
ARG PYTHON_MAGIC_VERSION=0.4.27

# Install required packages
RUN apt-get update && apt-get install -y \
    curl \
    unzip \
    libmagic1 \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies
RUN pip install --no-cache-dir \
    python-magic==${PYTHON_MAGIC_VERSION}

# Create directories
RUN mkdir -p /workspace /submission /results /grader /assignment

# Copy framework files
COPY main.py result.py /grader/

# Copy the grader script(s)
COPY ${GRADER_DIR}/*.py /assignment/

# Set Python path
ENV PYTHONPATH="/grader"

# Create non-root user for security
RUN useradd -m grader && \
    chown -R grader:grader /workspace /submission /results /grader /assignment

USER grader
WORKDIR /workspace

# Use argument-based interface: submission_path, original_filename, work_dir
ENTRYPOINT ["python3", "/grader/main.py", "/submission/submission.zip", "/workspace", "/results"]
