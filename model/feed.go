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
	DailyCap PolicyType = "dailycap"
	Audience PolicyType = "audience"
)

const (
	DailyCapMin int64 = 1
	DailyCapMax int64 = 10
)

type PolicyResolver interface {
	GetPostViewCount(ctx context.Context, postID string, uniqueUser bool, duration int64) (int64, error)
	GetViewerPostViewCount(ctx context.Context, postID, userID string) (int64, error)
	GetUserAttribute(ctx context.Context, userID string) ([]string, error)
}

// ExposureResolver answers how many times the viewer has already seen the feed
// today (Asia/Taipei calendar day). A resolver that does not implement it makes
// every dailycap policy a no-op, so an app can bump the SDK before it has the
// per-viewer count.
type ExposureResolver interface {
	GetViewerTodayExposure(ctx context.Context, feedID, userID string) (int64, error)
}

// AudienceResolver answers whether the viewer belongs to at least one of the
// audience packs. Like ExposureResolver it is optional: without it every
// audience policy is a no-op.
type AudienceResolver interface {
	IsViewerInAudience(ctx context.Context, userID string, packIDs []string) (bool, error)
}

type PolicyOptions struct {
	FailClosed bool
}

type PolicyOption func(*PolicyOptions)

func WithFailClosed() PolicyOption { return func(o *PolicyOptions) { o.FailClosed = true } }

func ApplyPolicyOptions(opts ...PolicyOption) PolicyOptions { return applyPolicyOptions(opts) }

func applyPolicyOptions(opts []PolicyOption) PolicyOptions {
	var o PolicyOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

const PolicyResolverAlertKey = "feed_policy_resolver_error"

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
	case DailyCap.String(): // per-viewer, per-day cap: hidden once the viewer saw it this many times today
		limit, err := strconv.ParseInt(rawParam, 10, 64)
		if err != nil || limit < DailyCapMin || limit > DailyCapMax {
			logging.Errorw(ctx, "failed parsing dailycap, the policy will not take effect", "feed_id", feedId, "policy", p, "param", rawParam, "err", err)
			return false
		}
		er, ok := resolver.(ExposureResolver)
		if !ok {
			logging.Warn(ctx, "resolver has no ExposureResolver, dailycap will not take effect feed_id=%s policy=%s", feedId, p)
			return false
		}
		seen, err := er.GetViewerTodayExposure(ctx, feedId, userId)
		if err != nil {
			return resolverFailed(ctx, o, err, feedId, p, "viewer_today_exposure")
		}
		if seen >= limit {
			return true
		}
	case Audience.String(): // hidden from viewers outside every listed audience pack (union)
		packIDs := parsed[1:]
		ar, ok := resolver.(AudienceResolver)
		if !ok {
			logging.Warn(ctx, "resolver has no AudienceResolver, audience will not take effect feed_id=%s policy=%s", feedId, p)
			return false
		}
		in, err := ar.IsViewerInAudience(ctx, userId, packIDs)
		if err != nil {
			return resolverFailed(ctx, o, err, feedId, p, "viewer_in_audience")
		}
		if !in {
			return true
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
