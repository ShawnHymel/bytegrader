from dataclasses import dataclass

@dataclass
class GradingResult:
    score: float
    max_score: float = 100.0
    feedback: str = ""
    error: str = ""
    
    def to_dict(self) -> dict:
        return {
            "score": self.score,
            "max_score": self.max_score,
            "feedback": self.feedback,
            "error": self.error
        }
    
    def is_valid(self) -> bool:
        return (
            0.0 <= self.score <= self.max_score and
            isinstance(self.feedback, str) and
            isinstance(self.error, str)
        )
