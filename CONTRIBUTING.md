# Contributing to maestro-runner

Thank you for your interest in contributing! This document provides guidelines for contributing to the project.

## Code of Conduct

Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md).

## Getting Started

See [README.md](README.md) for installation and setup instructions.

```bash
# Run tests
make test

# Run all checks (lint, vet, test)
make check
```

## How to Contribute

### Reporting Bugs

1. Check existing [issues](https://github.com/devicelab-dev/maestro-runner/issues) to avoid duplicates
2. Use the bug report template
3. Include:
   - Go version (`go version`)
   - OS and version
   - Steps to reproduce
   - Expected vs actual behavior
   - Relevant flow files (anonymized)

### Suggesting Features

1. Check existing issues and discussions
2. Use the feature request template
3. Explain the use case and expected behavior

### Pull Requests

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes
4. Run checks: `make check`
5. Commit with clear messages
6. Push and create a Pull Request

## Development Guidelines

### Code Style

- Follow standard Go conventions
- Run `make check` before committing
- Keep functions small and focused
- Add comments for exported types and functions

### Commit Messages

Use clear, descriptive commit messages:

```
Add support for swipe gestures

- Implement SwipeStep parsing
- Add swipe command to Appium driver
- Add tests for swipe directions
```

### Testing

- Write tests for new functionality
- Maintain or improve code coverage

### Documentation

- Update relevant documentation for changes
- Add godoc comments for exported APIs
- Update CHANGELOG.md for notable changes

## Project Structure

See [DEVELOPER.md](DEVELOPER.md) for architecture details and extension guides.

## Review Process

1. All PRs require at least one review
2. CI must pass (lint, test, build)
3. New features need tests
4. Breaking changes need discussion first

## Getting Help

- Open an issue for questions
- Tag maintainers if blocked

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
