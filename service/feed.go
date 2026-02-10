package service

import (
	"context"
	"math/rand"
	"slices"
	"sort"
	"sync"

	"github.com/A-pen-app/feed-sdk/model"
	"github.com/A-pen-app/logging"
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
	GetPolicies(ctx context.Context) ([]model.Policy, error)
	GetColdstart(ctx context.Context) ([]model.Policy, error)
	PatchFeed(ctx context.Context, id string, feedtype model.FeedType, position int) error
	DeleteFeed(ctx context.Context, id string) error
	AddRelation(ctx context.Context, feedID, relatedFeedID string, feedType model.FeedType, position int) error
	RemoveRelation(ctx context.Context, feedID, relatedFeedID string) error
	GetRelatedFeeds(ctx context.Context) (map[string][]string, error)
}

func (f *Service[T]) GetFeeds(ctx context.Context, data []T) (model.Feeds[T], error) {
	coldstart, _ := ctx.Value(model.COLD_START_KEY).(bool)

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

	var positions []model.Policy
	var err error

	if coldstart {
		positions, err = f.store.GetColdstart(ctx)
		if err != nil {
			return nil, err
		}

		// Select at most 5 coldstart feed IDs
		if len(positions) > 5 {
			rand.Shuffle(len(positions), func(i, j int) {
				positions[i], positions[j] = positions[j], positions[i]
			})
			positions = positions[:5]
		}

		// Build set of coldstart feed IDs
		coldstartIDs := make(map[string]bool)
		for _, p := range positions {
			coldstartIDs[p.FeedId] = true
		}

		// Remove coldstart feeds from list
		var coldstartFeeds []model.Feed[T]
		feeds = slices.DeleteFunc(feeds, func(feed model.Feed[T]) bool {
			if coldstartIDs[feed.ID] {
				coldstartFeeds = append(coldstartFeeds, feed)
				return true
			}
			return false
		})

		// Insert at random positions in first 10
		randomPositions := rand.Perm(10)[:len(coldstartFeeds)]
		sort.Ints(randomPositions)
		for i, pos := range randomPositions {
			feeds = slices.Insert(feeds, pos, coldstartFeeds[i])
		}
	} else {
		positions, err = f.store.GetPolicies(ctx)
		if err != nil {
			return nil, err
		}

		// create a position map to speed up the discovery of positioned feeds.
		positionMap := make(map[string]int)
		for _, position := range positions {
			positionMap[position.FeedId] = position.Position
		}

		// create a position->feed map
		positionedFeedMap := make(map[int]model.Feed[T])

		nonPositionedFeeds := feeds[:0]
		for i := 0; i < len(feeds); i++ {
			if v, exists := positionMap[feeds[i].ID]; exists {
				// if the feed is positioned, put it into map
				positionedFeedMap[v] = feeds[i]
			} else {
				// collect it otherwise
				nonPositionedFeeds = append(nonPositionedFeeds, feeds[i])
			}
		}
		feeds = nonPositionedFeeds

		for _, p := range positions {
			if feed, exist := positionedFeedMap[p.Position]; exist {
				if len(feeds) < p.Position {
					feeds = append(feeds, feed)
				} else {
					feeds = slices.Insert(feeds, p.Position, feed)
				}
			}
		}
	}

	return feeds, nil
}

func (f *Service[T]) GetPolicies(ctx context.Context, maxPositions int) ([]model.Policy, error) {
	usedPositions, err := f.store.GetPolicies(ctx)
	if err != nil {
		return nil, err
	}
	positions := []model.Policy{}
	for i, j := 0, 0; i < maxPositions; i++ {
		if j < len(usedPositions) {
			if usedPositions[j].Position == i {
				positions = append(positions, usedPositions[j])
				j++
				continue
			}
		}
		positions = append(positions, model.Policy{
			Position: i,
		})
	}
	return positions, nil
}

func (f *Service[T]) GetColdstartPolicies(ctx context.Context) ([]model.Policy, error) {
	return f.store.GetColdstart(ctx)
}

func (s *Service[T]) PatchFeed(ctx context.Context, id string, feedtype model.FeedType, position int) error {
	return s.store.PatchFeed(ctx, id, feedtype, position)
}

func (s *Service[T]) DeleteFeed(ctx context.Context, id string) error {
	return s.store.DeleteFeed(ctx, id)
}

func (f *Service[T]) BuildPolicyViolationMap(ctx context.Context, userID string, policyMap map[string]*model.Policy, resolver model.PolicyResolver) map[string]string {
	var (
		violation = make(map[string]string)
		mu        sync.Mutex
		wg        sync.WaitGroup
	)

	for postID, policy := range policyMap {
		wg.Add(1)
		go func(postID string, policies []string) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					logging.Errorw(ctx, "panic recovered in policy violation check", "post_id", postID, "error", r)
				}
			}()
			for _, pol := range policies {
				select {
				case <-ctx.Done():
					return
				default:
				}
				if model.PolicyType(pol).Violated(ctx, userID, postID, resolver) {
					mu.Lock()
					violation[postID] = pol
					mu.Unlock()
					return
				}
			}
		}(postID, policy.Policies)
	}

	wg.Wait()
	return violation
}

func (s *Service[T]) GetRelatedFeeds(ctx context.Context) (map[string][]string, error) {
	return s.store.GetRelatedFeeds(ctx)
}
