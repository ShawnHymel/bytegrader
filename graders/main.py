#!/usr/bin/env python3
"""
ByteGrader Universal Entrypoint
Loads and executes instructor grading scripts. Calls the `grade_submission` function
defined in the instructor's `grader.py` file located in the `/assignment` directory.
"""

import sys
import os
import json
import importlib.util
from pathlib import Path
import zipfile

import magic

from result import GradingResult

ALLOWED_FILE_TYPES = [
    'application/zip',
    'application/x-zip-compressed', 
    'application/zip-compressed',
]

# Add graders directory to path so we can import result.py
sys.path.insert(0, '/graders')

def determine_paths():
    """Determine file paths based on execution mode"""

    # If running in volume mode (production), extract to the container
    if os.getenv("BYTEGRADER_VOLUME_MODE") == "true":

        # Get job ID from environment variable
        job_id = os.getenv("BYTEGRADER_JOB_ID")
        if not job_id:
            raise ValueError("BYTEGRADER_JOB_ID environment variable not set")

        # Set paths based on job ID
        job_dir = f"/workspace/jobs/{job_id}"
        if not os.path.exists(job_dir):
            raise FileNotFoundError(f"Job directory does not exist: {job_dir}")
        
        # Paths for submission and results
        submission_path = os.path.join(job_dir, "submission", "submission.zip")
        results_dir = os.path.join(job_dir, "results")

    # If running locally, expect command line arguments
    else:
        if len(sys.argv) != 3:
            print("Usage: python3 main.py <submission_path> <results_dir>", 
                  file=sys.stderr)
            sys.exit(1)
        submission_path, results_dir = sys.argv[1:3]

    # Extract to ephemeral container filesystem (keep student files isolated)
    work_dir = "/tmp/grading"
    os.makedirs(work_dir, exist_ok=True)

    return submission_path, work_dir, results_dir

def find_grader_module():
    """Find the instructor's grader.py in /assignment directory"""

    # Locate the assignments's grader.py file
    grader_file = Path('/assignment/grader.py')
    if not grader_file.exists():
        raise FileNotFoundError("No grader.py found in /assignment directory")
    
    print(f"Loading grading module: {grader_file}", file=sys.stderr)
    
    # Dynamically import the module
    spec = importlib.util.spec_from_file_location("grader", grader_file)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    
    # Get the grade_submission function
    if not hasattr(module, 'grade_submission'):
        raise AttributeError("grader.py must have a 'grade_submission' function")
    
    return module.grade_submission

def validate_file_type(file_path, allowed_file_types=None):
    """Check if file exists and is of allowed type"""
    
    # Check if the file
    if not os.path.exists(file_path):
        return False
    
    # Get the MIME type of the file
    file_type = magic.from_file(file_path, mime=True)

    return file_type in allowed_file_types

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

def write_results(result, results_dir):
    """Write results to output.json"""
    if not result.is_valid():
        raise ValueError("Invalid grading result")
    
    output_file = os.path.join(results_dir, "output.json")
    with open(output_file, 'w') as f:
        json.dump(result.to_dict(), f, indent=2)

def main():
    """Main entrypoint for grading process"""

    # Get max unzip size from environment variable (default 100 MB) and convert to bytes
    max_unzip_size = int(os.getenv("MAX_UNZIP_SIZE_MB", 100)) * 1024 * 1024

    try:

        # Determine paths based on execution mode
        submission_path, work_dir, results_dir = determine_paths()
        
        # Print debug information
        print(f"Starting grading process", file=sys.stderr)
        print(f"   Submission: {submission_path}", file=sys.stderr)
        print(f"   Work dir: {work_dir}", file=sys.stderr)
        print(f"   Results dir: {results_dir}", file=sys.stderr)
        
        # Validate input paths exist
        if not os.path.exists(submission_path):
            raise FileNotFoundError(f"Submission file not found: {submission_path}")
        if not os.path.exists(work_dir):
            raise FileNotFoundError(f"Work directory not found: {work_dir}")
        if not os.path.exists(results_dir):
            raise FileNotFoundError(f"Results directory not found: {results_dir}")
        
        # Find and load instructor's grading function
        grade_function = find_grader_module()
        
        # Extract submission with security checks
        print(f"Extracting {submission_path}...", file=sys.stderr)
        safe_extract(submission_path, work_dir, max_unzip_size, ALLOWED_FILE_TYPES)
        print(f"Extraction complete to {work_dir}", file=sys.stderr)
        
        # Call instructor's grading function
        print(f"Starting grading...", file=sys.stderr)
        result = grade_function(work_dir)
        
        # Validate result type
        if not isinstance(result, GradingResult):
            raise ValueError(
                f"Grading function must return GradingResult object, got {type(result)}"
            )
        
        # Validate result contents
        if not result.is_valid():
            raise ValueError(
                f"Invalid grading result: score={result.score}, max_score={result.max_score}"
            )
        
        # Write results to output.json
        write_results(result, results_dir)
        print(f"Grading complete. Score: {result.score}/{result.max_score}", file=sys.stderr)
        
        # Also print to stdout for testing/debugging
        print(json.dumps(result.to_dict(), indent=2))
        
    except Exception as e:
        
        # Create proper error result
        error_result = GradingResult(
            score=0.0,
            max_score=100.0,
            feedback="",
            error=str(e)
        )
        
        try:
            write_results(error_result, results_dir)
        except:
            # Fallback if we can't write to results_dir
            try:
                output_file = "/results/output.json"
                with open(output_file, 'w') as f:
                    json.dump(error_result.to_dict(), f, indent=2)
            except:
                pass  # Give up on writing file
        
        # Print error
        print(f"Grading failed: {e}", file=sys.stderr)
        print(json.dumps(error_result.to_dict(), indent=2))

        sys.exit(1)

if __name__ == "__main__":
    main()
