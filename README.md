# ByteGrader Autograder Framework

<img src=".images/bytegrader-logo-text_2000px.png" alt="ByteGrader logo" width="1000" />

**ByteGrader** is an open-source, modular autograder designed to evaluate programming assignments in embedded systems, IoT, and edge AI. Built to support a variety of programming languages, it uses containerized environments to reliably compile, run, and assess student submissions.

This project was created to streamline the grading process for technical coursework, offering:
- Flexible assignment configurations
- Support for custom test scripts and code pattern checks
- Reproducible Docker-based environments for isolation and consistency

While ByteGrader is optimized for embedded systems and hardware-centric courses, its architecture is general enough to be extended to other domains and languages.

## Key Features

- **Language-agnostic grading engine** (Python, C, etc.)
- **Docker-based execution** to isolate and reproduce environments
- **Pluggable modules** for different courses or assignments
- **Code analysis tools** to verify API usage, function calls, or design patterns

## Example Use Cases

- Grading STM32 or ESP32 firmware assignments for embedded systems courses
- Evaluating IoT device data parsing and MQTT communication tasks
- Verifying Python scripts for edge AI inference pipelines
- Ensuring students follow proper function use and coding standards

## Quick Start

_Coming soon…_

## Todo

 * Create Arduino example
 * Make multi-stage Docker build for adding in environments (e.g. Arduino, ESP-IDF)
 * Build API for hosting site and LearnDash
 * Build loading scripts for hosting
 * Add pattern checking (i.e. did student call a particular function)
 * Add schema type checking for YAML config files
 * Set up GitHub actions for continuous testing (start with basic integration tests)
 * Construct unit tests

## License

All code, unless otherwise specified, is subject to the [3-Clause BSD License](https://opensource.org/license/bsd-3-clause).

> Copyright 2025 Shawn Hymel
>
> Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:
>
> 1. Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.
>
> 2. Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or other materials provided with the distribution.
>
>3. Neither the name of the copyright holder nor the names of its contributors may be used to endorse or promote products derived from this software without specific prior written permission.
>
>THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS “AS IS” AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
