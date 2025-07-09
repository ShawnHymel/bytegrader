import time

from result import GradingResult

def grade_submission(extracted_path: str) -> GradingResult:
    """
    Grade the test stub submission.
    This function receives the path to extracted student files and returns a GradingResult.
    """
    
    # Simulate some grading time
    time.sleep(30)
    
    # Construct dummy score and feedback
    result = GradingResult(
        score=85.0,
        max_score=100.0,
        feedback="Dummy grading complete - file processed successfully!",
        error=""
    )
    
    return result