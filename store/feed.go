package store

import (
	"context"

	"github.com/A-pen-app/feed-sdk/model"
	"github.com/jmoiron/sqlx"
)

var Feed *feedStore

type feedStore struct {
}

func NewFeed() *feedStore {
	return &feedStore{}
}

func (fs *feedStore) GetFeedPositions(ctx context.Context, db *sqlx.DB) ([]model.FeedPosition, error) {
	orders := []model.FeedPosition{}

	if err := db.Select(
		&orders,
		`
		SELECT 
			feed.feed_id,
			feed.position
		FROM 
			feed
		ORDER BY
			feed.position ASC
		`,
	); err != nil {
		return nil, err
	}

	return orders, nil
}

func (fs *feedStore) PatchFeed(ctx context.Context, id string, position int, db *sqlx.DB) error {
	_, err := db.NamedExec(
		`
		INSERT INTO 
			feed 
			(
				feed_id, 
				position
			)
		VALUES 
			(
				:feed_id,
				:position
			)
		ON CONFLICT
			(feed_id) 
		DO UPDATE SET 
			position = :position
		`,
		map[string]interface{}{
			"feed_id":  id,
			"position": position,
		})
	return err
}

func (fs *feedStore) DeleteFeed(ctx context.Context, id string, db *sqlx.DB) error {
	_, err := db.NamedExec(
		`
		DELETE FROM
			feed 
		WHERE 
			feed_id=:feed_id
		`,
		map[string]interface{}{
			"feed_id": id,
		})
	return err
}
