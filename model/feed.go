package model

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/A-pen-app/logging"
	"github.com/lib/pq"
)

type Feeds[T Scorable] []Feed[T]

func (f Feeds[T]) Sort() {
	sort.SliceStable(f, func(i, j int) bool {
		return greater(f[i].Data, f[j].Data)
	})
}

func greater[T Scorable](a, b T) bool {
	return a.Score() > b.Score()
}

type Feed[T Scorable] struct {
	ID   string   `json:"id" db:"-"`
	Type FeedType `json:"type" db:"-"`
	Data T        `json:"data" db:"-"`
}

type Scorable interface {
	Feedtype() FeedType
	Score() float64
	GetID() string
}

type FeedType string

const (
	TypePost    FeedType = "post"
	TypeBanners FeedType = "banners"
	TypeChat    FeedType = "chat"
)

type PolicyType string

const (
	Exposure PolicyType = "exposure"
	Inexpose PolicyType = "inexpose"
	Unexpose PolicyType = "unexpose"
	Istarget PolicyType = "istarget"
	Distinct PolicyType = "distinct"
	Duration PolicyType = "duration"
	IsTheOne PolicyType = "istheone"
)

type PolicyResolver interface {
	GetPostViewCount(ctx context.Context, postID string, uniqueUser bool, duration int64, targetUserId string) (int64, error)
	GetUserAttribute(ctx context.Context, userID string) ([]string, error)
}

func (p PolicyType) String() string {
	return string(p)
}

func (p PolicyType) exposureParamParser(ctx context.Context, parsed []string) (bool, int64, string, error) {
	var err error
	var duration int64
	var unique bool
	var userId string
loop:
	for i := 0; i < len(parsed); i++ {
		switch parsed[i] {
		case Distinct.String():
			unique = true
		case Duration.String():
			if i == len(parsed)-1 {
				err = errors.New("helper policy parsing error for polcy type duration")
				break loop // there should be a number following duration which defines how long the intercal is
			}
			duration, err = strconv.ParseInt(parsed[i+1], 10, 64)
			if err != nil {
				logging.Errorw(ctx, "failed parsing policy number", "policy", p, "param", parsed[i])
				break loop
			}
			i++ // we have used up two params from the parsed strings
		case IsTheOne.String():
			if i == len(parsed)-1 {
				err = errors.New("helper policy parsing error for polcy type istheone")
				break loop // there should be a string following istheone which defines which user_id to target
			}
			userId = parsed[i+1]
			i++
		default:
			err = errors.New("unknown helper policy for policy type exposure")
			break loop
		}
	}
	return unique, duration, userId, err
}

func (p PolicyType) Violated(ctx context.Context, userId, feedId string, resolver PolicyResolver) bool {
	// whenever there is a violation to policy attribute, the post is removed from the feed
	parsed := strings.Split(p.String(), "-")
	if len(parsed) <= 1 {
		logging.Errorw(ctx, "failed parsing policy, the policy will not take effect", "feed_id", feedId, "policy", p)
		return false
	}
	policyName, rawParam := parsed[0], parsed[1]
	logging.Debug(ctx, "examine violation of policy", "feed_id", feedId, "policy", p, "param", rawParam)
	switch policyName {
	case Exposure.String():
		limit, err := strconv.ParseInt(rawParam, 10, 64)
		if err != nil {
			logging.Errorw(ctx, "failed parsing policy number, the policy will not take effect", "feed_id", feedId, "policy", p, "param", rawParam)
			return false
		}
		if resolver == nil {
			logging.Errorw(ctx, "resolver cannot be nil, the policy will not take effect", "feed_id", feedId, "policy", p)
			return false
		}
		var duration int64
		var uniqueUser bool
		var targetUserId string
		if len(parsed) > 2 {
			uniqueUser, duration, targetUserId, err = Exposure.exposureParamParser(ctx, parsed[2:])
			if err != nil {
				logging.Errorw(ctx, "failed to parse exposure suffix", "feed_id", feedId, "policy", p, "err", err)
				return false
			}
		}
		views, err := resolver.GetPostViewCount(ctx, feedId, uniqueUser, duration, targetUserId)
		if err != nil {
			logging.Errorw(ctx, "failed getting post's view count, the policy will not take effect", "feed_id", feedId, "policy", p)
			return false
		}
		if views > limit {
			return true
		}
	case Inexpose.String(): // the time when the feed should start having exposure
		inexposeTime, err := strconv.ParseInt(rawParam, 10, 64)
		if err != nil {
			logging.Errorw(ctx, "failed parsing policy number, the policy will not take effect", "feed_id", feedId, "policy", p, "param", rawParam)
			return false
		}
		if time.Now().Unix() < inexposeTime {
			return true
		}
	case Unexpose.String(): // the time when the feed should stop having exposure
		unexposeTime, err := strconv.ParseInt(rawParam, 10, 64)
		if err != nil {
			logging.Errorw(ctx, "failed parsing policy number, the policy will not take effect", "feed_id", feedId, "policy", p, "param", rawParam)
			return false
		}
		if time.Now().Unix() > unexposeTime {
			return true
		}
	case Istarget.String(): // the target attribute which the feed should have a match
		if userAttrs, err := resolver.GetUserAttribute(ctx, userId); err != nil {
			logging.Errorw(ctx, "failed getting user attribute, the policy will not take effect", "feed_id", feedId, "policy", p)
			return false
		} else {
			if !slices.Contains(userAttrs, rawParam) {
				// no attribute matches the given target attribute, the policy is violated
				return true
			}
			// matched - no violation, return false to next policy
		}
	default:
		logging.Errorw(ctx, "unknown policy, the policy will not take effect", "feed_id", feedId, "policy", p)
	}
	return false
}

type Policy struct {
	FeedId   string         `json:"id" db:"feed_id"`
	FeedType FeedType       `json:"type" db:"feed_type"`
	Position int            `json:"position" db:"position"`
	Policies pq.StringArray `json:"policies" db:"policies"`
}
