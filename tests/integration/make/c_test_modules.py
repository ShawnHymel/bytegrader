"""
Test module for C code grading.

This module implements the `BaseModule` class to provide a custom grading
logic for C code submissions.
"""

import logging
from pathlib import Path
import subprocess

from bytegrader.base_module import BaseModule

class CCompilationModule(BaseModule):
    """C Compilation Module for grading C code submissions.

    This module implements the `run` method to compile and test C code
    submissions.
    """
    
    def __init__(
        self, 
        work_path: Path,
        config: dict = None,
        logger: logging.Logger = None
    ):
        """Initialize the C compilation module with a submission path

        Args:
            work_path (Path): Path to the submission directory.
            config (dict, optional): Configuration dictionary for the grading module.
            logger (logging.Logger, optional): Logger instance for logging messages.
        """
        super().__init__(work_path, config, logger)
    
    def run(self):
        """Compile C code submission"""
        
        # Run make in the submission directory
        try:
            result = subprocess.run(
                ["make"],
                cwd=self.work_path,
                check=True,
                capture_output=True,
            )

        except subprocess.CalledProcessError as e:
            print(f"Compilation failed: {e.stderr.decode()}")
            return False
        
        return True