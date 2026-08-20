# Changelog

All notable changes to this project will be documented in this file.

> The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

When making changes, add an entry to the **Unreleased** section.

## [0.8.5] - 2026-08-20

### Changed

- Added a shared `graders` group (GID 1800) for secure cross-container file access
- API server container now starts as root, sets workspace permissions, then drops to app user via `gosu`
- Workspace directories are created with mode `0775` (group-writable)
- Volume initialization sets setgid bit on `/workspace/jobs/` so new job subdirectories inherit the `graders` group
- Removed `grader_uid`/`grader_gid` fields from registry.yaml and the `AssignmentConfig` struct (no longer needed)
- Removed `user:` override and `no-new-privileges` from docker-compose.yaml
- Updated the main README with information on how to use the new `install.sh` installation process

### Added

- `install.sh` and `config.yaml` files to allow for streamlined installation and updates
- `deploy/entrypoint.sh`: sets up workspace group ownership and setgid bit as root, then execs the API server as the
app user
- `gosu` installed in API server image for clean privilege drop

### Security

- Grader containers no longer require world-writable (`777`) directories; write access is scoped to the `graders` group

## [0.8.4] - 2025-07-17

### Changed

- Updated resource limits in docker-compose.yaml (optimized for 2 vCPU, 2 GB RAM)
- Environment variables defined in registry.yaml can now be passed in to images
- Allow for /assignment/grader.py and /assignments/{GRADER_ASSIGNMENT}/grader.py (one base image for multiple assignments)

## [0.8.3] - 2025-07-11

### Changed

- Grader main script now unzips to temporary directory instead of to the shared volume

## [0.8.2] - 2025-07-10

### Changed

- Allow only one submission per username per assignment

## [0.8.1] - 2025-07-09

### Added

- Added CONTRIBUTING.md and docs/release.md for notes on how to contribute
- Added a "delay" test that waits for some time before returning a score
- Added a new "admin" endpoint category that requires an API key but no username

### Changed

- Moved /config and /version endpoints to the admin category

### Removed

- Removed main domain from certbot script (only the subdomain gets cert renewal)

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
