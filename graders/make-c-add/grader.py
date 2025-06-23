import subprocess
from pathlib import Path
import sys

from result import GradingResult

def grade_submission(extracted_path: str) -> GradingResult:
    """
    Grade a C program that adds two float numbers from command line arguments.
    
    Expected submission:
    - main.c: C source file that takes two float args and prints their sum
    - Makefile: Build file that compiles to executable named 'main'
    
    Args:
        extracted_path: Path to extracted student submission
        
    Returns:
        GradingResult with score, feedback, and any errors
    """
    
    try:
        # Check for required files
        main_c = Path(extracted_path) / "main.c"
        makefile = Path(extracted_path) / "Makefile"
        
        feedback = []
        score = 0.0
        max_score = 100.0
        
        # Check if required files exist
        if not main_c.exists():
            return GradingResult(
                score=0.0,
                max_score=max_score,
                feedback="main.c not found",
                error="main.c not found"
            )
        
        if not makefile.exists():
            return GradingResult(
                score=0.0,
                max_score=max_score,
                feedback="Makefile not found",
                error="Makefile not found"
            )
        
        # Add feedback for found files
        feedback.append("Required files found: main.c, Makefile")
        
        # Build the program (20 points)
        print(f"Building program with make...", file=sys.stderr)
        try:
            build_result = subprocess.run(
                ['make'], 
                cwd=extracted_path, 
                capture_output=True, 
                text=True, 
                timeout=30
            )
            
            if build_result.returncode != 0:
                return GradingResult(
                    score=0.0,
                    max_score=max_score,
                    feedback=f"Build failed:\n{build_result.stderr}",
                    error="Compilation error"
                )
            
            feedback.append("Build successful")
            score += 20.0
            
        except subprocess.TimeoutExpired:
            return GradingResult(
                score=0.0,
                max_score=max_score,
                feedback="Build timed out",
                error="Build timed out"
            )
        
        # Check if executable was created (10 points)
        executable = Path(extracted_path) / "main"
        if not executable.exists():
            return GradingResult(
                score=score,
                max_score=max_score,
                feedback="Executable 'main' not found after build",
                error="Missing executable"
            )
        feedback.append("Executable 'main' created")
        score += 10.0
        
        # Test cases: (a, b, expected_sum)
        test_cases = [
            (2.5, 3.7, 6.2),
            (10.0, -5.5, 4.5),
            (0.0, 0.0, 0.0),
            (-3.2, -1.8, -5.0),
            (100.5, 0.5, 101.0)
        ]
        
        # Points per test case (70 points total)
        points_per_test = 70.0 / len(test_cases)
        
        # Run each test case
        for i, (a, b, expected) in enumerate(test_cases):
            try:
                # Run the program with test inputs
                test_result = subprocess.run(
                    ['./main', str(a), str(b)], 
                    cwd=extracted_path, 
                    capture_output=True, 
                    text=True, 
                    timeout=5
                )
                
                if test_result.returncode != 0:
                    feedback.append(f"Test {i+1}: {a} + {b} → Program crashed")
                    print(f"Test {i+1}: Program crashed with args {a}, {b}", file=sys.stderr)
                    continue
                
                # Parse the output
                output = test_result.stdout.strip()
                try:
                    actual = float(output)
                    
                    # Check if result is close enough (allow small floating point errors)
                    if abs(actual - expected) < 0.001:
                        score += points_per_test
                        feedback.append(f"Test passed: {i+1}: {a} + {b} = {actual}")
                    else:
                        feedback.append(f"Test failed: {i+1} {a} + {b} = {actual} (expected {expected})")
                        
                except ValueError:
                    feedback.append(f"Test {i+1}: {a} + {b} → Invalid output: '{output}'")
                    
            except subprocess.TimeoutExpired:
                feedback.append(f"Test {i+1}: {a} + {b} → Timeout")
        
        # Calculate final score
        final_score = min(score, max_score)
        print(f"Grading complete. Final score: {final_score}/{max_score}", file=sys.stderr)
        
        return GradingResult(
            score=final_score,
            max_score=max_score,
            feedback="\n".join(feedback),
            error=""
        )
        
    except Exception as e:
        print(f"Grading exception: {e}", file=sys.stderr)
        return GradingResult(
            score=0.0,
            max_score=100.0,
            feedback="Internal grading error occurred",
            error=str(e)
        )
