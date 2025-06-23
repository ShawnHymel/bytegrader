FROM python:3.12-slim

# Settings
ARG GRADER_DIR=make-c-add
ARG PYTHON_MAGIC_VERSION=0.4.27

# Runtime environment variables
ENV GRADERS_PATH=/graders
ENV ASSIGNMENT_PATH=/assignment

# Install C build tools and required packages
RUN apt-get update && apt-get install -y \
    gcc \
    make \
    curl \
    unzip \
    libmagic1 \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies
RUN pip install --no-cache-dir \
    python-magic==${PYTHON_MAGIC_VERSION}

# Create directories
RUN mkdir -p /workspace /submission /results ${GRADERS_PATH} ${ASSIGNMENT_PATH}

# Copy framework files
COPY main.py result.py ${GRADERS_PATH}/

# Copy the grader script(s)
COPY ${GRADER_DIR}/*.py ${ASSIGNMENT_PATH}/

# Set Python path
ENV PYTHONPATH="${GRADERS_PATH}"

# Create non-root user for security
RUN useradd -m grader && \
    chown -R grader:grader /workspace /submission /results ${GRADERS_PATH} ${ASSIGNMENT_PATH}

USER grader
WORKDIR /workspace

# Use the framework entrypoint
ENTRYPOINT ["python3", "/graders/main.py", "/submission/submission.zip", "/workspace", "/results"]
