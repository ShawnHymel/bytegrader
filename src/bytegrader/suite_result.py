from dataclasses import dataclass
from typing import List, Optional

@dataclass
class SuiteResult:
    """Result that test suites must return
    
    Args:
        success (bool): Indicates if the suite ran successfully (separate from tests pass/fail).
        score (float): The score awarded by this suite.
        max_score (float): The maximum score possible for this suite.
        feedback_messages (List[str]): List of feedback messages from the suite.
        error (Optional[str]): Error message if the suite failed.
    """
    success: bool = True
    score: float = 0.0
    max_score: float = 0.0
    feedback_messages: List[str] = None
    error: Optional[str] = None
    
    def __post_init__(self):
        if self.feedback_messages is None:
            self.feedback_messages = []
    
    def add_feedback(self, message: str):
        """Add a feedback message
        
        Args:
            message (str): The feedback message to add.
        """
        self.feedback_messages.append(message)
    
    def get_score(self) -> float:
        """Get the score for this suite
        
        Returns:
            float: The score for this suite.
        """
        return self.score

    def set_score(self, score: float):
        """Set the score for this suite
        
        Args:
            score (float): The score to set for this suite.
        """
        self.score = score
