# Feed SDK

[![Tests](https://github.com/A-pen-app/feed-sdk/actions/workflows/test.yml/badge.svg)](https://github.com/A-pen-app/feed-sdk/actions/workflows/test.yml)
[![Lint](https://github.com/A-pen-app/feed-sdk/actions/workflows/lint.yml/badge.svg)](https://github.com/A-pen-app/feed-sdk/actions/workflows/lint.yml)

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
- Feed relations for linking related content
- Auto-creation of database tables on initialization

## Feed Types

The SDK supports the following feed types:

| Type | Constant | Description |
|------|----------|-------------|
| `post` | `model.TypePost` | Single post content |
| `posts` | `model.TypePosts` | Collection of posts |
| `banners` | `model.TypeBanners` | Banner content |
| `chat` | `model.TypeChat` | Chat content |

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

func (p Post) GetID() string            { return p.ID }
func (p Post) Feedtype() model.FeedType { return model.TypePost }
func (p Post) Score() float64           { return p.score }

// Initialize the service
db := sqlx.MustConnect("postgres", "postgresql://user:pass@host/db")
feedStore := store.NewFeed(db)  // auto-creates feed and feed_relation tables
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

### Feed Relations

```go
// Get related feeds for a given feed
relatedFeedIDs, err := feedService.GetRelatedFeeds(ctx, "post123")
```

### Policy Enforcement

To enforce policies, implement the `PolicyResolver` interface:

```go
type PolicyResolver interface {
    GetPostViewCount(ctx context.Context, postID string, uniqueUser bool, duration int64) (int64, error)
    GetViewerPostViewCount(ctx context.Context, postID, userID string) (int64, error)
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
| `exposure` | `exposure:{limit}[:distinct][:duration:{seconds}]` | Limits total view count. Optional `distinct` for unique users, `duration` for time window. |
| `istheone` | `istheone:{limit}:{userId}` | Limits view count for a specific user. |
| `inexpose` | `inexpose:{timestamp}` | Feed becomes visible after the specified Unix timestamp |
| `unexpose` | `unexpose:{timestamp}` | Feed becomes hidden after the specified Unix timestamp |
| `istarget` | `istarget:{attribute}` | Feed is only visible to users with the specified attribute |

### Policy Examples

```
exposure:1000                              # Max 1000 total views
exposure:1000:distinct                     # Max 1000 unique users
exposure:1000:distinct:duration:3600       # Max 1000 unique users per hour
istheone:5:user123                         # Max 5 views for user "user123"
inexpose:1735689600                        # Hidden until Jan 1, 2025
unexpose:1735689600                        # Visible until Jan 1, 2025
istarget:premium                           # Only for users with "premium" attribute
```

### Helper Policies

These are used as modifiers for the `exposure` policy:

- `distinct` - Counts unique users instead of total views
- `duration` - Specifies a time window in seconds for counting views

## Database Schema

The SDK automatically creates the required tables on initialization. The schemas are:

### Feed Table

```sql
CREATE TABLE IF NOT EXISTS feed (
    feed_id uuid NOT NULL,
    position integer NOT NULL DEFAULT 0,
    feed_type character varying(20) NOT NULL DEFAULT 'banners'::character varying,
    policies character varying(200)[] NOT NULL DEFAULT ARRAY[]::character varying[],
    CONSTRAINT feed_pkey PRIMARY KEY (feed_id),
    CONSTRAINT feed_position_position1_key UNIQUE (position) INCLUDE (position)
);
```

A trigger validates policy format on insert/update, ensuring policies match the pattern `{policy_type}:{params}` where params can contain lowercase letters, numbers, colons, underscores, and hyphens.

### Feed Relation Table

```sql
CREATE TABLE IF NOT EXISTS feed_relation (
    feed_id uuid NOT NULL,
    related_feed_id uuid NOT NULL,
    CONSTRAINT feed_relation_pkey PRIMARY KEY (feed_id, related_feed_id),
    CONSTRAINT feed_relation_feed_id_fkey FOREIGN KEY (feed_id) REFERENCES feed(feed_id) ON DELETE CASCADE,
    CONSTRAINT feed_relation_related_feed_id_fkey FOREIGN KEY (related_feed_id) REFERENCES feed(feed_id) ON DELETE CASCADE
);
```

### Feed Changelog Table

The SDK automatically tracks all changes to feeds in a changelog table:

```sql
CREATE TABLE IF NOT EXISTS feed_changelog (
    id SERIAL PRIMARY KEY,
    feed_id uuid NOT NULL,
    change_type character varying(20) NOT NULL,
    old_feed_type character varying(20),
    new_feed_type character varying(20),
    old_position integer,
    new_position integer,
    old_policies character varying(200)[],
    new_policies character varying(200)[],
    changed_at timestamp with time zone NOT NULL DEFAULT NOW()
);
```

Change types tracked:
- `INSERT` - New feed created
- `DELETE` - Feed deleted
- `UPDATE` - Feed type or position changed
- `POLICY_ADD` - Policy added to feed
- `POLICY_DELETE` - Policy removed from feed
- `POLICY_MODIFY` - Policy modified (same count, different content)

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
