package service

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

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

	positions, err := f.store.GetPolicies(ctx)
	if err != nil {
		return nil, err
	}

	// create a position map to speed up the discovery of positioned feeds.
	positionMap := make(map[string]int)
	for _, position := range positions {
		positionMap[position.FeedID] = position.Position
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

func (s *Service[T]) PatchFeed(ctx context.Context, id string, feedtype model.FeedType, position int) error {
	return s.store.PatchFeed(ctx, id, feedtype, position)
}

func (s *Service[T]) DeleteFeed(ctx context.Context, id string) error {
	return s.store.DeleteFeed(ctx, id)
}

type PolicyResolver interface {
	GetPostViewCount(ctx context.Context, postID string) (int64, error)
}

// GetPostViewCount(ctx context.Context, postID string) (int64, error)
func (f *Service[T]) BuildPolicyViolationMap(ctx context.Context, policyMap map[string]*model.Policy, resolver PolicyResolver) map[string]string {
	var violation map[string]string = make(map[string]string)
	for postID, policy := range policyMap {
		f.checkPolicyViolation(ctx, &violation, postID, policy.Policies, resolver)
	}
	return violation
}

func (f *Service[T]) checkPolicyViolation(ctx context.Context, violation *map[string]string, postID string, policies []string, resolver PolicyResolver) {
	for _, pol := range policies {
		// whenever there is a violation to policy attribute, the post is removed from the feed
		if parsed := strings.Split(pol, "-"); len(parsed) > 1 {
			policyName, rawsetting := parsed[0], parsed[1]
			policySetting, err := strconv.ParseInt(rawsetting, 10, 64)
			if err != nil {
				logging.Error(ctx, "failed parsing policy number, the policy will not take effect", "post_id", postID, "policy", pol, "policy_setting", policySetting)
				continue
			}
			switch model.PolicyType(policyName) {
			case model.Exposure:
				if resolver == nil {
					logging.Error(ctx, "resolver cannot be nil, the policy will not take effect", "post_id", postID, "policy", pol)
					continue
				}
				totalview, err := resolver.GetPostViewCount(ctx, postID)
				if err != nil {
					logging.Error(ctx, "failed getting post's view count, the policy will not take effect", "post_id", postID, "policy", pol)
					continue
				}
				if totalview > policySetting {
					(*violation)[postID] = pol
					return
				}
			case model.Inexpose:
				if time.Now().Unix() > policySetting {
					(*violation)[postID] = pol
					return
				}
			default:
				logging.Error(ctx, "unknown policy, the policy will not take effect", "post_id", postID, "policy", pol)
			}
		} else {
			logging.Error(ctx, "failed parsing policy, the policy will not take effect", "post_id", postID, "policy", pol)
		}
	}
}
