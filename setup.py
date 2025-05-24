"""Setup script for ByteGrader project."""

from setuptools import setup, find_packages
import os

# Read version from __init__.py
def get_version():
    init_file = os.path.join(os.path.dirname(__file__), 'src', 'bytegrader', '__init__.py')
    with open(init_file, 'r') as f:
        for line in f:
            if line.startswith('__version__'):
                return line.split('=')[1].strip().strip('"').strip("'")
    raise RuntimeError('Unable to find version string.')

setup(
    name="bytegrader",
    version=get_version(),
    description="Python-based autograder for student submissions",
    packages=find_packages(where="src"),
    package_dir={"": "src"},
    python_requires=">=3.8",
    install_requires=[
        "pyyaml>=5.4.1",
    ],
    extras_require={
        "dev": [
            # "pytest",
            # "pytest-cov",
            # "black",
            # "flake8",
        ]
    },
    entry_points={
        "console_scripts": [
            "bytegrader=bytegrader.bytegrader:main",
        ],
    },
)
