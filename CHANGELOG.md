# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.8.0] - 2025-06-22

### Added
- Initial prototype version of ByteGrader
- Common entrypoint system for graders
- Common grading result structure with dataclass validation
- Test-stub grader for basic functionality testing
- Make/C grader for compiling and testing C programs with multiple test cases
- Docker-based grading environment with security isolation
- Assignment registry system (YAML) for managing grader configurations
- Rate limiting and IP whitelisting for API security
- Zip bomb protection and path traversal prevention
- Volume-based file handling for efficient container operations
- Comprehensive API endpoints (/submit, /status, /queue, /health)
- DigitalOcean deployment guide and server setup scripts
- SSL/TLS configuration with Let's Encrypt integration

### Security
- Secure file extraction with size limits and compression ratio checks
- Container isolation for student code execution
- API key authentication system
- IP address whitelisting capabilities
