"""Setup script for ByteGrader project."""

from setuptools import setup, find_packages

setup(
    name="bytegrader",
    version="0.1.0",
    description="Python-based autograder for student submissions",
    packages=find_packages(where="src"),
    package_dir={"": "src"},
    python_requires=">=3.8",
    install_requires=[
        # Add your dependencies here as you identify them
        # "docker",
        # "pyyaml",
        # "pytest",
    ],
    extras_require={
        "dev": [
            "pytest",
            "pytest-cov",
            "black",
            "flake8",
        ]
    },
    entry_points={
        "console_scripts": [
            "bytegrader=bytegrader.bytegrader:main",
        ],
    },
)
