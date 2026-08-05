package service

import (
	"context"
	"hash/fnv"
	"math/rand"
	"slices"
	"sort"
	"sync"

	"github.com/A-pen-app/feed-sdk/model"
	"github.com/A-pen-app/logging"
	"github.com/lib/pq"
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
	GetColdstartByAudience(ctx context.Context, audience string) ([]model.Policy, error)
	GetColdstartBySpecialty(ctx context.Context, specialties []string) ([]model.Policy, error)
	PatchFeed(ctx context.Context, id string, feedtype model.FeedType, position int) error
	DeleteFeed(ctx context.Context, id string) error
	AddRelation(ctx context.Context, feedID, relatedFeedID string) error
	RemoveRelation(ctx context.Context, feedID, relatedFeedID string) error
	GetRelatedFeeds(ctx context.Context, feedID string) ([]string, error)
	GetPoolCandidates(ctx context.Context, poolID string) ([]model.PoolCandidate, error)
	CreateFeedPosition(ctx context.Context, feedID string, feedType model.FeedType, position int, policies pq.StringArray) error
	DeleteFeedPosition(ctx context.Context, feedID string, position int) error
}

func (f *Service[T]) GetFeeds(ctx context.Context, data []T) (model.Feeds[T], error) {
	coldstart, _ := ctx.Value(model.COLD_START_KEY).(bool)
	position, _ := ctx.Value(model.POSITION_KEY).(string)

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
		logging.Infow(ctx, "coldstart feed retrieval", "position", position)

		// Prefer the caller-supplied coldstart id set (already merged across
		// audiences and filtered for watched feeds); fall back to the default
		// feed_coldstart table for callers that don't supply one.
		var idList []string
		if ids, ok := ctx.Value(model.COLD_START_IDS_KEY).([]string); ok && len(ids) > 0 {
			idList = ids
		} else {
			positions, err = f.store.GetColdstart(ctx)
			if err != nil {
				return nil, err
			}
			for _, p := range positions {
				idList = append(idList, p.FeedId)
			}
		}

		// Select at most 5 coldstart feed IDs
		if len(idList) > 5 {
			rand.Shuffle(len(idList), func(i, j int) {
				idList[i], idList[j] = idList[j], idList[i]
			})
			idList = idList[:5]
		}

		// Build set of coldstart feed IDs
		coldstartIDs := make(map[string]bool)
		for _, id := range idList {
			coldstartIDs[id] = true
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

// GetColdstartBySpecialty returns the specialty coldstart policies matching the
// given specialties (see store.GetColdstartBySpecialty).
func (f *Service[T]) GetColdstartBySpecialty(ctx context.Context, specialties []string) ([]model.Policy, error) {
	return f.store.GetColdstartBySpecialty(ctx, specialties)
}

func (f *Service[T]) GetColdstartPolicies(ctx context.Context) ([]model.Policy, error) {
	return f.store.GetColdstart(ctx)
}

// GetColdstartByAudience returns the coldstart policies for a single audience
// (see model.ColdstartAudience*). Callers that match several audiences merge the
// results themselves.
func (f *Service[T]) GetColdstartByAudience(ctx context.Context, audience string) ([]model.Policy, error) {
	return f.store.GetColdstartByAudience(ctx, audience)
}

func (s *Service[T]) PatchFeed(ctx context.Context, id string, feedtype model.FeedType, position int) error {
	return s.store.PatchFeed(ctx, id, feedtype, position)
}

func (s *Service[T]) DeleteFeed(ctx context.Context, id string) error {
	return s.store.DeleteFeed(ctx, id)
}

func (s *Service[T]) CreateFeedPosition(ctx context.Context, feedID string, feedType model.FeedType, position int, policies pq.StringArray) error {
	return s.store.CreateFeedPosition(ctx, feedID, feedType, position, policies)
}

func (s *Service[T]) DeleteFeedPosition(ctx context.Context, feedID string, position int) error {
	return s.store.DeleteFeedPosition(ctx, feedID, position)
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

func (s *Service[T]) GetRelatedFeeds(ctx context.Context, feedID string) ([]string, error) {
	return s.store.GetRelatedFeeds(ctx, feedID)
}

// PickFromPool resolves which candidate a pool slot shows to a given user, and
// is the whole render-side contract of a 'posts' slot: the caller treats the
// returned id as an ordinary post and the client never learns the slot held a
// pool. An empty id with a nil error means no candidate survived — the caller
// should render nothing at that position, exactly as it would for a violated
// single-post slot.
//
// Selection happens in two stages.
//
// Eligibility: a candidate is out if its weight is <= 0 (paused: the row is
// kept, its policies and weight untouched, but it never shows) or if any of its
// own policies is violated for this user — the same Violated machinery that
// gates whole slots, evaluated against the candidate's post id so exposure
// counting stays per-post.
//
// Draw: survivors split traffic in proportion to weight via a sticky draw.
// FNV-1a over "userID:poolID" places a point on the accumulated weight line,
// walked in GetPoolCandidates order (feed_id ASC), which must stay
// deterministic for the draw to be sticky. The hash is not cryptographic and
// does not need to be — it only buckets users. The same user therefore always
// resolves to the same candidate while eligibility holds, so the slot does not
// flicker between refreshes, while the population as a whole splits by weight.
// When a candidate drops out (schedule ends, exposure cap reached, paused), its
// users are redistributed among the remaining survivors by the same rule.
func (s *Service[T]) PickFromPool(ctx context.Context, userID, poolID string, resolver model.PolicyResolver) (string, error) {
	candidates, err := s.store.GetPoolCandidates(ctx, poolID)
	if err != nil {
		return "", err
	}

	survivors := make([]model.PoolCandidate, 0, len(candidates))
	totalWeight := uint64(0)
	for _, c := range candidates {
		if c.Weight <= 0 {
			continue
		}
		violated := false
		for _, pol := range c.Policies {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			default:
			}
			if model.PolicyType(pol).Violated(ctx, userID, c.FeedID, resolver) {
				violated = true
				break
			}
		}
		if violated {
			continue
		}
		survivors = append(survivors, c)
		totalWeight += uint64(c.Weight)
	}

	if len(survivors) == 0 {
		return "", nil
	}

	h := fnv.New64a()
	h.Write([]byte(userID + ":" + poolID))
	point := h.Sum64() % totalWeight

	acc := uint64(0)
	for _, c := range survivors {
		acc += uint64(c.Weight)
		if point < acc {
			return c.FeedID, nil
		}
	}
	// Unreachable: point < totalWeight and the accumulator reaches totalWeight
	// on the last survivor. Kept so a future refactor fails loudly, not silently.
	return survivors[len(survivors)-1].FeedID, nil
}
