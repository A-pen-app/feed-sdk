package store

import (
	"context"

	"github.com/A-pen-app/feed-sdk/model"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// Row direction: (feed_id = the candidate/attached post, related_feed_id = the
// pool marker or head occupying the slot). Every writer uses this orientation —
// AddRelationWithPolicies, both promotion lookups in DeleteFeed/
// DeleteFeedPosition — and the FK sits on the related side because only the
// slot occupant is a feed row; candidates are bare post ids.
//
// weight drives pool selection (see service.PickFromPool): candidates split
// traffic in proportion to weight, and weight <= 0 means paused — the row is
// kept but never shown.
const createFeedRelationTableSQL = `
CREATE TABLE IF NOT EXISTS feed_relation (
	feed_id uuid NOT NULL,
	related_feed_id uuid NOT NULL,
	policies text[] NOT NULL DEFAULT ARRAY[]::text[],
	weight int NOT NULL DEFAULT 100,
	CONSTRAINT feed_relation_pkey PRIMARY KEY (feed_id, related_feed_id),
	CONSTRAINT feed_relation_related_feed_id_fkey FOREIGN KEY (related_feed_id) REFERENCES feed(feed_id) ON DELETE CASCADE
)`

// addRelationWeightColumnSQL brings databases created before the weight column
// existed up to the current shape. ADD COLUMN IF NOT EXISTS makes a second run
// a no-op, and DEFAULT 100 gives pre-existing rows equal weight, which is the
// neutral choice: with every candidate at the same weight the draw degrades to
// an even split.
const addRelationWeightColumnSQL = `
ALTER TABLE feed_relation ADD COLUMN IF NOT EXISTS weight int NOT NULL DEFAULT 100`

func (s *store) AddRelation(ctx context.Context, feedID, relatedFeedID string) error {
	_, err := s.db.NamedExecContext(ctx,
		`
		INSERT INTO feed_relation (feed_id, related_feed_id)
		VALUES (:feed_id, :related_feed_id)
		ON CONFLICT (feed_id, related_feed_id) DO NOTHING
		`,
		map[string]interface{}{
			"feed_id":         feedID,
			"related_feed_id": relatedFeedID,
		})
	return err
}

func (s *store) RemoveRelation(ctx context.Context, feedID, relatedFeedID string) error {
	_, err := s.db.NamedExecContext(ctx,
		`
		DELETE FROM feed_relation
		WHERE feed_id = :feed_id AND related_feed_id = :related_feed_id
		`,
		map[string]interface{}{
			"feed_id":         feedID,
			"related_feed_id": relatedFeedID,
		})
	return err
}

func (s *store) AddRelationWithPolicies(ctx context.Context, tx *sqlx.Tx, feedID, relatedFeedID string, policies pq.StringArray) error {
	_, err := tx.NamedExecContext(ctx,
		`
		INSERT INTO feed_relation (feed_id, related_feed_id, policies)
		VALUES (:feed_id, :related_feed_id, :policies)
		ON CONFLICT (feed_id, related_feed_id) DO NOTHING
		`,
		map[string]interface{}{
			"feed_id":         feedID,
			"related_feed_id": relatedFeedID,
			"policies":        policies,
		})
	return err
}

// GetRelatedFeeds returns the posts attached to the given slot occupant.
//
// Rows are stored as (feed_id=attached post, related_feed_id=occupant), so the
// occupant is matched against related_feed_id and the attached posts come back
// from the feed_id column. This query historically ran the other way around —
// matching the occupant against feed_id — which can never hit for a real
// occupant, so every TypePosts group rendered as the head alone. Flipping the
// reader rather than the writers is deliberate: all five write paths and the FK
// already agree on this orientation, and existing rows need no migration.
func (s *store) GetRelatedFeeds(ctx context.Context, feedID string) ([]string, error) {
	var relatedFeedIDs []string
	err := s.db.SelectContext(ctx, &relatedFeedIDs,
		`
		SELECT feed_id
		FROM feed_relation
		WHERE related_feed_id = $1
		`,
		feedID)
	return relatedFeedIDs, err
}

// GetPoolCandidates returns every candidate of a pool slot with its policies
// and weight. ORDER BY feed_id is load-bearing: PickFromPool walks the list
// accumulating weight to place a hash point, so the iteration order must be
// identical on every call or the sticky draw stops being sticky.
func (s *store) GetPoolCandidates(ctx context.Context, poolID string) ([]model.PoolCandidate, error) {
	candidates := []model.PoolCandidate{}
	err := s.db.SelectContext(ctx, &candidates,
		`
		SELECT feed_id, policies, weight
		FROM feed_relation
		WHERE related_feed_id = $1
		ORDER BY feed_id
		`,
		poolID)
	return candidates, err
}
