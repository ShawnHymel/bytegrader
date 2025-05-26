"""
Abstract test class for creating custom grading suites.
"""

from abc import ABC, abstractmethod
import logging
from pathlib import Path

from bytegrader.suite_result import SuiteResult

class BaseSuite(ABC):
    """Abstract base class for test suites.
    
    This class defines the interface for creating custom grading suites.
    Subclasses should implement the `run` method to define their own grading 
    logic.
    """
    def __init__(
        self, 
        work_path: Path,
        submission_id: int,
        config: dict = None,
        logger: logging.Logger = None,
    ):
        """Initialize the base suite with a configuration dictionary.
        
        Args:
            work_path (Path): Path to the unzipped submission directory.
            submission_id (int): ID of the submission being graded.
            config (dict, optional): Configuration dictionary for the grading suite.
            logger (logging.Logger, optional): Logger instance for logging messages.
        """
        self.work_path = work_path
        self._submission_id = submission_id
        self.config = config if config is not None else {}
        self.logger = logger if logger is not None else logging.getLogger(__name__)

        # Initialize the suite result
        self.result = SuiteResult()
        self.result.max_score = self.config.get("max_score", 0.0)

    @property
    def submission_id(self) -> int:
        """Get the submission ID."""
        return self._submission_id

    @abstractmethod
    def run(self) -> SuiteResult:
        """Run the grading logic.
        
        This method should be implemented by subclasses to define their own 
        grading logic. Must return a `SuiteResult` instance.

        Returns:
            SuiteResult: The result of the grading suite.
        """
        pass
        