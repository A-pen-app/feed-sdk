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

	// create a position map to speed up the discovery of positioned feeds.
	positionMap := make(map[string]int64)
	for _, position := range positions {
		positionMap[position.FeedID] = position.Position
	}

	// create a position->feed map
	positionedFeedMap := make(map[int64]model.Feed[T])
	for i := 0; i < len(feeds); {
		// if the feed is positioned, pull it out and put into map
		if v, exists := positionMap[feeds[i].ID]; exists {
			positionedFeedMap[v] = feeds[i]
			feeds = append(feeds[:i], feeds[i+1:]...)
			continue
		}
		i++
	}

	// insert positioned feeds into feeds
	for _, p := range positions {
		// get the feed from map
		feed := positionedFeedMap[p.Position]
		// insert the feed into feeds, since positions are in ascending order, later insertions will not affect the position of inserted ones.
		feeds = append(
			feeds[:p.Position],
			append(
				model.Feeds[T]{feed},
				feeds[p.Position:]...,
			)...,
		)
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
