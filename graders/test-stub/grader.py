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

# Name of the output file
OUTPUT_FILE = 'output.json'

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

def write_results_and_exit(results_dir, result, exit_code=0):
    """Write results to output.json and exit with specified code"""
    try:
        # Write results to working directory for Go app to read
        with open(os.path.join(results_dir, OUTPUT_FILE), 'w') as f:
            json.dump(result, f, indent=2)
        print(f"✅ Results written to output.json", file=sys.stderr)
    except Exception as e:
        print(f"⚠️  Failed to write output.json: {e}", file=sys.stderr)
    
    # Output JSON result to stdout (for direct testing)
    print(json.dumps(result, indent=2))
    sys.exit(exit_code)

def main():
    """
    Main function for the autograder script.
    It extracts the submission, performs dummy grading, and returns a JSON result in output.json.

    This script is designed to be run in a Docker container with mounted volumes, but can also be 
    run locally for testing. It expects two command line arguments: the path to the submission zip 
    file and the working directory. The results are written to a mounted output directory or printed
    to stdout.

    Usage: python3 grader.py <submission_path> <work_dir> <results_dir>
    """
    
    # volume mode (running on server): use environment variables to determine paths
    if os.getenv("BYTEGRADER_VOLUME_MODE") == "true":

        # Server/volume mode: ignore ENTRYPOINT arguments, use working directory
        work_dir = os.getcwd()  # e.g., /workspace/jobs/{jobID}
        submission_path = os.path.join(work_dir, "submission", "submission.zip")
        results_dir = os.path.join(work_dir, "results")

        print(f"📋 Volume mode: using working directory {work_dir}", file=sys.stderr)
        print(f"📋 Volume mode paths:", file=sys.stderr)
        print(f"   Submission: {submission_path}", file=sys.stderr)
        print(f"   Results: {results_dir}", file=sys.stderr)
        print(f"✅ Results directory ensured: {results_dir}", file=sys.stderr)

    # Local testing mode: validate arguments and use provided paths
    else:
        
        # Check if the correct number of arguments is provided
        if len(sys.argv) != 4:
            print("Usage: python3 grader.py <submission_path> <work_dir> <results_dir>", file=sys.stderr)
            sys.exit(1)
        
        # Parse command line arguments
        submission_path = sys.argv[1]
        work_dir = sys.argv[2]
        results_dir = sys.argv[3]

        print(f"📋 Local mode: using provided paths", file=sys.stderr)
        print(f"   Submission: {submission_path}", file=sys.stderr)
        print(f"   Work dir: {work_dir}", file=sys.stderr)
        print(f"   Results: {results_dir}", file=sys.stderr)

    # ***Debug: List what's actually in the working directory
    if os.getenv("BYTEGRADER_VOLUME_MODE") == "true":
        print(f"📂 Contents of {work_dir}:", file=sys.stderr)
        try:
            for item in os.listdir(work_dir):
                item_path = os.path.join(work_dir, item)
                if os.path.isdir(item_path):
                    print(f"   📁 {item}/", file=sys.stderr)
                    # List contents of subdirectories
                    try:
                        for subitem in os.listdir(item_path):
                            print(f"      📄 {subitem}", file=sys.stderr)
                    except:
                        pass
                else:
                    print(f"   📄 {item}", file=sys.stderr)
        except Exception as e:
            print(f"   ❌ Error listing directory: {e}", file=sys.stderr)

    # ***Debug: Check volume mount info
    print(f"📂 Volume mount info:", file=sys.stderr)
    try:
        with open('/proc/mounts', 'r') as f:
            for line in f:
                if '/workspace' in line:
                    print(f"   {line.strip()}", file=sys.stderr)
    except:
        print(f"   Could not read mount info", file=sys.stderr)

    # Validate the submission, work directory, and results directory
    if not os.path.isfile(submission_path):
        result = {
            "error": f"Submission file does not exist: {submission_path}"
        }
        write_results_and_exit(results_dir, result, 1)
    if not os.path.isdir(work_dir):
        result = {
            "error": f"Working directory does not exist: {work_dir}"
        }
        write_results_and_exit(results_dir, result, 1)
    if not os.path.isdir(results_dir):
        result = {
            "error": f"Results directory does not exist: {results_dir}"
        }
        write_results_and_exit(results_dir, result, 1)

    original_filename = os.path.basename(submission_path)

    print(f"Processing {original_filename} -> {work_dir}", file=sys.stderr)
    
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
        write_results_and_exit(results_dir, result, 1)

    # Simulate some grading time
    print(f"🔬 Grading {original_filename}...", file=sys.stderr)
    time.sleep(1)
    
    # Return dummy score and feedback
    result["score"] = 85.0
    result["feedback"] += "✅ Dummy grading complete - file processed successfully!"
    
    # Log the result
    print(f"✅ Grading complete. Score: {result['score']}/{result['max_score']}", file=sys.stderr)
    
    # Write results to output.json and exit
    write_results_and_exit(results_dir, result, 0)

if __name__ == "__main__":
    main()
