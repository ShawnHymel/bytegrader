# Contributing to ByteGrader

Thank you for your interest in contributing to ByteGrader! This document provides guidelines and instructions for contributing to the project.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Project Structure](#project-structure)
- [Development Environment](#development-environment)
- [Testing](#testing)
- [Code Standards](#code-standards)
- [Pull Request Process](#pull-request-process)
- [Issue Guidelines](#issue-guidelines)
- [Security](#security)

## Getting Started

### Prerequisites

Before you begin, ensure you have the following installed (locally or in a VM/container):

- **Git** - Version control
- **Docker** - For containerized development and testing
- **Go 1.24+** - For server development
- **Python 3.12+** - For grader script development
- **Make** - For build automation (optional but recommended)

### Fork and Clone

Since the main branch is protected, you'll need to fork the repository and work on feature branches:

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/bytegrader.git
   cd bytegrader
   ```
3. **Add the upstream remote**:
   ```bash
   git remote add upstream https://github.com/ShawnHymel/bytegrader.git
   ```

## Development Workflow

### Branch Strategy

1. **Keep your fork updated**:
   ```bash
   git fetch upstream
   git checkout main
   git merge upstream/main
   git push origin main
   ```

2. **Create a feature branch**:
   ```bash
   git checkout -b your-feature-name
   ```

### Commit Guidelines

- Use clear, descriptive commit messages
- Start with a verb in present tense (Add, Fix, Update, Remove)
- Keep the first line under 50 characters
- Include a detailed description if needed

**Examples:**
```
Add rate limiting to API endpoints

Implements configurable rate limiting per IP address and username
to prevent abuse and ensure fair resource usage.

- Add RateLimitManager with configurable windows
- Update middleware to check limits before processing
- Add rate limit configuration to environment variables
```

## Project Structure

```
bytegrader/
├── server/          # Go API server
│   ├── main.go      # Main server entry point
│   ├── handlers.go  # HTTP route handlers
│   ├── auth.go      # Authentication middleware
│   ├── config.go    # Configuration management
│   └── ...
├── graders/         # Grading environments
│   ├── main.py      # Universal entrypoint
│   ├── result.py    # Common result structure
│   ├── registry.yaml # Assignment configuration
│   ├── test-stub/   # Example grader
│   └── make-c-add/  # C programming grader
├── deploy/          # Deployment scripts and configs
├── doc/             # Documentation
└── test/            # Test submissions and data
```

### Key Components

- **Server**: Go-based API server handling HTTP requests, job queuing, and container orchestration
- **Graders**: Docker-based environments for executing and grading student code
- **Registry**: YAML configuration defining available assignments and their settings (*graders/registry.yaml*)
- **Deploy**: Scripts and configurations for server deployment

## Development Environment

### Local Development Setup

1. **Install dependencies**:
   ```bash
   # For Go server development
   cd server
   go mod download
   ```

2. **Build and test locally**:
   ```bash
   # Quick syntax check
   docker run --rm -v "$PWD/server":/app -w /app golang:1.24 go build -o /dev/null .
   
   # Build grader images
   cd graders
   chmod +x build.sh
   ./build.sh
   ```

3. **Test a grader locally**:
   ```bash
   cd graders
   docker build -t bytegrader-test-stub -f test-stub/Dockerfile .
   cd ..
   mkdir -p test/results/
   docker run --rm \
     -v "$(pwd)/test/make-c-add/submission.zip:/submission/submission.zip:ro" \
     -v "$(pwd)/test/results/:/results" \
     bytegrader-test-stub
   ```

### Environment Variables

For local testing, create a `.env` file (not committed) with:

```bash
# Security settings
REQUIRE_API_KEY=false
ALLOWED_IPS=127.0.0.1,::1

# Resource limits
MAX_FILE_SIZE_MB=10
GRADING_TIMEOUT_MIN=5
MAX_CONCURRENT_JOBS=1

# Rate limiting (for testing)
RATE_LIMIT_ENABLED=false
```

## Testing

### Test Categories

1. **Unit Tests** - Test individual functions and components
2. **Integration Tests** - Test API endpoints and grader interactions
3. **Grader Tests** - Test specific grading logic with sample submissions

### Running Tests

```bash
# Test Go server syntax
docker run --rm -v "$PWD/server":/app -w /app golang:1.24 go build -o /dev/null .

# Test grader builds
cd graders && ./build.sh

# Test sample submissions
# (Add specific test commands for your graders)
```

### Test Submissions

When developing graders, include test submissions in `test/your-grader-name/`:

```
test/your-grader-name/
├── passing-submission.zip     # Should get high score
├── failing-submission.zip     # Should get low score
├── invalid-submission.zip     # Should produce error
└── expected-results.json      # Expected grading outputs
```

## Code Standards

### Go Code

- Follow standard Go formatting (`gofmt`)
- Use meaningful variable names
- Include comments for exported functions
- Handle errors appropriately
- Use structured logging

### Python Code

- Follow PEP 8 style guidelines
- Use type hints where helpful
- Include docstrings for functions
- Handle exceptions gracefully
- Use the `GradingResult` dataclass for all outputs

### Docker

- Use multi-stage builds when possible
- Minimize image size
- Run containers as non-root users
- Include health checks
- Clean up temporary files

### Documentation

- Keep README.md updated
- Document new environment variables
- Include examples in code comments
- Update API documentation for new endpoints

## Pull Request Process

### Before Submitting

1. **Test your changes locally**
2. **Update documentation** if needed
3. **Add tests** for new functionality
4. **Check code formatting**
5. **Update CHANGELOG.md** with your changes

### Updating CHANGELOG.md

The changelog format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). When making changes, add an entry to the **Unreleased** section of CHANGELOG.md:

```markdown
## [Unreleased]

### Added
- Your new feature description

### Fixed  
- Your bug fix description

### Changed
- Your modification description

### Removed
- What you removed
```

### Submitting a Pull Request

1. **Push your branch** to your fork:
   ```bash
   git push origin feature/your-feature-name
   ```

2. **Create a pull request** on GitHub with:
   - Clear title describing the change
   - Detailed description of what was changed and why
   - Link to any related issues
   - Screenshots or examples if applicable

3. **PR Requirements**:
   - All automated tests must pass
   - At least 1 reviewer approval required
   - No merge conflicts with main branch
   - Documentation updated if needed

### PR Template

```markdown
## Description
Brief description of changes made.

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update
- [ ] New grader

## Testing
- [ ] Tested locally
- [ ] Added/updated tests
- [ ] All existing tests pass

## Checklist
- [ ] Code follows project style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] CHANGELOG.md updated
```

## Issue Guidelines

### Reporting Bugs

Include:
- Clear description of the issue
- Steps to reproduce
- Expected vs actual behavior
- Environment details (OS, Docker version, etc.)
- Relevant logs or error messages

### Feature Requests

Include:
- Clear description of the proposed feature
- Use case and motivation
- Possible implementation approach
- Any breaking changes

### Grader Requests

Include:
- Programming language/framework
- Assignment requirements
- Sample input/output
- Grading criteria
- Resource requirements

## Security

### Security Issues

- **Do NOT** open public issues for security vulnerabilities
- Email (or contact via social media) the security issues to the maintainer
- Include details about the vulnerability
- Allow time for fixes before public disclosure

### Security Guidelines

- Never commit API keys, passwords, or certificates
- Use environment variables for sensitive configuration
- Validate all user inputs
- Use least-privilege principles
- Keep dependencies updated

### Safe Development

- Test with non-privileged users
- Use IP whitelisting in production
- Enable API key authentication
- Monitor for suspicious activity
- Use secure defaults

## Getting Help

- **Documentation**: Check existing docs in `/doc` folder
- **Issues**: Search existing issues before creating new ones
- **Discussions**: Use GitHub Discussions for questions
- **Examples**: Look at existing graders for implementation patterns

## Code of Conduct

This project adheres to a code of conduct that ensures a welcoming environment for all contributors. Be respectful, constructive, and collaborative in all interactions.
