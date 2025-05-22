"""Entry point for the ByteGrader autograder application

Main framework that orchestrates the autograding process.
It loads and runs course-specific configurations, manages the grading process,
and handles the output of results.

All submissions are expected to be in a zip file format.
"""

import argparse
import importlib
import logging
import pathlib
import shutil

from bytegrader.base_module import BaseModule

# Settings
DEFAULT_UNZIP_DIR = "temp_unzip"

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s]: %(message)s"
)
logger = logging.getLogger(__name__)

class ModuleRunner:
    """Discovers, loads, and runs the course-specific grading modules"""

    def __init__(self, modules_path: str):
        """Initialize the module runner with the path to the modules

        Args:
            module_path (str): Path to the course-specific grading modules
        """
        self._modules_path = pathlib.Path(modules_path)
        self.modules = {}
        self._load_modules()

    def _load_modules(self):
        """Load the course-specific grading module"""

        # Check if the module path is valid
        if not self._modules_path.is_dir():
            raise ValueError(f"Invalid module path: {self._modules_path}")

        # Discover and load modules
        for file_path in self._modules_path.glob("*.py"):

            # Ignore any files that start with an underscore (e.g. __init__.py)
            if file_path.stem.startswith("_"):
                continue
            
            try:
                # Import module from file
                module_name = file_path.stem
                spec = importlib.util.spec_from_file_location(
                    module_name, 
                    file_path,
                )
                if spec is None:
                    logger.warning(f"Could not create spec for {file_path}")
                    continue
                module = importlib.util.module_from_spec(spec)
                
                # Find TestModule classes
                for attr_name in dir(module):
                    attr = getattr(module, attr_name)
                    if (
                        isinstance(attr, type) and 
                        issubclass(attr, BaseModule) and 
                        attr is not BaseModule
                    ):
                        self.available_modules[attr_name] = attr
                        logger.info(f"Discovered test module: {attr_name}")
            
            except Exception as e:
                logger.error(f"Failed to load module {file_path}: {e}")
        
        # Log loaded modules
        logger.info(f"Loaded {len(self.modules)} modules")
        if not self.modules:
            logger.warning("No modules loaded")

class Autograder:
    """Main class for the autograder framework

    This class is responsible for loading the course configuration,
    running the grading process, and handling the output of results.
    """

    def __init__(
            self, 
            config: str = None,
            modules_path: str = None,
            output: str = None,
            submission: str = None,
            unzip_path: str = None,
        ):
        """Initialize the autograder with optional configuration and module paths

        Args:
            config (str): Path to the autograder configuration file
            module (str): Path to the course-specific grading module
            submission (str): Path to the submission zip file
        """

        # Set attributes from arguments
        self._config = config
        self._modules_path = modules_path
        self._output = output
        self._submission = submission
        self._unzip_path = unzip_path

        # Create runner
        self._module_runner = ModuleRunner(self._modules_path)

    def grade(self):
        """Run the grading process"""
        logging.info("Starting grading process")

        # Copy and extract submission
        try:
            grading_path = self._copy_and_extract_submission()
            logging.info(f"Submission extracted to {grading_path}")
        except Exception as e:
            logging.error(f"Failed to extract submission: {e}")
            return
        
        # Load the course-specific grading module
        
        
    def _copy_and_extract_submission(self):
        """Copy and extract the submission zip file

        Returns:
            pathlib.Path: Path to the extracted directory for grading
        """
        if not self._submission:
            raise ValueError("Submission path is required")

        # Create a temporary directory for the submission
        if self._unzip_path:
            temp_dir = pathlib.Path(self._unzip_path)
        else:
            temp_dir = pathlib.Path(DEFAULT_UNZIP_DIR)
        temp_dir.mkdir(parents=True, exist_ok=True)
        logger.info(f"Temporary directory created at {temp_dir}")

        # Copy the submission zip file to the temporary directory
        shutil.copy(self._submission, temp_dir / "submission.zip")

        # Extract the zip file and remove the zip file
        shutil.unpack_archive(temp_dir / "submission.zip", temp_dir)
        (temp_dir / "submission.zip").unlink()

        # Check if the extracted directory is valid
        if not temp_dir.is_dir():
            raise ValueError(f"Invalid submission directory: {temp_dir}")

        return temp_dir

def main():
    """Main entry point"""

    # Command line arguments
    parser = argparse.ArgumentParser(description="Modular Autograder Framework")
    parser.add_argument(
        "--config",
        "-c",
        type=str,
        help="Path to autograder configuration file"
    )
    parser.add_argument(
        "--debug",
        "-d",
        action="store_true", 
        help="Enable debug logging",
    )
    parser.add_argument(
        "--modules_path", 
        "-m",
        type=str,
        help="Path to course-specific grading modules",
    )
    parser.add_argument(
        "--output",
        "-o",
        type=str,
        help="Path to output directory for results",
    )
    parser.add_argument(
        "--submission",
        "-s",
        type=str,
        help="Path to the submission zip file",
    )
    parser.add_argument(
        "--unzip_path",
        "-u",
        type=str,
        help="Path to the directory where the submission is unzipped",
    )
    
    # Parse arguments
    args = parser.parse_args()

    # If debug mode is enabled, set logging to DEBUG level
    if args.debug:
        logger.setLevel(logging.DEBUG)
        logger.debug("Debug mode enabled")
    
    # Run autograder
    autograder = Autograder(
        config=args.config,
        modules_path=args.modules_path,
        output=args.output,
        submission=args.submission,
        unzip_path=args.unzip_path,
    )
    autograder.grade()

if __name__ == "__main__":
    main()
