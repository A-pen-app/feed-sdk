package store

import (
	"context"

	"github.com/A-pen-app/feed-sdk/model"
	"github.com/jmoiron/sqlx"
)

func NewFeed(db *sqlx.DB) *store {
	if db == nil {
		panic("database connection cannot be nil")
	}

	return &store{
		db: db,
	}
}

type store struct {
	db *sqlx.DB
}

func (f *store) GetFeedPositions(ctx context.Context) ([]model.FeedPosition, error) {
	orders := []model.FeedPosition{}

	if err := f.db.Select(
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

func (f *store) PatchFeed(ctx context.Context, id string, position int) error {
	_, err := f.db.NamedExec(
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

func (f *store) DeleteFeed(ctx context.Context, id string) error {
	_, err := f.db.NamedExec(
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
