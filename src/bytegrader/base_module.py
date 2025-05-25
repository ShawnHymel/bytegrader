"""
Abstract test module for creating custom grading modules.
"""

from abc import ABC, abstractmethod
import logging
from pathlib import Path

class BaseModule(ABC):
    """Abstract base class for test modules.
    
    This class defines the interface for creating custom grading modules.
    Subclasses should implement the `run` method to define their own grading 
    logic.
    """
    def __init__(
        self, 
        work_path: Path,
        config: dict = None,
        logger: logging.Logger = None
    ):
        """Initialize the base module with a configuration dictionary.
        
        Args:
            work_path (Path): Path to the unzipped submission directory.
            config (dict, optional): Configuration dictionary for the grading module.
            logger (logging.Logger, optional): Logger instance for logging messages.
        """
        self.work_path = work_path
        self.config = config if config is not None else {}
        self.logger = logger if logger is not None else logging.getLogger(__name__)

    @abstractmethod
    def run(self):
        """Run the grading logic.
        
        This method should be implemented by subclasses to define their own 
        grading logic.
        """
        pass
        