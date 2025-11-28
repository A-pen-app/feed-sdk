package model

import (
	"context"
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
	Inverval PolicyType = "interval"
)

type PolicyResolver interface {
	GetPostViewCount(ctx context.Context, postID string, uniqueUser bool) (int64, error)
	GetUserAttribute(ctx context.Context, userID string) ([]string, error)
}

func (p PolicyType) String() string {
	return string(p)
}

func (p PolicyType) Violated(ctx context.Context, userId, feedId string, resolver PolicyResolver) bool {
	// whenever there is a violation to policy attribute, the post is removed from the feed
	if parsed := strings.Split(p.String(), "-"); len(parsed) > 1 {
		policyName, rawParam := parsed[0], parsed[1]
		logging.Debug(ctx, "examine violation of policy", "feed_id", feedId, "policy", p, "param", rawParam)
		switch PolicyType(policyName) {
		case Exposure:
			limit, err := strconv.ParseInt(rawParam, 10, 64)
			if err != nil {
				logging.Errorw(ctx, "failed parsing policy number, the policy will not take effect", "feed_id", feedId, "policy", p, "param", rawParam)
				return false
			}
			if resolver == nil {
				logging.Errorw(ctx, "resolver cannot be nil, the policy will not take effect", "feed_id", feedId, "policy", p)
				return false
			}
			var uniqueUser bool
			if len(parsed) > 2 && parsed[2] == Distinct.String() {
				uniqueUser = true
			}
			views, err := resolver.GetPostViewCount(ctx, feedId, uniqueUser)
			if err != nil {
				logging.Errorw(ctx, "failed getting post's view count, the policy will not take effect", "feed_id", feedId, "policy", p)
				return false
			}
			if views > limit {
				return true
			}
		case Inexpose: // the time when the feed should start having exposure
			inexposeTime, err := strconv.ParseInt(rawParam, 10, 64)
			if err != nil {
				logging.Errorw(ctx, "failed parsing policy number, the policy will not take effect", "feed_id", feedId, "policy", p, "param", rawParam)
				return false
			}
			if time.Now().Unix() < inexposeTime {
				return true
			}
		case Unexpose: // the time when the feed should stop having exposure
			unexposeTime, err := strconv.ParseInt(rawParam, 10, 64)
			if err != nil {
				logging.Errorw(ctx, "failed parsing policy number, the policy will not take effect", "feed_id", feedId, "policy", p, "param", rawParam)
				return false
			}
			if time.Now().Unix() > unexposeTime {
				return true
			}
		case Istarget: // the target attribute which the feed should have a match
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
		case Distinct: // helper policy for Exposure
		case Inverval: // helper policy for Exposure
		default:
			logging.Errorw(ctx, "unknown policy, the policy will not take effect", "feed_id", feedId, "policy", p)
		}
	} else {
		logging.Errorw(ctx, "failed parsing policy, the policy will not take effect", "feed_id", feedId, "policy", p)
	}
	return false
}

type Policy struct {
	FeedId   string         `json:"id" db:"feed_id"`
	FeedType FeedType       `json:"type" db:"feed_type"`
	Position int            `json:"position" db:"position"`
	Policies pq.StringArray `json:"policies" db:"policies"`
}
