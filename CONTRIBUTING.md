# Contributing to Niyantrak

Thank you for your interest in contributing to Niyantrak! We welcome contributions from the community to help improve this rate limiter library. This guide will help you get started.

## 📋 Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [How to Contribute](#how-to-contribute)
- [Coding Standards](#coding-standards)
- [Testing Guidelines](#testing-guidelines)
- [Commit Message Format](#commit-message-format)
- [Pull Request Process](#pull-request-process)
- [Reporting Issues](#reporting-issues)
- [Feature Requests](#feature-requests)

## Code of Conduct

We are committed to providing a welcoming and inclusive environment for all contributors. Please be respectful, considerate, and professional in all interactions.

**Expected Behavior:**
- Be respectful and inclusive
- Welcome newcomers and help them get started
- Focus on constructive criticism
- Support others in the community

**Unacceptable Behavior:**
- Harassment, discrimination, or exclusion
- Trolling or intentional disruption
- Violation of others' privacy or safety
- Any form of abusive language or behavior

Contributors who violate these standards may be temporarily or permanently banned from the project.

## Getting Started

### Prerequisites

- Go 1.19 or later
- Git for version control
- Basic familiarity with Go and rate limiting concepts

### Fork and Clone

1. Fork the repository on GitHub
2. Clone your fork locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/niyantrak.git
   cd niyantrak
   ```
3. Add upstream remote:
   ```bash
   git remote add upstream https://github.com/abhipray-cpu/niyantrak.git
   ```

## Development Setup

### 1. Install Development Tools

```bash
make setup-workspace
```

This installs all required development tools and verifies your environment.

### 2. Verify Setup

```bash
make check-env
make quick-check
```

### 3. Explore the Codebase

```bash
make info           # Project structure
make examples       # Available examples
make docs-server    # View documentation locally
```

## How to Contribute

### 1. Check Existing Issues

- Look at [GitHub Issues](https://github.com/abhipray-cpu/niyantrak/issues) for existing work
- Comment if you want to work on something
- Discuss the approach before starting

### 2. Create a Feature Branch

```bash
git checkout -b feature/my-feature
# or for bug fixes:
git checkout -b fix/my-fix
```

Branch naming conventions:
- `feature/description` - New features
- `fix/description` - Bug fixes
- `docs/description` - Documentation
- `test/description` - Test additions
- `perf/description` - Performance improvements

### 3. Make Your Changes

```bash
# Make your changes to the code

# Run tests frequently
make quick-check

# Watch tests auto-run
make watch
```

### 4. Run Quality Checks

Before committing, run the full quality suite:

```bash
# Quick validation
make pre-commit

# Full validation
make validate

# Even more comprehensive
make all
```

### 5. Commit and Push

```bash
git add .
git commit -m "Feature: description of your change"
git push origin feature/my-feature
```

### 6. Create a Pull Request

- Go to GitHub and create a PR from your fork to main repo
- Fill in the PR template completely
- Link related issues
- Wait for review and CI checks

## Coding Standards

### Go Code Style

We follow standard Go conventions:

```bash
# Format code
make fmt

# Run linters
make lint

# Static analysis
make vet
make staticcheck
```

### Code Organization

**Packages:**
- `algorithm/` - Rate limiting algorithms
- `backend/` - Storage backends
- `limiters/` - Limiter implementations
- `middleware/` - HTTP/gRPC middleware
- `observability/` - Logging, metrics, tracing
- `features/` - Advanced features
- `config/` - Configuration structures
- `examples/` - Example implementations

**Naming Conventions:**
- Exported identifiers are PascalCase: `TokenBucket`, `Allow()`
- Unexported identifiers are camelCase: `newTokenBucket()`, `allow()`
- Interface names end with "er": `Limiter`, `Backend`, `Logger`
- Boolean getters use "Is" or "Has": `IsEnabled()`, `HasError()`

### Comments

- Exported functions must have docstrings
- Complex logic should be commented
- Docstrings start with the function name
- Examples in comments are helpful

```go
// Allow checks if request is allowed and updates state
func (l *Limiter) Allow(ctx context.Context, key string) Result {
    // Implementation...
}
```

### Error Handling

- Always return errors; don't ignore them
- Use wrapped errors with `fmt.Errorf("context: %w", err)`
- Create specific error types for library errors
- Provide helpful error messages

```go
if err != nil {
    return fmt.Errorf("failed to initialize limiter: %w", err)
}
```

## Testing Guidelines

### Running Tests

```bash
# Run all tests
make test

# Run with race detector
make test-race

# Run with coverage
make coverage

# View coverage
make coverage-report
```

### Writing Tests

1. **Test file naming**: `filename_test.go`
2. **Test function naming**: `TestFunctionName(t *testing.T)`
3. **Use table-driven tests** for multiple cases:

```go
func TestAllow(t *testing.T) {
    tests := []struct {
        name     string
        key      string
        expected bool
    }{
        {"allowed request", "user:1", true},
        {"denied request", "user:2", false},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := limiter.Allow(context.Background(), tt.key)
            if result.Allowed != tt.expected {
                t.Errorf("got %v, want %v", result.Allowed, tt.expected)
            }
        })
    }
}
```

### Coverage Requirements

- Minimum 80% code coverage
- All public APIs must be tested
- Edge cases should be covered
- Run `make coverage-report` to verify

## Commit Message Format

### Format

```
<type>: <subject>

<body>

<footer>
```

### Types

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `test`: Test addition or update
- `perf`: Performance improvement
- `refactor`: Code refactoring
- `style`: Code style (formatting, missing semicolons, etc.)
- `chore`: Build, dependencies, release tasks

### Examples

```
feat: Add token bucket with dynamic refill rate

Implements token bucket algorithm with support for
adjusting refill rate at runtime. Includes proper
concurrency handling and metrics.

Fixes #123
```

```
fix: Race condition in memory backend

Fixed data race when multiple goroutines access
the same key simultaneously by adding proper locking.

Fixes #456
```

## Pull Request Process

### PR Guidelines

1. **Keep it focused** - One feature or fix per PR
2. **Descriptive title** - Clear what the PR does
3. **Fill PR template** - Include all requested info
4. **Link issues** - Reference related GitHub issues
5. **Document changes** - Update docs if needed

### Before Submitting

- [ ] Code follows style guidelines (`make lint`)
- [ ] All tests pass (`make test`)
- [ ] Race detector passes (`make test-race`)
- [ ] Coverage maintained (`make coverage`)
- [ ] Examples run (`make test-examples`)
- [ ] Commit messages follow format

### Review Process

1. At least one maintainer review required
2. CI checks must pass
3. Discussion may be needed
4. Commits should be squashed when merging
5. Rebase on main if needed

### After Approval

- Maintainers will merge when ready
- CI pipeline validates the merge
- Code is automatically deployed

## Reporting Issues

### Before Reporting

- Search existing issues for similar problems
- Check closed issues in case it's already fixed
- Verify it's not a usage question (use Discussions instead)

### Issue Template

```markdown
## Description
Clear description of the issue

## Steps to Reproduce
1. Step 1
2. Step 2
3. Step 3

## Expected Behavior
What should happen

## Actual Behavior
What actually happens

## Environment
- Go version: 
- OS: 
- Niyantrak version:

## Logs/Screenshots
Include error messages or screenshots
```

### Good Bug Reports Include

- Clear title and description
- Reproducible steps
- Expected vs actual behavior
- Environment details
- Relevant code/logs
- Error messages

## Feature Requests

### Propose New Features

1. **Search first** - Check if similar features exist
2. **Create issue** - Use "Feature Request" template
3. **Describe use case** - Why is this needed?
4. **Provide examples** - How would it be used?
5. **Discuss** - Get feedback before implementing

### Feature Template

```markdown
## Summary
Brief description of the feature

## Motivation
Why is this feature needed?

## Proposed Solution
How would it work?

## Example Usage
Code showing how it would be used

## Alternatives
Other approaches considered
```

## Development Workflow

### Daily Development

```bash
# Terminal 1 - Auto-test on changes
make watch

# Terminal 2 - Work on code
# Edit files...

# Terminal 3 - Run specific example
make run-example NUM=01
```

### Before Each Commit

```bash
make pre-commit  # Format + tidy + lint
make validate    # Full validation
```

### Before Each Push

```bash
make ci-check    # Run CI checks locally
```

### Common Tasks

```bash
# View documentation locally
make docs-server

# Run all examples
make run-examples

# Generate coverage report
make coverage

# Profile performance
make benchmarks

# Debug specific test
make debug-test
```

## Getting Help

- **Questions**: Use GitHub Discussions
- **Issues**: File a GitHub Issue
- **Chat**: Check project README for community channels
- **Documentation**: Read docs locally with `make docs-server`

## Release Process

Only maintainers can create releases. The process:

1. Update version numbers
2. Update CHANGELOG
3. Create git tag
4. Push to GitHub
5. Automated CI creates release
6. Published to package repository

## License

By contributing to Niyantrak, you agree that your contributions will be licensed under its MIT License. See [LICENSE](LICENSE) for details.

## Recognition

Contributors are recognized in:
- CONTRIBUTORS.md (for significant contributions)
- Release notes (for features/fixes)
- GitHub contributor page

## Additional Resources

- **Main Repository**: https://github.com/abhipray-cpu/niyantrak
- **Documentation**: See `README.md` and examples
- **Architecture**: See `docs/architecture-diagram.md`
- **Examples**: See `examples/` directory
- **Makefile**: Run `make help` for available commands

## Questions?

- Check existing issues for similar questions
- Create a GitHub Discussion
- Comment on related issues
- Review documentation and examples

---

Thank you for contributing to Niyantrak! Your efforts help make this library better for everyone. 🙏
