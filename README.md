# Feed SDK

[![Tests](https://github.com/A-pen-app/feed-sdk/actions/workflows/test.yml/badge.svg)](https://github.com/A-pen-app/feed-sdk/actions/workflows/test.yml)
[![Lint](https://github.com/A-pen-app/feed-sdk/actions/workflows/lint.yml/badge.svg)](https://github.com/A-pen-app/feed-sdk/actions/workflows/lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/A-pen-app/feed-sdk)](https://goreportcard.com/report/github.com/A-pen-app/feed-sdk)

A Go SDK for managing content feeds with policy-based filtering and positioning.

## Installation

```bash
go get github.com/A-pen-app/feed-sdk
```

## Features

- Feed aggregation and sorting based on scoring
- Policy enforcement for feed visibility
- Feed positioning and reordering
- Policy violation detection to filter feeds
- Database persistence for feed policies

## Usage

### Basic Setup

```go
import (
    "github.com/A-pen-app/feed-sdk/model"
    "github.com/A-pen-app/feed-sdk/service"
    "github.com/A-pen-app/feed-sdk/store"
    "github.com/jmoiron/sqlx"
)

// Your data type must implement model.Scorable interface
type Post struct {
    ID    string
    Title string
    score float64
}

func (p Post) GetID() string           { return p.ID }
func (p Post) Feedtype() model.FeedType { return model.TypePost }
func (p Post) Score() float64          { return p.score }

// Initialize the service
db := sqlx.MustConnect("postgres", "postgresql://user:pass@host/db")
feedStore := store.NewFeed(db)
feedService := service.NewFeed[Post](feedStore)
```

### Get Sorted Feeds

```go
posts := []Post{
    {ID: "1", Title: "First", score: 10.0},
    {ID: "2", Title: "Second", score: 20.0},
}

// Returns feeds sorted by score (descending), with positioned feeds placed accordingly
feeds, err := feedService.GetFeeds(ctx, posts)
```

### Position Feeds

```go
// Pin a feed to position 0
err := feedService.PatchFeed(ctx, "post123", model.TypePost, 0)

// Get all positions (for displaying available slots)
positions, err := feedService.GetPolicies(ctx, 10)

// Remove a feed from its position
err := feedService.DeleteFeed(ctx, "post123")
```

### Policy Enforcement

To enforce policies, implement the `PolicyResolver` interface:

```go
type PolicyResolver interface {
    GetPostViewCount(ctx context.Context, postID string, uniqueUser bool, duration int64, targetUserId string) (int64, error)
    GetUserAttribute(ctx context.Context, userID string) ([]string, error)
}
```

Then build a violation map:

```go
violations := feedService.BuildPolicyViolationMap(ctx, userID, policyMap, resolver)
// violations is map[feedID]violatedPolicy - feeds in the map should be filtered out
```

## Policy Types

The SDK supports the following policy types for controlling feed visibility:

| Policy | Format | Description |
|--------|--------|-------------|
| `exposure` | `exposure-{limit}[-distinct][-duration-{seconds}][-istheone]` | Limits total view count. Optional `distinct` for unique users, `duration` for time window, `istheone` for targeting a specific user. |
| `inexpose` | `inexpose-{timestamp}` | Feed becomes visible after the specified Unix timestamp |
| `unexpose` | `unexpose-{timestamp}` | Feed becomes hidden after the specified Unix timestamp |
| `istarget` | `istarget-{attribute}` | Feed is only visible to users with the specified attribute |

### Policy Examples

```
exposure-1000                         # Max 1000 total views
exposure-1000-distinct                # Max 1000 unique users
exposure-1000-distinct-duration-3600  # Max 1000 unique users per hour
inexpose-1735689600                   # Hidden until Jan 1, 2025
unexpose-1735689600                   # Visible until Jan 1, 2025
istarget-premium                      # Only for users with "premium" attribute
```

### Helper Policies

These are used as modifiers for the `exposure` policy:

- `distinct` - Counts unique users instead of total views
- `duration` - Specifies a time window in seconds for counting views
- `istheone` - Targets view count for a specific user (uses the current user's ID)

## Database Schema

The SDK expects a `feed` table:

```sql
CREATE TABLE feed (
    feed_id   VARCHAR PRIMARY KEY,
    feed_type VARCHAR,
    position  INTEGER,
    policies  TEXT[] DEFAULT ARRAY[]::TEXT[]
);
```

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

Current test coverage: model 69.7%, service 97.8%, store 100%

## CI/CD

This project uses GitHub Actions for continuous integration:

- **Tests**: Automatically run on push to `main` and on pull requests
- **Lint**: Code quality checks using golangci-lint
- **Coverage**: Minimum 80% coverage required to pass

## TODO

- auto create feed table if not exist already