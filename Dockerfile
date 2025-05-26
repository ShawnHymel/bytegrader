# Settings
ARG DEBIAN_VERSION=stable-20250520-slim

#-------------------------------------------------------------------------------
# Base Image and Dependencies

# Use Debian as the base image
FROM debian:${DEBIAN_VERSION}

# Redeclare arguments after FROM
ARG TARGETARCH

# Set default shell during Docker image build to bash
SHELL ["/bin/bash", "-c"]

# Check if the target architecture is either x86_64 (amd64) or arm64 (aarch64)
RUN if [ "$TARGETARCH" = "amd64" ] || [ "$TARGETARCH" = "arm64" ]; then \
        echo "Architecture $TARGETARCH is supported."; \
    else \
        echo "Unsupported architecture: $TARGETARCH"; \
        exit 1; \
    fi

# Set non-interactive frontend for apt-get to skip any user confirmations
ENV DEBIAN_FRONTEND=noninteractive

# Install base packages
RUN apt-get -y update && \
	apt-get install --no-install-recommends -y \
    dos2unix \
    wget \
    git \
    python3 \
    python3-pip \
    python3-venv \
    cmake \
    build-essential \
    unzip

# Clean up stale packages
RUN apt-get clean -y && \
	apt-get autoremove --purge -y && \
	rm -rf /var/lib/apt/lists/*

#-------------------------------------------------------------------------------
# Autograder setup

# Set up directories for autograder
RUN mkdir -p /app/
WORKDIR /app

# Create virtual environment for autograder
RUN python3 -m venv /app/venv
ENV PATH="/app/venv/bin:$PATH"
ENV VIRTUAL_ENV="/app/venv"

# Install bytegrader dependencies
COPY requirements.txt /app/requirements.txt
RUN python3 -m pip install --upgrade pip && \
    python3 -m pip install --no-cache-dir -r /app/requirements.txt

# Install bytegrader from source
COPY setup.py /app/
COPY src/ /app/src/
RUN python3 -m pip install -e .

#-------------------------------------------------------------------------------
# Entrypoint

# Create a directory for submissions and suites
RUN mkdir -p /app/submissions
RUN mkdir -p /app/suites

# Set the entrypoint to bytegrader
ENTRYPOINT ["python3", "-m", "bytegrader.bytegrader"]
