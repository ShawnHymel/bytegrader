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
RUN mkdir -p /app/config /app/modules /app/submission
WORKDIR /app

# Copy in autograder scripts
COPY scripts/autograder/*.py /app/
COPY scripts/autograder/config/ /app/config/
COPY scripts/autograder/modules/ /app/modules/

# Create virtual environment for autograder
RUN python3 -m venv /app/venv
ENV PATH="/app/venv/bin:$PATH"
ENV VIRTUAL_ENV="/app/venv"

# Install autograder dependencies
COPY scripts/autograder/requirements.txt /app/requirements.txt
RUN python3 -m pip install --upgrade pip && \
    python3 -m pip install --no-cache-dir -r /app/requirements.txt

# Make the autograder scripts executable
RUN chmod +x /app/autograder.py

#-------------------------------------------------------------------------------
# Entrypoint

# Export environment variables to be system-wide
RUN echo "IDF_TOOLS_PATH=${IDF_TOOLS_PATH}" >> /etc/environment && \
    echo "IDF_VERSION=${IDF_VERSION}" >> /etc/environment

# Add alias to bashrc and enable environment on terminal open
RUN echo "alias get_idf='. /opt/toolchains/esp-idf/export.sh'" >> /root/.bashrc && \
    echo "get_idf" >> /root/.bashrc

# Set the entrypoint to our Python script
ENTRYPOINT ["python3", "/app/autograder.py", "--course", "/app/esp32_iot_course.json", "--debug"]
