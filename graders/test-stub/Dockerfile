FROM python:3.12-slim

# Install required packages
RUN apt-get update && apt-get install -y \
    curl \
    unzip \
    libmagic1 \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies
RUN pip install --no-cache-dir python-magic

# Create directories
RUN mkdir -p /workspace /submission /results

# Copy the grader script
COPY grader.py /grader.py
RUN chmod +x /grader.py

# Create non-root user for security
RUN useradd -m grader && \
    chown -R grader:grader /workspace /submission /results

USER grader
WORKDIR /workspace

# Use argument-based interface: submission_path, original_filename, work_dir
ENTRYPOINT ["python3", "/grader.py", "./submission/submission.zip", "./extracted"]
