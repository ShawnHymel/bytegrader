"""
Test suite for C code grading.

This module implements the `BaseSuite` class to provide a custom grading
logic for C code submissions.
"""

import logging
from pathlib import Path
import subprocess

from bytegrader import BaseSuite

# Settings
EXEC_NAME = "main"

class CompileSuite(BaseSuite):
    """Compile Suite for grading C code submissions.

    This suite implements the `run` method to compile and test C code
    submissions.
    """
    
    def __init__(self, *args, **kwargs):
        """Initialize the C compilation suite with flexible arguments.

        Args:
            *args: Positional arguments for the base class.
            **kwargs: Keyword arguments for the base class.
        """
        super().__init__(*args, **kwargs)
    
    def run(self):
        """Compile C code submission"""
        
        # Run make in the work directory
        try:
            self.logger.debug(f"Compiling C code in {self.work_path}")
            result = subprocess.run(
                ["make"],
                cwd=self.work_path,
                check=True,
                capture_output=True,
            )

            # Add feedback based on the compilation result
            if result.stderr:
                self.logger.debug(f"Compilation errors: {result.stderr.decode()}")
                self.result.success = False
                self.result.error = f"Compilation failed with errors: {result.stderr.decode()}"
            else:
                self.logger.info("Compilation completed successfully without errors.")
                self.result.success = True
                self.result.add_feedback("Compilation successful")
                self.result.set_score(20)

        except subprocess.CalledProcessError as e:
            self.logger.error(f"Compilation failed: {e.stderr.decode()}")
            self.result.success = False
            self.result.error = e.stderr.decode()
            self.result.add_feedback(f"Compilation failed: {e.stderr.decode()}")
        
        return self.result
    
class RunSuite(BaseSuite):
    """Run suite for grading C code submissions."""
    
    def __init__(self, *args, **kwargs):
        """Initialize the C compilation suite with flexible arguments.

        Args:
            *args: Positional arguments for the base class.
            **kwargs: Keyword arguments for the base class.
        """
        super().__init__(*args, **kwargs)
    
    def run(self):
        """Run C code submission"""

        # Ensure the C program has been compiled before running
        if not (self.work_path / EXEC_NAME).exists():
            self.logger.error(f"C program {EXEC_NAME} does not exist. Please compile first.")
            return False

        score = 0
        try:

            # Test 1: Add two numbers using the compiled C program and check the output
            self.logger.debug(f"Running C program in {self.work_path}")
            result = subprocess.run(
                [f"./{EXEC_NAME}", "1", "2"],
                cwd=self.work_path,
                check=True,
                capture_output=True,
            )
            val = result.stdout.decode().strip()
            self.logger.debug(f"Program output: {val}")

            # Check if the output is as expected
            if val == "3":
                self.logger.info("Test 1 passed: Output is '3'")
                self.result.add_feedback("Test 1 passed")
                score += 10
            else:
                self.logger.info(f"Test 1 failed: Expected output '3', but got '{val}'")
                self.result.add_feedback(f"Test 1 failed")
            
            # Test 2: Add two numbers but with an incorrect expected output
            result = subprocess.run(
                [f"./{EXEC_NAME}", "-140", "30"],
                cwd=self.work_path,
                check=True,
                capture_output=True,
            )
            val = result.stdout.decode().strip()
            self.logger.debug(f"Program output: {val}")

            # Check if the output is as expected (purposefully incorrect for testing)
            if val == "-120":
                self.logger.info("Test 2 passed: Output is '-120'")
                self.result.add_feedback("Test 2 passed")
                score += 10
            else:
                self.logger.info(f"Test 2 failed: Expected output '-120', but got '{val}'")
                self.result.add_feedback(f"Test 2 failed")

        except subprocess.CalledProcessError as e:

             # Show error details if the run fails
            stderr_content = e.stderr.decode() if e.stderr else "(no stderr)"
            stdout_content = e.stdout.decode() if e.stdout else "(no stdout)"
            self.logger.error(f"Run failed with return code {e.returncode}")
            self.logger.error(f"stderr: {stderr_content}")
            self.logger.error(f"stdout: {stdout_content}")

            # Fail the suite and add feedback
            self.result.success = False
            self.result.error = (
                f"Run failed with return code {e.returncode}. "
                f"stderr: {stderr_content}, stdout: {stdout_content}"
            )

        # Set the final score and return the result
        self.result.set_score(score)
        self.result.success = True
        if score == 20:
            self.result.add_feedback("All tests passed successfully.")
        else:
            self.result.add_feedback(f"Some tests failed. Total score: {score}/20")

        return self.result
