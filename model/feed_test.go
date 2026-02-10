package model

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/A-pen-app/logging"
	"github.com/lib/pq"
)

func TestMain(m *testing.M) {
	// Initialize logging for tests to prevent nil pointer panics
	_ = logging.Initialize(&logging.Config{
		ProjectID:   "test",
		Development: true,
	})
	defer logging.Finalize()
	os.Exit(m.Run())
}

// Mock implementation of PolicyResolver for testing
type mockPolicyResolver struct {
	viewCounts       map[string]int64
	uniqueViewCounts map[string]int64
	userAttrs        map[string][]string
	err              error
	userAttrsErr     error
}

func (m *mockPolicyResolver) GetPostViewCount(ctx context.Context, postID string, uniqueUser bool, interval int64) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	if uniqueUser {
		if count, exists := m.uniqueViewCounts[postID]; exists {
			return count, nil
		}
	} else {
		if count, exists := m.viewCounts[postID]; exists {
			return count, nil
		}
	}
	return 0, nil
}

func (m *mockPolicyResolver) GetViewerPostViewCount(ctx context.Context, postID, userID string) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	// For testing purposes, return 0 by default
	return 0, nil
}

func (m *mockPolicyResolver) GetPostUniqueUserViewCount(ctx context.Context, postID string) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	if count, exists := m.uniqueViewCounts[postID]; exists {
		return count, nil
	}
	return 0, nil
}

func (m *mockPolicyResolver) GetUserAttribute(ctx context.Context, userID string) ([]string, error) {
	if m.userAttrsErr != nil {
		return nil, m.userAttrsErr
	}
	if attrs, exists := m.userAttrs[userID]; exists {
		return attrs, nil
	}
	return []string{}, nil
}

// Mock implementation of Scorable for testing
type MockPost struct {
	id       string
	feedType FeedType
	score    float64
}

func (m MockPost) GetID() string {
	return m.id
}

func (m MockPost) Feedtype() FeedType {
	return m.feedType
}

func (m MockPost) Score() float64 {
	return m.score
}

func TestFeedsSort(t *testing.T) {
	tests := []struct {
		name     string
		input    Feeds[MockPost]
		expected []string // Expected order of IDs after sorting
	}{
		{
			name: "sort feeds by score descending",
			input: Feeds[MockPost]{
				{ID: "post1", Data: MockPost{id: "post1", score: 50.0}},
				{ID: "post2", Data: MockPost{id: "post2", score: 100.0}},
				{ID: "post3", Data: MockPost{id: "post3", score: 75.0}},
			},
			expected: []string{"post2", "post3", "post1"},
		},
		{
			name: "sort with equal scores maintains stable order",
			input: Feeds[MockPost]{
				{ID: "post1", Data: MockPost{id: "post1", score: 50.0}},
				{ID: "post2", Data: MockPost{id: "post2", score: 50.0}},
				{ID: "post3", Data: MockPost{id: "post3", score: 50.0}},
			},
			expected: []string{"post1", "post2", "post3"},
		},
		{
			name:     "empty feeds slice",
			input:    Feeds[MockPost]{},
			expected: []string{},
		},
		{
			name: "single feed",
			input: Feeds[MockPost]{
				{ID: "post1", Data: MockPost{id: "post1", score: 100.0}},
			},
			expected: []string{"post1"},
		},
		{
			name: "sort with negative and positive scores",
			input: Feeds[MockPost]{
				{ID: "post1", Data: MockPost{id: "post1", score: -10.0}},
				{ID: "post2", Data: MockPost{id: "post2", score: 0.0}},
				{ID: "post3", Data: MockPost{id: "post3", score: 10.0}},
			},
			expected: []string{"post3", "post2", "post1"},
		},
		{
			name: "sort TypePosts feeds by score descending",
			input: Feeds[MockPost]{
				{ID: "posts1", Type: TypePosts, Data: MockPost{id: "posts1", feedType: TypePosts, score: 30.0}},
				{ID: "posts2", Type: TypePosts, Data: MockPost{id: "posts2", feedType: TypePosts, score: 90.0}},
				{ID: "posts3", Type: TypePosts, Data: MockPost{id: "posts3", feedType: TypePosts, score: 60.0}},
			},
			expected: []string{"posts2", "posts3", "posts1"},
		},
		{
			name: "sort mixed feed types by score",
			input: Feeds[MockPost]{
				{ID: "post1", Type: TypePost, Data: MockPost{id: "post1", feedType: TypePost, score: 50.0}},
				{ID: "posts1", Type: TypePosts, Data: MockPost{id: "posts1", feedType: TypePosts, score: 80.0}},
				{ID: "banner1", Type: TypeBanners, Data: MockPost{id: "banner1", feedType: TypeBanners, score: 70.0}},
				{ID: "chat1", Type: TypeChat, Data: MockPost{id: "chat1", feedType: TypeChat, score: 90.0}},
			},
			expected: []string{"chat1", "posts1", "banner1", "post1"},
		},
		{
			name: "sort TypePosts with equal scores maintains stable order",
			input: Feeds[MockPost]{
				{ID: "posts1", Type: TypePosts, Data: MockPost{id: "posts1", feedType: TypePosts, score: 75.0}},
				{ID: "posts2", Type: TypePosts, Data: MockPost{id: "posts2", feedType: TypePosts, score: 75.0}},
				{ID: "posts3", Type: TypePosts, Data: MockPost{id: "posts3", feedType: TypePosts, score: 75.0}},
			},
			expected: []string{"posts1", "posts2", "posts3"},
		},
		{
			name: "single TypePosts feed",
			input: Feeds[MockPost]{
				{ID: "posts1", Type: TypePosts, Data: MockPost{id: "posts1", feedType: TypePosts, score: 100.0}},
			},
			expected: []string{"posts1"},
		},
		{
			name: "TypePosts with zero and negative scores",
			input: Feeds[MockPost]{
				{ID: "posts1", Type: TypePosts, Data: MockPost{id: "posts1", feedType: TypePosts, score: -5.0}},
				{ID: "posts2", Type: TypePosts, Data: MockPost{id: "posts2", feedType: TypePosts, score: 0.0}},
				{ID: "posts3", Type: TypePosts, Data: MockPost{id: "posts3", feedType: TypePosts, score: 5.0}},
			},
			expected: []string{"posts3", "posts2", "posts1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.Sort()

			if len(tt.input) != len(tt.expected) {
				t.Fatalf("expected length %d, got %d", len(tt.expected), len(tt.input))
			}

			for i, expectedID := range tt.expected {
				if tt.input[i].ID != expectedID {
					t.Errorf("at position %d: expected ID %s, got %s", i, expectedID, tt.input[i].ID)
				}
			}
		})
	}
}

func TestGreater(t *testing.T) {
	tests := []struct {
		name     string
		a        MockPost
		b        MockPost
		expected bool
	}{
		{
			name:     "a greater than b",
			a:        MockPost{score: 100.0},
			b:        MockPost{score: 50.0},
			expected: true,
		},
		{
			name:     "a less than b",
			a:        MockPost{score: 50.0},
			b:        MockPost{score: 100.0},
			expected: false,
		},
		{
			name:     "a equals b",
			a:        MockPost{score: 50.0},
			b:        MockPost{score: 50.0},
			expected: false,
		},
		{
			name:     "negative scores",
			a:        MockPost{score: -10.0},
			b:        MockPost{score: -20.0},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := greater(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("greater(%v, %v) = %v, expected %v", tt.a.score, tt.b.score, result, tt.expected)
			}
		})
	}
}

func TestFeedTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		feedType FeedType
		expected string
	}{
		{"post type", TypePost, "post"},
		{"posts type", TypePosts, "posts"},
		{"banners type", TypeBanners, "banners"},
		{"chat type", TypeChat, "chat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.feedType) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(tt.feedType))
			}
		})
	}
}

func TestPolicyTypeConstants(t *testing.T) {
	tests := []struct {
		name       string
		policyType PolicyType
		expected   string
	}{
		{"exposure policy", Exposure, "exposure"},
		{"inexpose policy", Inexpose, "inexpose"},
		{"unexpose policy", Unexpose, "unexpose"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.policyType) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(tt.policyType))
			}
		})
	}
}

func TestPolicyStruct(t *testing.T) {
	tests := []struct {
		name           string
		feedId         string
		feedType       FeedType
		position       int
		policies       pq.StringArray
		expectedLength int
	}{
		{
			name:           "policy with TypePost",
			feedId:         "feed123",
			feedType:       TypePost,
			position:       5,
			policies:       pq.StringArray{"exposure:1000", "inexpose:1234567890"},
			expectedLength: 2,
		},
		{
			name:           "policy with TypePosts",
			feedId:         "feed456",
			feedType:       TypePosts,
			position:       0,
			policies:       pq.StringArray{"exposure:500"},
			expectedLength: 1,
		},
		{
			name:           "policy with TypePosts and multiple policies",
			feedId:         "feed789",
			feedType:       TypePosts,
			position:       3,
			policies:       pq.StringArray{"exposure:1000", "inexpose:1234567890", "unexpose:9999999999"},
			expectedLength: 3,
		},
		{
			name:           "policy with TypePosts and no policies",
			feedId:         "feed012",
			feedType:       TypePosts,
			position:       10,
			policies:       pq.StringArray{},
			expectedLength: 0,
		},
		{
			name:           "policy with TypeBanners",
			feedId:         "banner123",
			feedType:       TypeBanners,
			position:       1,
			policies:       pq.StringArray{"istarget:premium"},
			expectedLength: 1,
		},
		{
			name:           "policy with TypeChat",
			feedId:         "chat123",
			feedType:       TypeChat,
			position:       2,
			policies:       pq.StringArray{"exposure:100", "istarget:verified"},
			expectedLength: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := Policy{
				FeedId:   tt.feedId,
				FeedType: tt.feedType,
				Position: tt.position,
				Policies: tt.policies,
			}

			if policy.FeedId != tt.feedId {
				t.Errorf("expected FeedId '%s', got '%s'", tt.feedId, policy.FeedId)
			}
			if policy.FeedType != tt.feedType {
				t.Errorf("expected FeedType '%s', got '%s'", tt.feedType, policy.FeedType)
			}
			if policy.Position != tt.position {
				t.Errorf("expected Position %d, got %d", tt.position, policy.Position)
			}
			if len(policy.Policies) != tt.expectedLength {
				t.Errorf("expected %d policies, got %d", tt.expectedLength, len(policy.Policies))
			}
		})
	}
}

func TestFeedStruct(t *testing.T) {
	tests := []struct {
		name         string
		id           string
		feedType     FeedType
		dataID       string
		score        float64
	}{
		{
			name:     "feed with TypePost",
			id:       "feed123",
			feedType: TypePost,
			dataID:   "data123",
			score:    100.0,
		},
		{
			name:     "feed with TypePosts",
			id:       "feed456",
			feedType: TypePosts,
			dataID:   "data456",
			score:    85.5,
		},
		{
			name:     "feed with TypeBanners",
			id:       "feed789",
			feedType: TypeBanners,
			dataID:   "data789",
			score:    50.0,
		},
		{
			name:     "feed with TypeChat",
			id:       "feed012",
			feedType: TypeChat,
			dataID:   "data012",
			score:    75.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockData := MockPost{
				id:       tt.dataID,
				feedType: tt.feedType,
				score:    tt.score,
			}

			feed := Feed[MockPost]{
				ID:   tt.id,
				Type: tt.feedType,
				Data: mockData,
			}

			if feed.ID != tt.id {
				t.Errorf("expected ID '%s', got '%s'", tt.id, feed.ID)
			}
			if feed.Type != tt.feedType {
				t.Errorf("expected Type '%s', got '%s'", tt.feedType, feed.Type)
			}
			if feed.Data.GetID() != tt.dataID {
				t.Errorf("expected Data ID '%s', got '%s'", tt.dataID, feed.Data.GetID())
			}
			if feed.Data.Feedtype() != tt.feedType {
				t.Errorf("expected Data Feedtype '%s', got '%s'", tt.feedType, feed.Data.Feedtype())
			}
			if feed.Data.Score() != tt.score {
				t.Errorf("expected Data Score %f, got %f", tt.score, feed.Data.Score())
			}
		})
	}
}

func TestPolicyTypeString(t *testing.T) {
	tests := []struct {
		name     string
		policy   PolicyType
		expected string
	}{
		{"exposure", Exposure, "exposure"},
		{"inexpose", Inexpose, "inexpose"},
		{"unexpose", Unexpose, "unexpose"},
		{"istarget", Istarget, "istarget"},
		{"distinct", Distinct, "distinct"},
		{"duration", Duration, "duration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.policy.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.policy.String())
			}
		})
	}
}

func TestPolicyTypeViolated(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Unix()

	tests := []struct {
		name             string
		policy           PolicyType
		userId           string
		feedId           string
		resolver         PolicyResolver
		expectedViolated bool
	}{
		{
			name:             "invalid policy format - no dash",
			policy:           PolicyType("invalid"),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{},
			expectedViolated: false,
		},
		{
			name:             "exposure - under limit",
			policy:           PolicyType("exposure:1000"),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{viewCounts: map[string]int64{"post1": 500}},
			expectedViolated: false,
		},
		{
			name:             "exposure - over limit (violation)",
			policy:           PolicyType("exposure:1000"),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{viewCounts: map[string]int64{"post1": 1500}},
			expectedViolated: true,
		},
		{
			name:             "exposure - invalid number",
			policy:           PolicyType("exposure:abc"),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{},
			expectedViolated: false,
		},
		{
			name:             "exposure - nil resolver",
			policy:           PolicyType("exposure:1000"),
			userId:           "user1",
			feedId:           "post1",
			resolver:         nil,
			expectedViolated: false,
		},
		{
			name:             "exposure - resolver error",
			policy:           PolicyType("exposure:1000"),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{err: errors.New("db error")},
			expectedViolated: false,
		},
		{
			name:             "inexpose - before start time (violation)",
			policy:           PolicyType("inexpose:" + strconv.FormatInt(now+10000, 10)),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{},
			expectedViolated: true,
		},
		{
			name:             "inexpose - after start time",
			policy:           PolicyType("inexpose:" + strconv.FormatInt(now-10000, 10)),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{},
			expectedViolated: false,
		},
		{
			name:             "inexpose - invalid number",
			policy:           PolicyType("inexpose:abc"),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{},
			expectedViolated: false,
		},
		{
			name:             "unexpose - after end time (violation)",
			policy:           PolicyType("unexpose:" + strconv.FormatInt(now-10000, 10)),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{},
			expectedViolated: true,
		},
		{
			name:             "unexpose - before end time",
			policy:           PolicyType("unexpose:" + strconv.FormatInt(now+10000, 10)),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{},
			expectedViolated: false,
		},
		{
			name:             "unexpose - invalid number",
			policy:           PolicyType("unexpose:abc"),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{},
			expectedViolated: false,
		},
		{
			name:   "istarget - user has attribute",
			policy: PolicyType("istarget:premium"),
			userId: "user1",
			feedId: "post1",
			resolver: &mockPolicyResolver{
				userAttrs: map[string][]string{"user1": {"premium", "verified"}},
			},
			expectedViolated: false,
		},
		{
			name:   "istarget - user missing attribute (violation)",
			policy: PolicyType("istarget:premium"),
			userId: "user1",
			feedId: "post1",
			resolver: &mockPolicyResolver{
				userAttrs: map[string][]string{"user1": {"basic"}},
			},
			expectedViolated: true,
		},
		{
			name:   "istarget - resolver error",
			policy: PolicyType("istarget:premium"),
			userId: "user1",
			feedId: "post1",
			resolver: &mockPolicyResolver{
				userAttrsErr: errors.New("db error"),
			},
			expectedViolated: false,
		},
		{
			name:             "distinct - no violation (helper policy)",
			policy:           PolicyType("distinct:100"),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{},
			expectedViolated: false,
		},
		{
			name:             "interval - no violation (helper policy)",
			policy:           PolicyType("interval:100"),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{},
			expectedViolated: false,
		},
		{
			name:             "unknown policy type",
			policy:           PolicyType("unknown:100"),
			userId:           "user1",
			feedId:           "post1",
			resolver:         &mockPolicyResolver{},
			expectedViolated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.policy.Violated(ctx, tt.userId, tt.feedId, tt.resolver)
			if result != tt.expectedViolated {
				t.Errorf("expected violated=%v, got %v", tt.expectedViolated, result)
			}
		})
	}
}
