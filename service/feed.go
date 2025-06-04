package service

import (
	"context"

	"github.com/A-pen-app/feed-sdk/model"
)

func NewFeed[T model.Scorable](s store) *Service[T] {
	return &Service[T]{
		store: s,
	}
}

type Service[T model.Scorable] struct {
	store store
}

type store interface {
	GetFeedPositions(ctx context.Context) ([]model.FeedPosition, error)
	PatchFeed(ctx context.Context, id string, feedtype model.FeedType, position int) error
	DeleteFeed(ctx context.Context, id string) error
}

func (f *Service[T]) GetFeeds(ctx context.Context, data []T) (model.Feeds[T], error) {
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

func (f *Service[T]) GetFeedPositions(ctx context.Context, maxPositions int) ([]model.FeedPosition, error) {
	usedPositions, err := f.store.GetFeedPositions(ctx)
	if err != nil {
		return nil, err
	}
	positions := []model.FeedPosition{}
	for i, j := 0, 0; i < maxPositions; i++ {
		if j < len(usedPositions) {
			if usedPositions[j].Position == int64(i) {
				positions = append(positions, usedPositions[j])
				j++
				continue
			}
		}
		positions = append(positions, model.FeedPosition{
			Position: int64(i),
		})
	}
	return positions, nil
}

func (s *Service[T]) PatchFeed(ctx context.Context, id string, feedtype model.FeedType, position int) error {
	return s.store.PatchFeed(ctx, id, feedtype, position)
}

func (s *Service[T]) DeleteFeed(ctx context.Context, id string) error {
	return s.store.DeleteFeed(ctx, id)
}
