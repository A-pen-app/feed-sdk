package store

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

const createFeedRelationTableSQL = `
CREATE TABLE IF NOT EXISTS feed_relation (
	feed_id uuid NOT NULL,
	related_feed_id uuid NOT NULL,
	policies character varying(200)[] NOT NULL DEFAULT ARRAY[]::character varying[],
	CONSTRAINT feed_relation_pkey PRIMARY KEY (feed_id, related_feed_id),
	CONSTRAINT feed_relation_related_feed_id_fkey FOREIGN KEY (related_feed_id) REFERENCES feed(feed_id) ON DELETE CASCADE
)`

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

func (s *store) GetRelatedFeeds(ctx context.Context, feedID string) ([]string, error) {
	var relatedFeedIDs []string
	err := s.db.SelectContext(ctx, &relatedFeedIDs,
		`
		SELECT related_feed_id
		FROM feed_relation
		WHERE feed_id = $1
		`,
		feedID)
	return relatedFeedIDs, err
}
