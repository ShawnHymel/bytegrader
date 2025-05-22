"""
Abstract test module for creating custom grading modules.
"""

from abc import ABC, abstractmethod
from pathlib import Path

class BaseModule(ABC):
    """Abstract base class for test modules.
    
    This class defines the interface for creating custom grading modules.
    Subclasses should implement the `run` method to define their own grading 
    logic.
    """
    def __init__(self, submission_path: Path):
        """Initialize the base module with a configuration dictionary.
        
        Args:
            config (dict): Configuration dictionary for the grading module.
        """
        self.submission_path = submission_path

    @abstractmethod
    def run(self):
        """Run the grading logic.
        
        This method should be implemented by subclasses to define their own 
        grading logic.
        """
        pass
        