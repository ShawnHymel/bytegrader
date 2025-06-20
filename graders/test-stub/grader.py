#!/usr/bin/env python3
"""
Autograder Script - Simple Stub Implementation
This script receives a student submission and returns a dummy score if the file exists.

Usage: python3 test-stub.py <submission_path> <original_filename> <work_dir>
"""

import sys
import os
import json
import time
import zipfile

import magic

# Supported file types for grading (handled by safe_extract)
ALLOWED_FILE_TYPES = [
    'application/zip',
    'application/x-zip-compressed', 
    'application/zip-compressed',
]

# Max unzip size in bytes
MAX_UNZIP_SIZE = 10 * 1024 * 1024  # 10 MB

def validate_file_type(file_path, allowed_types=None):
    """Check if the file exists and is of an allowed type"""

    # Check if the file exists
    if not os.path.exists(file_path):
        return False

    # Check file type against allowed types
    file_type = magic.from_file(file_path, mime=True)
    if file_type in allowed_types or allowed_types is None:
        return True
    else:
        return False

def safe_extract(zip_path, extract_to, max_size, allowed_file_types=None):
    """Safely extract a zip file, checking for zip bombs and path traversal"""
    total_size = 0

    # Validate the file type
    if not validate_file_type(zip_path, allowed_file_types):
        raise ValueError(f"Invalid file type for zip: {zip_path}")
    
    # Check if the zip file exists (and is a valid zip file)
    if not zipfile.is_zipfile(zip_path):
        raise ValueError(f"Invalid zip file: {zip_path}")

    # Check if the extract path is a directory
    if not os.path.isdir(extract_to):
        raise ValueError(f"Extract path is not a directory: {extract_to}")

    # Unzip and validate contents
    with zipfile.ZipFile(zip_path, 'r') as zip_ref:
        for member in zip_ref.infolist():

            # Check for path traversal
            if os.path.isabs(member.filename) or ".." in member.filename:
                raise ValueError(f"Unsafe path: {member.filename}")
            
            # Check compression ratio (zip bomb detection)
            if member.compress_size > 0:
                ratio = member.file_size / member.compress_size
                if ratio > 100:  # Suspicious compression ratio
                    raise ValueError(f"Suspicious compression ratio: {ratio}")
            
            # Check total extracted size
            total_size += member.file_size
            if total_size > max_size:
                raise ValueError(f"Archive too large when extracted: {total_size}")
        
        # If all checks pass, extract the files
        zip_ref.extractall(extract_to)

def main():
    """
    Main function to handle grading logic.
    This function checks the number of arguments to determine the mode of operation:
    - Volume mode: No arguments, uses a default submission path.
    - Standalone mode: Requires 3 arguments for submission path, original filename, and work directory.
    
    It extracts the submission, performs dummy grading, and returns a JSON result.
    """
    
    # Volume mode - no arguments needed
    if len(sys.argv) == 1:
        submission_path = '/workspace/submission/submission.zip'
        original_filename = 'submission.zip'
        work_dir = '/workspace/extracted'
        results_dir = '/workspace/results'
        print("🐳 Running in VOLUME mode", file=sys.stderr)
        
    # Standalone mode - 3 arguments required
    elif len(sys.argv) == 4:
        # Standalone mode - 3 arguments required
        submission_path = sys.argv[1]
        original_filename = sys.argv[2] 
        work_dir = sys.argv[3]
        results_dir = None  # Write to current directory in standalone mode
        print("🖥️  Running in STANDALONE mode", file=sys.stderr)
    
    # Unsupported mode - invalid arguments
    else:
        result = {
            "error": "Invalid arguments. Use either:\n" +
                    "  Volume mode: python3 grader.py (no arguments)\n" +
                    "  Standalone mode: python3 grader.py <submission_path> <original_filename> <work_dir>"
        }
        print(json.dumps(result), file=sys.stderr)
        sys.exit(1)
    
    # Ensure results directory exists (volume mode)
    if results_dir:
        os.makedirs(results_dir, exist_ok=True)
    
    # Initialize grading result
    result = {
        "score": 0.0,
        "max_score": 100.0,
        "feedback": "",
        "error": ""
    }

    # Unzip the submission
    try:
        if submission_path.endswith('.zip'):
            print(f"📦 Extracting {submission_path}...", file=sys.stderr)
            safe_extract(submission_path, work_dir, MAX_UNZIP_SIZE, ALLOWED_FILE_TYPES)
            print(f"✅ Extraction complete.", file=sys.stderr)
        else:
            raise ValueError("Submission must be a zip file.")
    except Exception as e:
        result["error"] = f"Extraction failed: {str(e)}"
        print(f"❌ Extraction error: {e}", file=sys.stderr)
        print(json.dumps(result, indent=2))
        sys.exit(1)

    # Simulate some grading time
    print(f"🔬 Grading {original_filename}...", file=sys.stderr)
    time.sleep(1)
    
    # Return dummy score and feedback
    result["score"] = 85.0
    result["feedback"] += "✅ Dummy grading complete - file processed successfully!"
    
    # Log the result
    print(f"✅ Grading complete. Score: {result['score']}/{result['max_score']}", file=sys.stderr)
    
    # Write results to mounted output directory (for container integration)
    try:
        with open('/results/output.json', 'w') as f:
            json.dump(result, f, indent=2)
    except:
        # If /results doesn't exist, we're probably running locally
        pass
    
    # Output JSON result to stdout (for direct testing and Go app parsing)
    print(json.dumps(result, indent=2))

if __name__ == "__main__":
    main()
