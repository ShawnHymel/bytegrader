"""
Test module for C code grading.

This module implements the `BaseModule` class to provide a custom grading
logic for C code submissions.
"""

import subprocess

from bytegrader.base_module import BaseModule

class CCompilationModule(BaseModule):
    """C Compilation Module for grading C code submissions.

    This module implements the `run` method to compile and test C code
    submissions.
    """
    
    def __init__(self, submission_path: str):
        """Initialize the C compilation module with a submission path

        Args:
            config (dict): Configuration dictionary for the grading module.
        """
        super().__init__(submission_path)
    
    def run(self):
        """Compile C code submission"""
        
        # Run make in the submission directory
        try:
            result = subprocess.run(
                ["make"],
                cwd=self.submission_path,
                check=True,
                capture_output=True,
            )
            print(result.stdout.decode())
        except subprocess.CalledProcessError as e:
            print(f"Compilation failed: {e.stderr.decode()}")
            return False