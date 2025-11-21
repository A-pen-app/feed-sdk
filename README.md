# Feed SDK

[![Tests](https://github.com/A-pen-app/feed-sdk/actions/workflows/test.yml/badge.svg)](https://github.com/A-pen-app/feed-sdk/actions/workflows/test.yml)
[![Lint](https://github.com/A-pen-app/feed-sdk/actions/workflows/lint.yml/badge.svg)](https://github.com/A-pen-app/feed-sdk/actions/workflows/lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/A-pen-app/feed-sdk)](https://goreportcard.com/report/github.com/A-pen-app/feed-sdk)

A Go SDK for managing content feeds with policy-based filtering and positioning.

## Features

- Feed aggregation and sorting based on scoring
- Policy enforcement for feed visibility (exposure, inexposure, unexposure)
- Feed positioning and reordering
- Policy violation detection to filter feeds
- Database persistence for feed policies

## Testing

Run the unit tests:

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test ./... -cover

# Run tests with verbose output
go test ./... -v

# Generate coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

Current test coverage: **98.8%**

## CI/CD

This project uses GitHub Actions for continuous integration:

- **Tests**: Automatically run on push to `main` and on pull requests
- **Lint**: Code quality checks using golangci-lint
- **Coverage**: Minimum 80% coverage required to pass

## TODO

- auto create feed table if not exist already