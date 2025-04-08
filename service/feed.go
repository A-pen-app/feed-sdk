package service

import (
	"context"

	"github.com/A-pen-app/feed-sdk/model"
)

type FeedStore interface {
	GetFeedPositions(ctx context.Context) ([]model.FeedPosition, error)
	PatchFeed(ctx context.Context, id string, position int) error
	DeleteFeed(ctx context.Context, id string) error
}

var Feed *FeedSvc[model.Scorable]

type FeedSvc[T model.Scorable] struct {
	store FeedStore
}

func NewFeed[T model.Scorable](store FeedStore) *FeedSvc[T] {
	return &FeedSvc[T]{
		store: store,
	}
}

func (f *FeedSvc[T]) GetFeeds(ctx context.Context, data []T) (model.Feeds[T], error) {
	feeds := model.Feeds[T]{}
	for i := range data {
		feeds = append(
			feeds,
			model.Feed[T]{
				ID:   data[i].GetID(),
				Type: data[i].Feedtype(),
				Data: data[i],
			},
		)
	}

	// sort with scores
	feeds.Sort()

	positions, err := f.store.GetFeedPositions(ctx)
	if err != nil {
		return nil, err
	}

	// make order maps, for later assign
	positionMap := make(map[string]int64)
	for _, position := range positions {
		positionMap[position.FeedID] = position.Position
	}

	// assign orders to feeds
	for i, feed := range feeds {
		// the position of this feed is set
		if j, exists := positionMap[feed.ID]; exists {
			// swap the feed at i to its position at j
			feeds[i], feeds[j] = feeds[j], feeds[i]
		}
	}

	return feeds, nil
}

func (f *FeedSvc[T]) PatchFeed(ctx context.Context, id string, position int) error {
	return f.store.PatchFeed(ctx, id, position)
}

func (f *FeedSvc[T]) DeleteFeed(ctx context.Context, id string) error {
	return f.store.DeleteFeed(ctx, id)
}
