package store

import (
	"context"

	"github.com/A-pen-app/feed-sdk/model"
)

const createFeedRelationTableSQL = `
CREATE TABLE IF NOT EXISTS feed_relation (
	feed_id uuid NOT NULL,
	related_feed_id uuid NOT NULL,
	position integer NOT NULL DEFAULT 0,
	feed_type character varying(20) NOT NULL DEFAULT 'banners'::character varying,
	policies character varying(200)[] NOT NULL DEFAULT ARRAY[]::character varying[],
	CONSTRAINT feed_relation_pkey PRIMARY KEY (feed_id, related_feed_id),
	CONSTRAINT feed_relation_feed_id_fkey FOREIGN KEY (feed_id) REFERENCES feed(feed_id) ON DELETE CASCADE,
	CONSTRAINT feed_relation_related_feed_id_fkey FOREIGN KEY (related_feed_id) REFERENCES feed(feed_id) ON DELETE CASCADE
)`

func (s *store) AddRelation(ctx context.Context, feedID, relatedFeedID string, feedType model.FeedType, position int) error {
	_, err := s.db.NamedExecContext(ctx,
		`
		INSERT INTO feed_relation (feed_id, related_feed_id, feed_type, position)
		VALUES (:feed_id, :related_feed_id, :feed_type, :position)
		ON CONFLICT (feed_id, related_feed_id) DO UPDATE SET
			feed_type = :feed_type,
			position = :position
		`,
		map[string]interface{}{
			"feed_id":         feedID,
			"related_feed_id": relatedFeedID,
			"feed_type":       feedType,
			"position":        position,
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

type feedRelation struct {
	FeedID        string `db:"feed_id"`
	RelatedFeedID string `db:"related_feed_id"`
}

func (s *store) GetRelatedFeeds(ctx context.Context) (map[string][]string, error) {
	var relations []feedRelation
	err := s.db.SelectContext(ctx, &relations,
		`
		SELECT feed_id, related_feed_id
		FROM feed_relation
		ORDER BY feed_id, position ASC
		`)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]string)
	for _, r := range relations {
		result[r.FeedID] = append(result[r.FeedID], r.RelatedFeedID)
	}
	return result, nil
}
