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

type contextKey string

const COLD_START_KEY contextKey = "coldstart"
const POSITION_KEY contextKey = "position"

// COLD_START_IDS_KEY optionally carries the exact set of coldstart feed ids the
// caller has already assembled (e.g. merged across audiences and filtered for
// already-watched feeds). When present, GetFeeds pins this set instead of
// re-querying the default feed_coldstart table, so the caller owns selection and
// fade-out in one place. Falls back to GetColdstart when absent.
const COLD_START_IDS_KEY contextKey = "coldstart_ids"

// Coldstart audiences map a user condition to the coldstart table that serves it.
// Each audience is backed by its own feed_coldstart* table (see store.GetColdstartByAudience).
const (
	ColdstartAudienceDefault   = "default"   // new users (account created within the coldstart window)
	ColdstartAudienceStudent   = "student"   // users whose character is student
	ColdstartAudienceSpecialty = "specialty" // users who have at least one specialty
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
	TypePost             FeedType = "post"
	TypePosts            FeedType = "posts"
	TypeBanners          FeedType = "banners"
	TypeChat             FeedType = "chat"
	TypeRecommendAuthors FeedType = "recommend_authors"
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
	GetPostViewCount(ctx context.Context, postID string, uniqueUser bool, duration int64) (int64, error)
	GetViewerPostViewCount(ctx context.Context, postID, userID string) (int64, error)
	GetUserAttribute(ctx context.Context, userID string) ([]string, error)
}

// PolicyOptions tune how Violated treats a policy it cannot evaluate.
type PolicyOptions struct {
	// FailClosed makes a policy count as violated (feed hidden) when its
	// resolver lookup fails. The default, fail-open, keeps the historical
	// behaviour of showing the feed; it is also what let a broken exposure
	// query silently disable every exposure cap for months, so operators
	// pinning paid placements should turn this on.
	FailClosed bool
}

// PolicyOption mutates PolicyOptions.
type PolicyOption func(*PolicyOptions)

// WithFailClosed hides the feed when a policy's resolver lookup fails.
func WithFailClosed() PolicyOption { return func(o *PolicyOptions) { o.FailClosed = true } }

func applyPolicyOptions(opts []PolicyOption) PolicyOptions {
	var o PolicyOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

// PolicyResolverAlertKey is the value of the "alert_key" field on every log
// line Violated writes when a resolver lookup fails. Build the log-based alert
// on it rather than on the message text.
const PolicyResolverAlertKey = "feed_policy_resolver_error"

// resolverFailed logs a resolver failure with the fixed alert key and returns
// the verdict the fail mode dictates.
func resolverFailed(ctx context.Context, o PolicyOptions, err error, feedId string, p PolicyType, what string) bool {
	logging.Errorw(ctx, "feed policy resolver failed",
		"alert_key", PolicyResolverAlertKey,
		"lookup", what,
		"feed_id", feedId,
		"policy", p,
		"fail_closed", o.FailClosed,
		"err", err)
	return o.FailClosed
}

func (p PolicyType) String() string {
	return string(p)
}

func (p PolicyType) exposureParamParser(ctx context.Context, parsed []string) (bool, int64, error) {
	var err error
	var duration int64
	var unique bool
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
		default:
			err = errors.New("unknown helper policy for policy type exposure")
			break loop
		}
	}
	return unique, duration, err
}

// Violated reports whether feedId breaks policy p for userId. Malformed
// policies never take effect (they are configuration errors, logged). A
// resolver lookup failure is fail-open unless WithFailClosed is given; either
// way it is logged with PolicyResolverAlertKey.
func (p PolicyType) Violated(ctx context.Context, userId, feedId string, resolver PolicyResolver, opts ...PolicyOption) bool {
	o := applyPolicyOptions(opts)
	// whenever there is a violation to policy attribute, the post is removed from the feed
	parsed := strings.Split(p.String(), ":")
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
		if len(parsed) > 2 {
			uniqueUser, duration, err = Exposure.exposureParamParser(ctx, parsed[2:])
			if err != nil {
				logging.Errorw(ctx, "failed to parse exposure suffix", "feed_id", feedId, "policy", p, "err", err)
				return false
			}
		}
		views, err := resolver.GetPostViewCount(ctx, feedId, uniqueUser, duration)
		if err != nil {
			return resolverFailed(ctx, o, err, feedId, p, "post_view_count")
		}
		if views > limit {
			return true
		}
	case IsTheOne.String():
		limit, err := strconv.ParseInt(rawParam, 10, 64)
		if err != nil {
			logging.Errorw(ctx, "failed parsing policy number, the policy will not take effect", "feed_id", feedId, "policy", p, "param", rawParam)
			return false
		}
		if len(parsed) < 3 {
			logging.Errorw(ctx, "failed to parse istheone suffix", "feed_id", feedId, "policy", p, "err", err)
			return false
		}
		views, err := resolver.GetViewerPostViewCount(ctx, feedId, parsed[2])
		if err != nil {
			return resolverFailed(ctx, o, err, feedId, p, "viewer_post_view_count")
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
			return resolverFailed(ctx, o, err, feedId, p, "user_attribute")
		} else {
			// Every segment after the policy name is one alternative, ORed together:
			// "istarget:cardiology:neurology" matches a user holding either. Separate
			// istarget policies stay ANDed, so pairing it with "istarget:male" reads as
			// (cardiology OR neurology) AND male — the shape an audience filter needs.
			// A single-segment policy is unchanged, which is every policy written to
			// date, so this is purely additive.
			//
			// Matching is case-insensitive: the validate_policies_format trigger
			// installed by store.addPolicyFormatConstraintSQL only accepts
			// [a-z0-9:_-] for the param, so a policy can never carry an
			// upper-case attribute. Resolvers,
			// meanwhile, return attributes verbatim from their own vocabulary (e.g.
			// apen-api returns specialties such as "Cardiology"). Comparing verbatim
			// would make those attributes impossible to target at all.
			matched := false
			for _, target := range parsed[1:] {
				if slices.ContainsFunc(userAttrs, func(attr string) bool {
					return strings.EqualFold(attr, target)
				}) {
					matched = true
					break
				}
			}
			if !matched {
				// no attribute matches any of the target attributes, the policy is violated
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
