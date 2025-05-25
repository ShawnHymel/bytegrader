"""Entry point for the ByteGrader autograder application

Main framework that orchestrates the autograding process.
It loads and runs course-specific configurations, manages the grading process,
and handles the output of results.

All submissions are expected to be in a zip file format.
"""

import argparse
import importlib
import importlib.util
import logging
import multiprocessing
import pathlib
import resource
import shutil

import yaml

from bytegrader import __version__
from bytegrader.base_module import BaseModule

# Settings
DEFAULT_UNZIP_DIR = "temp_unzip"
DEFAULT_TIMEOUT_SEC = 30
DEFAULT_RAM_LIMIT_MB = 512
DEFAULT_FILE_SIZE_LIMIT_MB = 50
DEFAULT_NUM_PROC_LIMIT = 100
DEFAULT_NUM_OPEN_FILES_LIMIT = 100

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s]: %(message)s"
)
logger = logging.getLogger(__name__)

def run_module_process(
    module_class, 
    work_path, 
    module_config, 
    logger_level,
    return_dict,
):
    """Run a module in a separate process

    Args:
        module_class (BaseModule): The module class to run
        work_path (str): Path to the unzipped submission directory
        module_config (dict): Configuration for the module
        return_dict (multiprocessing.Manager().dict): Dictionary to store the result
        logger_level (int, optional): Logging level for the module. Defaults to logging.INFO.
    """
    try:
        # Set up logging for the module
        module_logger = logging.getLogger(module_class.__name__)
        module_logger.setLevel(logger_level)

        # Set resource limits for the module
        timeout_sec = module_config.get("timeout_sec", DEFAULT_TIMEOUT_SEC)
        ram_limit_mb = module_config.get("ram_limit_mb", DEFAULT_RAM_LIMIT_MB)
        file_size_limit_mb = module_config.get("file_size_limit_mb", DEFAULT_FILE_SIZE_LIMIT_MB)
        num_proc_limit = module_config.get("num_proc_limit", DEFAULT_NUM_PROC_LIMIT)
        num_open_files_limit = module_config.get(
            "num_open_files_limit", 
            DEFAULT_NUM_OPEN_FILES_LIMIT
        )
        module_logger.debug(
            f"Setting resource limits for module {module_class.__name__}: "
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

        # Instantiate the module class and run it
        module_instance = module_class(
            work_path=work_path,
            config=module_config,
            logger=module_logger,
        )
        result = module_instance.run()
        return_dict['success'] = True
        return_dict['result'] = result

    except Exception as e:
        module_logger.error(f"Error running module {module_class.__name__}: {e}")
        return_dict['success'] = False
        return_dict['error'] = str(e)

class ModuleRunner:
    """Discovers, loads, and runs the course-specific grading modules"""

    def __init__(
        self, 
        config: dict, 
        config_dir_path: str, 
        work_path: str):
        """Initialize the module runner with the path to the modules

        Args:
            config (dict): Configuration dictionary containing module paths and settings
            config_dir_path (str): Path to the directory containing the configuration file
            work_path (str): Path to the unzipped submission directory

        Raises:
            ValueError: If the config or module path is invalid or not provided
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
        if not self._work_path.is_dir():
            raise ValueError(f"Work path is not a directory: {self._work_path}")

        # Load modules
        self._modules = {}
        self._load_modules()

    def _load_modules(self):
        """Load the course-specific grading module"""
        
        # Create list of complete module paths from config
        modules = self._config.get("modules", [])
        if not modules:
            raise ValueError("No modules specified in configuration")
        
        # Load each module specified in the configuration
        for module in modules:

            # Get the module name and config
            module_name = next(iter(module))
            module_config = module[module_name]
            logger.debug(f"Loading module: {module_name}")

            # Get absolute path to the module file
            if pathlib.Path(module_config["path"]).is_absolute():
                module_path = pathlib.Path(module_config["path"])
            else:
                module_path = self._config_dir_path / module_config["path"]
            module_path = module_path.resolve()
            logger.debug(f"Module path: {module_path}")
            
            try:
                # Import module from file
                spec = importlib.util.spec_from_file_location(
                    module_name, 
                    module_path,
                )
                if spec is None:
                    logger.warning(f"Could not create spec for {module_path}")
                    continue

                # Load the module
                module = importlib.util.module_from_spec(spec)
                if module is None:
                    logger.warning(f"Could not load module {module_path}")
                    continue
                spec.loader.exec_module(module)
                if module is None:
                    logger.warning(f"Module {module_path} is None after loading")
                    continue
                
                # Find BaseModule class that matches the "class" key in the config
                if "class" in module_config:
                    class_name = module_config["class"]
                    if hasattr(module, class_name):
                        module_class = getattr(module, class_name)
                        if isinstance(module_class, type) and issubclass(module_class, BaseModule):
                            self._modules[module_name] = module_class
                            logger.info(f"Discovered test module: {class_name}")
                        else:
                            logger.error(
                                f"{class_name} is not a valid BaseModule subclass in {module_path}"
                            )
                            continue
                    else:
                        logger.error(f"{class_name} not found in module {module_path}")
                        continue
            
            except Exception as e:
                logger.error(f"Failed to load module {module_path}: {e}")
                continue
        
        # Log loaded modules
        logger.info(f"Loaded {len(self._modules)} modules")
        logger.debug(f"Available modules: {list(self._modules.keys())}")
        if not self._modules:
            logger.warning("No modules loaded")

    def run_module(self, module_name: str, module_config: dict = None):
        """Run a specific module with the given submission path

        Args:
            module_name (str): Name of the module to run
            module_config (dict, optional): Configuration for the module.

        Returns:
            result: Result of the module's run method
        """

        # Validate module name
        if module_name not in self._modules:
            raise ValueError(f"Module {module_name} not found")
        
        # Validate module configuration
        if module_config is None:
            module_config = self._config.get("default_module_config", {})
        if not module_config:
            raise ValueError(f"No configuration provided for module {module_name}")
        
        # Get timeout
        timeout_sec = module_config.get("timeout_sec", DEFAULT_TIMEOUT_SEC)
        if timeout_sec <= 0:
            raise ValueError(f"Invalid timeout for module {module_name}: {timeout_sec} seconds")
        
        try:
            # Get the module class
            module_class = self._modules[module_name]
            
            # Set up multiprocessing to run the module
            manager = multiprocessing.Manager()
            return_dict = manager.dict()

            # Create a new process for the module
            process = multiprocessing.Process(
                target=run_module_process,
                args=(
                    module_class, 
                    self._work_path, 
                    module_config, 
                    logger.level,
                    return_dict,
                )
            )

            # Start the process
            process.start()
            process.join(timeout=timeout_sec)
            if process.is_alive():
                logger.warning(f"Module {module_name} timed out after {timeout_sec} seconds")
                process.terminate()
                return None
            
            # Check exit code
            if process.exitcode != 0:
                logger.error(f"Module {module_name} exited with code {process.exitcode}")
                return None
            
            # Check if the process completed successfully
            if return_dict.get('success'):
                result = return_dict.get('result')
                logger.debug(f"Module {module_name} succeeded with result: {result}")
            else:
                logger.error(f"Module {module_name} failed with error: {return_dict.get('error')}")
                result = None

        except Exception as e:
            logger.error(f"Error running module {module_name}: {e}")
            return None
        
        return result

class Autograder:
    """Main class for the autograder framework

    This class is responsible for loading the course configuration,
    running the grading process, and handling the output of results.
    """

    def __init__(
            self, 
            submission: str,
            config_path: str,
            output: str = "./",
            work_path: str = "./",
        ):
        """Initialize the autograder with optional configuration and module paths

        Args:
            submission (str): Path to the submission zip file
            config_path (str): Path to the configuration file (YAML format)
            output (str, optional): Path to output directory for results. 
                Defaults to current directory.
            work_path (str, optional): Path to the directory where the 
                submission is unzipped and compiled. Defaults to current directory.
        """

        # Set attributes from arguments
        self._config_path = pathlib.Path(config_path).resolve()
        self._output = output
        self._submission = submission
        self._work_path = work_path

        # Load configuration
        self._config = self._load_config()
        if not self._config:
            raise ValueError("Configuration could not be loaded")
        logging.debug(f"Configuration loaded: {self._config}")

        # Create runner
        self._module_runner = ModuleRunner(
            self._config, 
            self._config_path.parent, 
            self._work_path
        )

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
        
        # Run each module specified in the configuration
        for module in self._config.get("modules", []):
            module_name = next(iter(module))
            module_config = module[module_name]
            logging.info(f"Running module: {module_name}")
            logging.debug(f"Module config: {module_config}")
            try:
                result = self._module_runner.run_module(module_name, module_config)
                if result is not None:
                    logging.info(f"Module {module_name} completed successfully")
                else:
                    logging.error(f"Module {module_name} failed")
            except Exception as e:
                logging.error(f"Error running module {module_name}: {e}")
                continue
        
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
        
    def _copy_and_extract_submission(self):
        """Copy and extract the submission zip file

        Returns:
            pathlib.Path: Path to the extracted directory for grading
        """
        if not self._submission:
            raise ValueError("Submission path is required")

        # Create a temporary directory for the submission
        if self._work_path:
            temp_dir = pathlib.Path(self._work_path)
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
        "--output",
        "-o",
        required=True,
        type=str,
        help="Path to output directory for results",
    )
    parser.add_argument(
        "--submission",
        "-s",
        required=True,
        type=str,
        help="Path to the submission zip file",
    )
    parser.add_argument(
        "--work_dir",
        "-w",
        type=str,
        help="Path to the directory where the submission is unzipped and compiled",
    )
    
    # Parse arguments
    args = parser.parse_args()

    # Print welcome message
    logger.info(f"ByteGrader Autograder Framework v{__version__}")

    # If debug mode is enabled, set logging to DEBUG level
    if args.debug:
        logging.getLogger().setLevel(logging.DEBUG)
        logging.basicConfig(
            level=logging.DEBUG, 
            force=True,
            format="%(asctime)s [%(levelname)s]: %(message)s"
        )
        logger.debug("Debug mode enabled")

    # Run autograder
    autograder = Autograder(
        config_path=args.config,
        output=args.output,
        submission=args.submission,
        work_path=args.work_dir,
    )
    autograder.grade()

if __name__ == "__main__":
    main()
