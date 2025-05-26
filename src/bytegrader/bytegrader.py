"""Entry point for the ByteGrader autograder application

Main framework that orchestrates the autograding process.
It loads and runs course-specific configurations, manages the grading process,
and handles the output of results.

All submissions are expected to be in a zip file format.
"""

import argparse
from datetime import datetime
import importlib
import importlib.util
import logging
import multiprocessing
import pathlib
import resource
import shutil

import yaml

from bytegrader import __version__
from bytegrader.base_suite import BaseSuite
from bytegrader.suite_result import SuiteResult

# Settings
DEFAULT_UNZIP_DIR = "temp_unzip"
DEFAULT_SKIP = False
DEFAULT_STOP_ON_FAILURE = False
DEFAULT_TIMEOUT_SEC = 30
DEFAULT_RAM_LIMIT_MB = 512
DEFAULT_FILE_SIZE_LIMIT_MB = 50
DEFAULT_NUM_PROC_LIMIT = 100
DEFAULT_NUM_OPEN_FILES_LIMIT = 100

# Configure logging
logger = logging.getLogger(__package__)

################################################################################
# Module-level functions

def configure_logging(logger_level: int) -> None:
    """Configure logging for the suite

    Args:
        logger_level (int): Logging level to set
    """
    logging.basicConfig(
        level=logger_level,
        format="%(asctime)s %(name)s [%(levelname)s]: %(message)s",
        force=True,
        handlers=[logging.StreamHandler()],
    )
    logger.setLevel(logger_level)
    logger.debug(f"Logging configured at level: {logger_level}")

def run_suite_process(
    suite_class: BaseSuite, 
    work_path: str,
    submission_id: int,
    suite_config: dict,
    logger_level: int,
    return_dict: dict,
) -> None:
    """Run a suite in a separate process

    Args:
        suite_class (BaseSuite): The suite class to run
        work_path (str): Path to the unzipped submission directory
        submission_id (int): Unique identifier for the student's submission
        suite_config (dict): Configuration for the suite
        logger_level (int): Logging level for the suite. Defaults to logging.INFO.
        return_dict (dict): Dictionary to store the result of the suite run
    """
    feedback = None
    try:
        # Set up logging for the suite
        logging.basicConfig(
            level=logger_level,
            format="%(asctime)s %(name)s [%(levelname)s]: %(message)s",
            force=True,
            handlers=[logging.StreamHandler()],
        )
        suite_logger = logging.getLogger(suite_class.__name__)
        suite_logger.setLevel(logger_level)
        

        # Set resource limits for the suite
        timeout_sec = suite_config.get("timeout_sec", DEFAULT_TIMEOUT_SEC)
        ram_limit_mb = suite_config.get("ram_limit_mb", DEFAULT_RAM_LIMIT_MB)
        file_size_limit_mb = suite_config.get("file_size_limit_mb", DEFAULT_FILE_SIZE_LIMIT_MB)
        num_proc_limit = suite_config.get("num_proc_limit", DEFAULT_NUM_PROC_LIMIT)
        num_open_files_limit = suite_config.get(
            "num_open_files_limit", 
            DEFAULT_NUM_OPEN_FILES_LIMIT
        )
        suite_logger.debug(
            f"Setting resource limits for suite {suite_class.__name__}: "
            f"timeout={timeout_sec}s, ram={ram_limit_mb}MB, "
            f"file_size={file_size_limit_mb}MB, "
            f"num_proc={num_proc_limit}, num_open_files={num_open_files_limit}"
        )
        
        # Set resource limits
        resource.setrlimit(resource.RLIMIT_CPU, (timeout_sec, timeout_sec))
        resource.setrlimit(resource.RLIMIT_AS, (ram_limit_mb * 1024 * 1024, ram_limit_mb * 1024 * 1024))
        resource.setrlimit(resource.RLIMIT_FSIZE, (file_size_limit_mb * 1024 * 1024, file_size_limit_mb * 1024 * 1024))
        resource.setrlimit(resource.RLIMIT_NPROC, (num_proc_limit, num_proc_limit))
        resource.setrlimit(resource.RLIMIT_NOFILE, (num_open_files_limit, num_open_files_limit))

        # Instantiate the suite class and run it
        suite_instance = suite_class(
            work_path=work_path,
            submission_id=submission_id,
            config=suite_config,
            logger=suite_logger,
        )
        result = suite_instance.run()

        # Validate that a SuiteResult was returned
        if not isinstance(suite_instance.result, SuiteResult):
            raise TypeError(
                f"Suite {suite_class.__name__} did not return a SuiteResult instance"
            )

        # Extract data for multiprocessing
        return_dict['success'] = result.success
        return_dict['score'] = result.score
        return_dict['max_score'] = result.max_score
        return_dict['feedback_messages'] = result.feedback_messages
        return_dict['error'] = result.error

    except Exception as e:
        suite_logger.error(f"Error running suite {suite_class.__name__}: {e}")
        return_dict['success'] = False
        return_dict['error'] = str(e)
        return_dict['feedback_messages'] = []
        return_dict['score'] = 0
        return_dict['max_score'] = suite_config.get('max_score', 0)

################################################################################
# Classes

class SuiteRunner:
    """Discovers, loads, and runs the course-specific grading suites"""

    def __init__(
        self, 
        config: dict, 
        config_dir_path: str, 
        work_path: str,
        submission_id: int,
    ):
        """Initialize the suite runner with the path to the suites

        Args:
            config (dict): Configuration dictionary containing suite paths and settings
            config_dir_path (str): Path to the directory containing the configuration file
            work_path (str): Path to the unzipped submission directory
            submission_id (int): Unique identifier for the student's submission

        Raises:
            ValueError: If the config or suite path is invalid or not provided
        """

        # Validate config
        if not config:
            raise ValueError("Configuration is required")
        self._config = config

        # Validate config path
        self._config_dir_path = pathlib.Path(config_dir_path).resolve()
        if not self._config_dir_path.is_dir():
            raise ValueError(f"Configuration directory does not exist: {self._config_dir_path}")
        
        # Validate submission path
        if not work_path:
            raise ValueError("Submission path is required")
        self._work_path = pathlib.Path(work_path).resolve()
        logger.debug(f"Work path resolved to: {self._work_path}")

        # Load suites
        self._suites = {}
        self._suite_names = []
        self._submission_id = submission_id
        self._load_suites()

    @property
    def suite_names(self):
        """Get the loaded suite names
        Returns:
            list: List of suite names that have been loaded
        """
        return self._suite_names
    
    def get_suite_config(self, suite_name: str) -> dict:
        """Get the configuration for a specific suite

        Args:
            suite_name (str): Name of the suite to get the configuration for

        Returns:
            dict: Configuration dictionary for the specified suite
        """
        if suite_name not in self._suites:
            raise ValueError(f"Suite '{suite_name}' not found")
        return self._suites[suite_name].get('config', {})

    def run_suite(
        self, 
        suite_name: str
    ) -> SuiteResult:
        """Run a specific suite with the given submission path

        Args:
            suite_name (str): Name of the suite to run

        Returns:
            result: Result of the suite's run method
        """

        # Validate suite name
        if suite_name not in self._suites:
            raise ValueError(f"Suite '{suite_name}' not found")
        
        # Get the suite configuration
        suite_config = self._suites[suite_name].get('config', {})
        if not suite_config:
            raise ValueError(f"Configuration for suite '{suite_name}' is missing or invalid")
        
        # Get timeout
        timeout_sec = suite_config.get("timeout_sec", DEFAULT_TIMEOUT_SEC)
        if timeout_sec <= 0:
            raise ValueError(f"Invalid timeout for suite '{suite_name}': {timeout_sec} seconds")
        
        # Initialize the result
        result = SuiteResult(
            success=False,
            score=0.0,
            max_score=suite_config.get("max_score", 0.0),
            feedback_messages=[],
            error=None,
        )

        try:
            # Get the suite class
            suite_class = self._suites[suite_name]['class']

            # Create a dictionary to store the result of the suite run
            return_dict = multiprocessing.Manager().dict()

            # Create a new process for the suite
            process = multiprocessing.Process(
                target=run_suite_process,
                args=(
                    suite_class, 
                    self._work_path,
                    self._submission_id,
                    suite_config, 
                    logger.level,
                    return_dict,
                )
            )

            # Start the process
            process.start()
            process.join(timeout=timeout_sec)
            if process.is_alive():
                logger.warning(f"Suite '{suite_name}' timed out after {timeout_sec} seconds")
                process.terminate()
                result.success = False
                result.error = f"Suite '{suite_name}' timed out after {timeout_sec} seconds"
                return result
            
            # Check exit code
            if process.exitcode != 0:
                logger.error(f"Suite '{suite_name}' exited with code {process.exitcode}")
                result.success = False
                result.error = f"Suite '{suite_name}' exited with code {process.exitcode}"
                return result

        except Exception as e:
            logger.error(f"Error running suite '{suite_name}': {e}")
            return None
        
        # Extract the result from the return dictionary
        result.success = return_dict['success']
        result.score = return_dict.get('score', 0.0)
        result.max_score = return_dict.get('max_score', 0.0)
        result.feedback_messages = return_dict.get('feedback_messages', [])
        result.error = return_dict.get('error', None)

        # Log the result
        logger.debug(f"Suite '{suite_name}' result: {result}")
        if result.success:
            logger.info(
                f"Suite '{suite_name}' for ID {self._submission_id} completed with "
                f"score {result.score}/{result.max_score}"
            )
        else:
            logger.error(f"Suite '{suite_name}' for ID {self._submission_id} failed.")

        return result

    def _load_suites(self):
        """Load the course-specific grading suite"""
        
        # Create list of complete suite paths from config
        suites = self._config.get("suites", [])
        if not suites:
            raise ValueError("No suites specified in configuration")
        
        # Load each suite specified in the configuration
        for suite in suites:

            # Get the suite name and config
            suite_name = next(iter(suite))
            suite_config = suite[suite_name]
            if suite.get("skip", DEFAULT_SKIP):
                logger.info(f"Skipping suite: {suite_name}")
                continue
            logger.debug(f"Loading suite: ''{suite_name}''")

            # Get absolute path to the suite file
            if pathlib.Path(suite_config["path"]).is_absolute():
                suite_path = pathlib.Path(suite_config["path"])
            else:
                suite_path = self._config_dir_path / suite_config["path"]
            suite_path = suite_path.resolve()
            logger.debug(f"Suite path: {suite_path}")
            
            try:
                # Import suite from file
                spec = importlib.util.spec_from_file_location(
                    suite_name, 
                    suite_path,
                )
                if spec is None:
                    logger.warning(f"Could not create spec for {suite_path}")
                    continue

                # Load the suite
                suite = importlib.util.module_from_spec(spec)
                if suite is None:
                    logger.warning(f"Could not load suite {suite_path}")
                    continue
                spec.loader.exec_module(suite)
                if suite is None:
                    logger.warning(f"Suite {suite_path} is None after loading")
                    continue
                
                # Find BaseSuite class that matches the "class" key in the config
                if 'class' in suite_config:
                    class_name = suite_config['class']
                    logger.debug(f"Looking for class {class_name} in suite {suite_path}")
                    if hasattr(suite, class_name):
                        suite_class = getattr(suite, class_name)
                        if isinstance(suite_class, type) and issubclass(suite_class, BaseSuite):
                            self._suites[suite_name] = {'class': suite_class}
                            self._suites[suite_name]['config'] = suite_config
                            logger.info(f"Discovered test suite: {class_name}")
                        else:
                            logger.error(
                                f"{class_name} is not a valid BaseSuite subclass in {suite_path}"
                            )
                            continue
                    else:
                        logger.error(f"{class_name} not found in suite {suite_path}")
                        continue
            
            except Exception as e:
                logger.error(f"Failed to load suite {suite_path}: {e}")
                continue
        
        # Get a list of suite names
        self._suite_names = list(self._suites.keys())

        # Log loaded suites
        logger.info(f"Loaded suites: {self._suite_names}")
        logger.debug(f"Available suites: {self._suites}")
        if not self._suites:
            logger.warning("No suites loaded")

class Autograder:
    """Main class for the autograder framework

    This class is responsible for loading the course configuration,
    running the grading process, and handling the output of results.
    """

    def __init__(
            self, 
            config_path: str,
            submission_id: int = -1,
            output_path: str = "./output.txt",
            work_dir: str = "./",
        ):
        """Initialize the autograder with optional configuration and suite paths

        Args:
            config_path (str): Path to the configuration file (YAML format)
            submission_id (int, optional): Unique identifier for the student's submission.
            output_path (str, optional): Path to output directory for results. 
                Defaults to current directory.
            work_dir (str, optional): Path to the directory where the 
                submission is unzipped and compiled. Defaults to current directory.
        """

        # Set attributes from arguments
        self._config_path = pathlib.Path(config_path).resolve()
        self._output_path = pathlib.Path(output_path).resolve()
        self._submission_id = submission_id

        # Validate work directory, create if it does not exist
        if not work_dir:
            raise ValueError("Work directory is required")
        self._work_path = pathlib.Path(work_dir).resolve()
        if not self._work_path.is_dir():
            logger.debug(f"Creating work directory: {self._work_path}")
            self._work_path.mkdir(parents=True, exist_ok=True)
        logger.debug(f"Work path resolved to: {self._work_path}")


        # Load configuration
        self._config = self._load_config()
        if not self._config:
            raise ValueError("Configuration could not be loaded")
        logger.debug(f"Configuration loaded: {self._config}")

        # Create runner
        self._suite_runner = SuiteRunner(
            self._config, 
            self._config_path.parent, 
            self._work_path,
            self._submission_id,
        )

    def grade(self):
        """Run the grading process"""
        logger.info("Starting grading process")
        
        # Record total/max score and feedback messages
        total_score = 0.0
        total_max_score = 0.0
        feedback = []

        # Get timestamp
        timestamp = datetime.now()

        # Run each suite specified in the configuration
        for suite_name in self._suite_runner.suite_names:

            # Add max score
            suite_config = self._suite_runner.get_suite_config(suite_name)
            suite_max_score = suite_config.get("max_score", 0.0)
            total_max_score += suite_max_score

            # Delineate the suite in the feedback
            feedback.append("")
            feedback.append(f"=== Suite: {suite_name} ===")

            # Run the suite
            logger.info(f"Running suite: '{suite_name}'")
            try:
                result = self._suite_runner.run_suite(suite_name)
            except Exception as e:
                logger.error(f"Error running suite '{suite_name}': {e}")
                feedback.append(f"Error running suite '{suite_name}': {e}")
                feedback.append(f"Suite score: 0 / {suite_max_score}")
                continue

            # Handle the result of the suite run
            if result is None:
                logger.error(f"Suite '{suite_name}' did not return a valid result")
                feedback.append(f"Suite '{suite_name}' did not return a valid result")
                feedback.append(f"Suite score: 0 / {suite_max_score}")
                continue

            # Stop grading if the suite failed and config is set to stop on failure
            stop_on_failure = self._suite_runner.get_suite_config(suite_name).get(
                "stop_on_failure", 
                DEFAULT_STOP_ON_FAILURE,
            )
            if not result.success and stop_on_failure:
                logger.warning(f"Suite '{suite_name}' failed, stopping further grading")
                feedback.append(
                    f"Suite failed: {result.error or 'No error message provided'}"
                )
                feedback.append(f"Suite score: 0 / {suite_max_score}")
                break

            # If the suite did not run successfully, log the error
            if not result.success:
                logger.error(f"Suite '{suite_name}' failed: {result.error}")
                feedback.append(
                    f"Suite failed: {result.error or 'No error message provided'}"
                )
                feedback.append(f"Suite score: 0 / {suite_max_score}")
                continue

            # Update total score
            total_score += result.score

            # Collect feedback messages
            if result.feedback_messages:
                feedback.extend(result.feedback_messages)
                logger.info(f"Feedback from suite '{suite_name}': {result.feedback_messages}")

            # Add suite score to feedback
            suite_score = f"Suite score: {result.score} / {result.max_score}"
            feedback.append(suite_score)

        # Get elapsed time
        elapsed_time = datetime.now() - timestamp

        # Log the final results
        logger.info(f"Grading completed for submission ID {self._submission_id}")
        logger.info(f"Total score: {total_score}/{total_max_score}")

        # Prepend stats to feedback
        feedback.insert(0, f"Submission ID: {self._submission_id}")
        feedback.insert(1, f"Total score: {total_score} / {total_max_score}")
        feedback.insert(2, f"Elapsed time: {elapsed_time}")
        
        # Create output directory if it does not exist
        if self._output_path.parent and not self._output_path.parent.exists():
            logger.debug(f"Creating output directory: {self._output_path.parent}")
            self._output_path.parent.mkdir(parents=True, exist_ok=True)

        # Remove existing output file if it exists
        if self._output_path.exists():
            logger.debug(f"Removing existing output file: {self._output_path}")
            self._output_path.unlink()

        # Write feedback to the output file
        logger.debug(f"Feedback messages: {feedback}")
        with open(self._output_path, 'w') as output_file:
            for line in feedback:
                output_file.write(f"{line}\n")
        logger.info(f"Feedback written to {self._output_path}")
        
    def extract_submission(self, submission: str):
        """Copy and extract the submission zip file

        Returns:
            pathlib.Path: Path to the extracted directory for grading
        """
        if not submission:
            raise ValueError("Path to submission zip file is required")

        # Create a temporary directory for the submission
        if self._work_path:
            temp_dir = pathlib.Path(self._work_path)
        else:
            temp_dir = pathlib.Path(DEFAULT_UNZIP_DIR)
        temp_dir.mkdir(parents=True, exist_ok=True)
        logger.info(f"Temporary directory created at {temp_dir}")

        # Copy the submission zip file to the temporary directory
        shutil.copy(submission, temp_dir / "submission.zip")

        # Extract the zip file and remove the zip file
        shutil.unpack_archive(temp_dir / "submission.zip", temp_dir)
        (temp_dir / "submission.zip").unlink()

        # Check if the extracted directory is valid
        if not temp_dir.is_dir():
            raise ValueError(f"Invalid submission directory: {temp_dir}")
    
    def _load_config(self):
        """Load the autograder configuration from a YAML file

        Returns:
            dict: Configuration dictionary
        """
        if not self._config_path:
            raise ValueError("Configuration path is required")

        try:
            with open(self._config_path, 'r') as config_file:
                config = yaml.safe_load(config_file)
                logger.info(f"Loaded configuration from {self._config_path}")
                return config
        except Exception as e:
            logger.error(f"Failed to load configuration: {e}")
            raise

################################################################################
# Main entry point

def main():
    """Main entry point"""

    # Command line arguments
    parser = argparse.ArgumentParser(description="Modular Autograder Framework")
    parser.add_argument(
        "--config",
        "-c",
        required=True,
        type=str,
        help="Path to configuration file (YAML format)",
    )
    parser.add_argument(
        "--debug",
        "-d",
        action="store_true", 
        help="Enable debug logging",
    )
    parser.add_argument(
        "--id",
        "-i",
        type=int,
        default=-1,
        help="Unique identifier for the student's submission (default: -1)",
    )
    parser.add_argument(
        "--output",
        "-o",
        type=str,
        default="./output.txt",
        help="Path to output file for results. Defaults to ./output.txt. Note that any existing "
            "file will be overwritten.",
    )
    parser.add_argument(
        "--submission",
        "-s",
        type=str,
        help="Path to the submission zip file",
    )
    parser.add_argument(
        "--work_dir",
        "-w",
        required=True,
        type=str,
        help="Path to the directory where the submission is unzipped and compiled. This directory "
            "will be created if it does not exist.",
    )
    
    # Parse arguments
    args = parser.parse_args()

    # If debug mode is enabled, set logging to DEBUG level
    if args.debug:
        configure_logging(logging.DEBUG)
    else:
        configure_logging(logging.INFO)

    # Print welcome message
    logger.info(f"ByteGrader Autograder Framework v{__version__}")

    # Initialize the autograder
    autograder = Autograder(
        config_path=args.config,
        submission_id=args.id,
        output_path=args.output,
        work_dir=args.work_dir,
    )

    # Extract the submission if provided
    if args.submission:
        try:
            autograder.extract_submission(args.submission)
            logger.info(f"Submission extracted to {args.work_dir}")
        except Exception as e:
            logger.error(f"Failed to extract submission: {e}")
            return
    else:
        logger.info(f"Skipping extraction. Grading in {args.work_dir}")
        
    # Run the grading process
    autograder.grade()

if __name__ == "__main__":
    main()
