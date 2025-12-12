package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/A-pen-app/feed-sdk/model"
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

// Mock implementation of Scorable for testing
type MockPost struct {
	id       string
	feedType model.FeedType
	score    float64
}

func (m MockPost) GetID() string {
	return m.id
}

func (m MockPost) Feedtype() model.FeedType {
	return m.feedType
}

func (m MockPost) Score() float64 {
	return m.score
}

// Mock store implementation
type mockStore struct {
	policies    []model.Policy
	policiesErr error
	patchErr    error
	deleteErr   error
}

func (m *mockStore) GetPolicies(ctx context.Context) ([]model.Policy, error) {
	if m.policiesErr != nil {
		return nil, m.policiesErr
	}
	return m.policies, nil
}

func (m *mockStore) PatchFeed(ctx context.Context, id string, feedtype model.FeedType, position int) error {
	return m.patchErr
}

func (m *mockStore) DeleteFeed(ctx context.Context, id string) error {
	return m.deleteErr
}

// Mock policy resolver
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

func TestGetFeeds(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		input         []MockPost
		policies      []model.Policy
		storeErr      error
		expectedIDs   []string // Expected order after sorting and positioning
		expectedError bool
	}{
		{
			name: "basic sorting without policies",
			input: []MockPost{
				{id: "post1", feedType: model.TypePost, score: 50.0},
				{id: "post2", feedType: model.TypePost, score: 100.0},
				{id: "post3", feedType: model.TypePost, score: 75.0},
			},
			policies:    []model.Policy{},
			expectedIDs: []string{"post2", "post3", "post1"},
		},
		{
			name: "positioning at index 0",
			input: []MockPost{
				{id: "post1", feedType: model.TypePost, score: 50.0},
				{id: "post2", feedType: model.TypePost, score: 100.0},
				{id: "post3", feedType: model.TypePost, score: 75.0},
			},
			policies: []model.Policy{
				{FeedId: "post1", Position: 0},
			},
			expectedIDs: []string{"post1", "post2", "post3"},
		},
		{
			name: "positioning in middle",
			input: []MockPost{
				{id: "post1", feedType: model.TypePost, score: 50.0},
				{id: "post2", feedType: model.TypePost, score: 100.0},
				{id: "post3", feedType: model.TypePost, score: 75.0},
			},
			policies: []model.Policy{
				{FeedId: "post1", Position: 1},
			},
			expectedIDs: []string{"post2", "post1", "post3"},
		},
		{
			name: "positioning at end",
			input: []MockPost{
				{id: "post1", feedType: model.TypePost, score: 50.0},
				{id: "post2", feedType: model.TypePost, score: 100.0},
				{id: "post3", feedType: model.TypePost, score: 75.0},
			},
			policies: []model.Policy{
				{FeedId: "post2", Position: 2},
			},
			expectedIDs: []string{"post3", "post1", "post2"},
		},
		{
			name: "multiple positioned feeds",
			input: []MockPost{
				{id: "post1", feedType: model.TypePost, score: 50.0},
				{id: "post2", feedType: model.TypePost, score: 100.0},
				{id: "post3", feedType: model.TypePost, score: 75.0},
				{id: "post4", feedType: model.TypePost, score: 60.0},
			},
			policies: []model.Policy{
				{FeedId: "post1", Position: 0},
				{FeedId: "post4", Position: 2},
			},
			expectedIDs: []string{"post1", "post2", "post4", "post3"},
		},
		{
			name: "store returns error",
			input: []MockPost{
				{id: "post1", feedType: model.TypePost, score: 50.0},
			},
			storeErr:      errors.New("database error"),
			expectedError: true,
		},
		{
			name:        "empty input",
			input:       []MockPost{},
			policies:    []model.Policy{},
			expectedIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &mockStore{
				policies:    tt.policies,
				policiesErr: tt.storeErr,
			}
			svc := NewFeed[MockPost](mockStore)

			feeds, err := svc.GetFeeds(ctx, tt.input)

			if tt.expectedError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(feeds) != len(tt.expectedIDs) {
				t.Fatalf("expected %d feeds, got %d", len(tt.expectedIDs), len(feeds))
			}

			for i, expectedID := range tt.expectedIDs {
				if feeds[i].ID != expectedID {
					t.Errorf("at position %d: expected ID %s, got %s", i, expectedID, feeds[i].ID)
				}
			}
		})
	}
}

func TestGetPolicies(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		maxPositions   int
		usedPolicies   []model.Policy
		storeErr       error
		expectedCount  int
		expectedError  bool
		validateResult func(t *testing.T, policies []model.Policy)
	}{
		{
			name:          "generate policies with no used positions",
			maxPositions:  5,
			usedPolicies:  []model.Policy{},
			expectedCount: 5,
			validateResult: func(t *testing.T, policies []model.Policy) {
				for i := 0; i < 5; i++ {
					if policies[i].Position != i {
						t.Errorf("at index %d: expected position %d, got %d", i, i, policies[i].Position)
					}
					if policies[i].FeedId != "" {
						t.Errorf("at index %d: expected empty FeedId, got %s", i, policies[i].FeedId)
					}
				}
			},
		},
		{
			name:         "mix used and empty positions",
			maxPositions: 5,
			usedPolicies: []model.Policy{
				{FeedId: "feed1", FeedType: model.TypePost, Position: 1},
				{FeedId: "feed2", FeedType: model.TypePost, Position: 3},
			},
			expectedCount: 5,
			validateResult: func(t *testing.T, policies []model.Policy) {
				// Position 0: empty
				if policies[0].FeedId != "" || policies[0].Position != 0 {
					t.Errorf("position 0: expected empty policy")
				}
				// Position 1: feed1
				if policies[1].FeedId != "feed1" || policies[1].Position != 1 {
					t.Errorf("position 1: expected feed1")
				}
				// Position 2: empty
				if policies[2].FeedId != "" || policies[2].Position != 2 {
					t.Errorf("position 2: expected empty policy")
				}
				// Position 3: feed2
				if policies[3].FeedId != "feed2" || policies[3].Position != 3 {
					t.Errorf("position 3: expected feed2")
				}
				// Position 4: empty
				if policies[4].FeedId != "" || policies[4].Position != 4 {
					t.Errorf("position 4: expected empty policy")
				}
			},
		},
		{
			name:         "consecutive used positions",
			maxPositions: 3,
			usedPolicies: []model.Policy{
				{FeedId: "feed1", FeedType: model.TypePost, Position: 0},
				{FeedId: "feed2", FeedType: model.TypePost, Position: 1},
				{FeedId: "feed3", FeedType: model.TypePost, Position: 2},
			},
			expectedCount: 3,
			validateResult: func(t *testing.T, policies []model.Policy) {
				for i := 0; i < 3; i++ {
					expectedID := fmt.Sprintf("feed%d", i+1)
					if policies[i].FeedId != expectedID {
						t.Errorf("at position %d: expected %s, got %s", i, expectedID, policies[i].FeedId)
					}
				}
			},
		},
		{
			name:          "store returns error",
			maxPositions:  3,
			storeErr:      errors.New("database error"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &mockStore{
				policies:    tt.usedPolicies,
				policiesErr: tt.storeErr,
			}
			svc := NewFeed[MockPost](mockStore)

			policies, err := svc.GetPolicies(ctx, tt.maxPositions)

			if tt.expectedError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(policies) != tt.expectedCount {
				t.Fatalf("expected %d policies, got %d", tt.expectedCount, len(policies))
			}

			if tt.validateResult != nil {
				tt.validateResult(t, policies)
			}
		})
	}
}

func TestPatchFeed(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		id            string
		feedType      model.FeedType
		position      int
		storeErr      error
		expectedError bool
	}{
		{
			name:     "successful patch",
			id:       "feed123",
			feedType: model.TypePost,
			position: 5,
		},
		{
			name:          "store returns error",
			id:            "feed123",
			feedType:      model.TypePost,
			position:      5,
			storeErr:      errors.New("database error"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &mockStore{
				patchErr: tt.storeErr,
			}
			svc := NewFeed[MockPost](mockStore)

			err := svc.PatchFeed(ctx, tt.id, tt.feedType, tt.position)

			if tt.expectedError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDeleteFeed(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		id            string
		storeErr      error
		expectedError bool
	}{
		{
			name: "successful delete",
			id:   "feed123",
		},
		{
			name:          "store returns error",
			id:            "feed123",
			storeErr:      errors.New("database error"),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &mockStore{
				deleteErr: tt.storeErr,
			}
			svc := NewFeed[MockPost](mockStore)

			err := svc.DeleteFeed(ctx, tt.id)

			if tt.expectedError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuildPolicyViolationMap(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name               string
		policyMap          map[string]*model.Policy
		resolver           model.PolicyResolver
		expectedViolations map[string]string
	}{
		{
			name:               "empty policy map",
			policyMap:          map[string]*model.Policy{},
			resolver:           &mockPolicyResolver{},
			expectedViolations: map[string]string{},
		},
		{
			name: "exposure policy - no violation",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"exposure:1000"},
				},
			},
			resolver: &mockPolicyResolver{
				viewCounts: map[string]int64{"post1": 500},
			},
			expectedViolations: map[string]string{},
		},
		{
			name: "exposure policy - violation",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"exposure:1000"},
				},
			},
			resolver: &mockPolicyResolver{
				viewCounts: map[string]int64{"post1": 1500},
			},
			expectedViolations: map[string]string{
				"post1": "exposure:1000",
			},
		},
		{
			name: "inexpose policy - not yet exposed (violation)",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"inexpose:9999999999"}, // Far future timestamp
				},
			},
			resolver: &mockPolicyResolver{},
			expectedViolations: map[string]string{
				"post1": "inexpose:9999999999",
			},
		},
		{
			name: "inexpose policy - can be exposed (no violation)",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"inexpose:1000000000"}, // Old timestamp
				},
			},
			resolver:           &mockPolicyResolver{},
			expectedViolations: map[string]string{},
		},
		{
			name: "unexpose policy - already expired (violation)",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"unexpose:1000000000"}, // Old timestamp
				},
			},
			resolver: &mockPolicyResolver{},
			expectedViolations: map[string]string{
				"post1": "unexpose:1000000000",
			},
		},
		{
			name: "unexpose policy - not expired (no violation)",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"unexpose:9999999999"}, // Far future
				},
			},
			resolver:           &mockPolicyResolver{},
			expectedViolations: map[string]string{},
		},
		{
			name: "multiple policies with first violation",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId: "post1",
					Policies: pq.StringArray{
						"exposure:1000",
						"unexpose:9999999999",
					},
				},
			},
			resolver: &mockPolicyResolver{
				viewCounts: map[string]int64{"post1": 1500},
			},
			expectedViolations: map[string]string{
				"post1": "exposure:1000", // First violation stops checking
			},
		},
		{
			name: "multiple feeds with mixed violations",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"exposure:1000"},
				},
				"post2": {
					FeedId:   "post2",
					Policies: pq.StringArray{"exposure:1000"},
				},
				"post3": {
					FeedId:   "post3",
					Policies: pq.StringArray{"inexpose:1000000000"},
				},
			},
			resolver: &mockPolicyResolver{
				viewCounts: map[string]int64{
					"post1": 500,  // No violation
					"post2": 1500, // Violation
				},
			},
			expectedViolations: map[string]string{
				"post2": "exposure:1000",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &mockStore{}
			svc := NewFeed[MockPost](mockStore)

			violations := svc.BuildPolicyViolationMap(ctx, "test-user", tt.policyMap, tt.resolver)

			if len(violations) != len(tt.expectedViolations) {
				t.Fatalf("expected %d violations, got %d", len(tt.expectedViolations), len(violations))
			}

			for feedID, expectedPolicy := range tt.expectedViolations {
				if actualPolicy, exists := violations[feedID]; !exists {
					t.Errorf("expected violation for feed %s, but not found", feedID)
				} else if actualPolicy != expectedPolicy {
					t.Errorf("for feed %s: expected policy %s, got %s", feedID, expectedPolicy, actualPolicy)
				}
			}

			// Check for unexpected violations
			for feedID := range violations {
				if _, expected := tt.expectedViolations[feedID]; !expected {
					t.Errorf("unexpected violation for feed %s", feedID)
				}
			}
		})
	}
}

func TestCheckPolicyViolation(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Unix()

	tests := []struct {
		name               string
		feedID             string
		policies           []string
		resolver           model.PolicyResolver
		expectedViolation  bool
		expectedPolicyName string
	}{
		{
			name:              "invalid policy format - no colon",
			feedID:            "post1",
			policies:          []string{"invalid"},
			resolver:          &mockPolicyResolver{},
			expectedViolation: false,
		},
		{
			name:              "invalid policy setting - not a number",
			feedID:            "post1",
			policies:          []string{"exposure:abc"},
			resolver:          &mockPolicyResolver{},
			expectedViolation: false,
		},
		{
			name:              "unknown policy type",
			feedID:            "post1",
			policies:          []string{"unknown:1000"},
			resolver:          &mockPolicyResolver{},
			expectedViolation: false,
		},
		{
			name:              "exposure with nil resolver",
			feedID:            "post1",
			policies:          []string{"exposure:1000"},
			resolver:          nil,
			expectedViolation: false,
		},
		{
			name:     "exposure with resolver error",
			feedID:   "post1",
			policies: []string{"exposure:1000"},
			resolver: &mockPolicyResolver{
				err: errors.New("resolver error"),
			},
			expectedViolation: false,
		},
		{
			name:               "inexpose - current time before threshold",
			feedID:             "post1",
			policies:           []string{"inexpose:" + strconv.FormatInt(now+10000, 10)},
			resolver:           &mockPolicyResolver{},
			expectedViolation:  true,
			expectedPolicyName: "inexpose:" + strconv.FormatInt(now+10000, 10),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &mockStore{}
			svc := NewFeed[MockPost](mockStore)

			violation := make(map[string]string)
			svc.checkPolicyViolation(ctx, "test-user", tt.feedID, &violation, tt.policies, tt.resolver)

			if tt.expectedViolation {
				if _, exists := violation[tt.feedID]; !exists {
					t.Errorf("expected violation for feed %s, but not found", tt.feedID)
				} else if violation[tt.feedID] != tt.expectedPolicyName {
					t.Errorf("expected policy %s, got %s", tt.expectedPolicyName, violation[tt.feedID])
				}
			} else {
				if _, exists := violation[tt.feedID]; exists {
					t.Errorf("unexpected violation for feed %s: %s", tt.feedID, violation[tt.feedID])
				}
			}
		})
	}
}

func TestBuildPolicyViolationMap_IstargetPolicy(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name               string
		userID             string
		policyMap          map[string]*model.Policy
		userAttrs          map[string][]string
		expectedViolations map[string]string
		description        string
	}{
		{
			name:   "user has matching target attribute",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:premium"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"premium", "verified"},
			},
			expectedViolations: map[string]string{},
			description:        "Post should be visible when user has the target attribute",
		},
		{
			name:   "user does not have matching target attribute",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:premium"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"basic", "verified"},
			},
			expectedViolations: map[string]string{
				"post1": "istarget:premium",
			},
			description: "Post should be hidden when user lacks the target attribute",
		},
		{
			name:   "user has empty attributes list",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:premium"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {},
			},
			expectedViolations: map[string]string{
				"post1": "istarget:premium",
			},
			description: "Post should be hidden when user has no attributes",
		},
		{
			name:   "multiple posts with different target attributes",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:premium"},
				},
				"post2": {
					FeedId:   "post2",
					Policies: pq.StringArray{"istarget:verified"},
				},
				"post3": {
					FeedId:   "post3",
					Policies: pq.StringArray{"istarget:admin"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"premium", "verified"},
			},
			expectedViolations: map[string]string{
				"post3": "istarget:admin",
			},
			description: "Only posts requiring missing attributes should be hidden",
		},
		{
			name:   "case sensitive attribute matching",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:Premium"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"premium"},
			},
			expectedViolations: map[string]string{
				"post1": "istarget:Premium",
			},
			description: "Attribute matching should be case-sensitive",
		},
		{
			name:   "user with many attributes matches one target",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:verified"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"basic", "active", "verified", "subscribed", "member"},
			},
			expectedViolations: map[string]string{},
			description:        "Post should be visible when target is found among many attributes",
		},
		{
			name:   "exact string match required",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:prem"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"premium"},
			},
			expectedViolations: map[string]string{
				"post1": "istarget:prem",
			},
			description: "Partial matches should not count - exact match required",
		},
		{
			name:   "attribute with underscore",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:vip_2024"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"vip_2024", "active"},
			},
			expectedViolations: map[string]string{},
			description:        "Target attributes with underscores should work",
		},
		{
			name:   "attribute with dash in value",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:vip-2024"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"vip-2024", "active"},
			},
			expectedViolations: map[string]string{},
			description:        "With colon separator, dashes in attribute value work correctly",
		},
		{
			name:   "user not in attributes map",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:premium"},
				},
			},
			userAttrs: map[string][]string{
				"user2": {"premium"},
			},
			expectedViolations: map[string]string{
				"post1": "istarget:premium",
			},
			description: "Post should be hidden when user not found in attributes map",
		},
		{
			name:   "multiple istarget policies - all must match",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:premium", "istarget:verified"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"premium"},
			},
			expectedViolations: map[string]string{
				"post1": "istarget:verified",
			},
			description: "First violation should be returned when multiple istarget policies exist",
		},
		{
			name:   "istarget with other policies - istarget checked in order",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"exposure:1000", "istarget:premium"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"basic"},
			},
			expectedViolations: map[string]string{
				"post1": "istarget:premium",
			},
			description: "Istarget policy should be evaluated after exposure if exposure passes",
		},
		{
			name:   "empty target attribute parameter",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"premium", ""},
			},
			expectedViolations: map[string]string{},
			description:        "Empty string target should match empty string in user attributes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockPolicyResolver{
				userAttrs:    tt.userAttrs,
				viewCounts:   map[string]int64{},
				userAttrsErr: nil,
			}

			service := NewFeed[MockPost](&mockStore{})
			violations := service.BuildPolicyViolationMap(ctx, tt.userID, tt.policyMap, resolver)

			if len(violations) != len(tt.expectedViolations) {
				t.Errorf("%s: expected %d violations, got %d\nExpected: %v\nGot: %v",
					tt.description, len(tt.expectedViolations), len(violations),
					tt.expectedViolations, violations)
			}

			for postID, expectedPolicy := range tt.expectedViolations {
				if actualPolicy, exists := violations[postID]; !exists {
					t.Errorf("%s: expected violation for post %s with policy %s, but not found",
						tt.description, postID, expectedPolicy)
				} else if actualPolicy != expectedPolicy {
					t.Errorf("%s: expected policy %s for post %s, got %s",
						tt.description, expectedPolicy, postID, actualPolicy)
				}
			}

			for postID := range violations {
				if _, expected := tt.expectedViolations[postID]; !expected {
					t.Errorf("%s: unexpected violation for post %s: %s",
						tt.description, postID, violations[postID])
				}
			}
		})
	}
}

func TestBuildPolicyViolationMap_IstargetErrorHandling(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name               string
		userID             string
		policyMap          map[string]*model.Policy
		resolver           model.PolicyResolver
		expectedViolations map[string]string
		description        string
	}{
		{
			name:   "resolver returns error for user attributes",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:premium"},
				},
			},
			resolver: &mockPolicyResolver{
				userAttrs:    map[string][]string{},
				userAttrsErr: errors.New("database error"),
			},
			expectedViolations: map[string]string{},
			description:        "Post should not be hidden when resolver returns error",
		},
		{
			name:   "multiple feeds with resolver error",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:premium"},
				},
				"post2": {
					FeedId:   "post2",
					Policies: pq.StringArray{"istarget:verified"},
				},
			},
			resolver: &mockPolicyResolver{
				userAttrs:    map[string][]string{},
				userAttrsErr: errors.New("service unavailable"),
			},
			expectedViolations: map[string]string{},
			description:        "All istarget policies should be skipped when resolver errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewFeed[MockPost](&mockStore{})
			violations := service.BuildPolicyViolationMap(ctx, tt.userID, tt.policyMap, tt.resolver)

			if len(violations) != len(tt.expectedViolations) {
				t.Errorf("%s: expected %d violations, got %d", tt.description, len(tt.expectedViolations), len(violations))
			}

			for postID, expectedPolicy := range tt.expectedViolations {
				if actualPolicy, exists := violations[postID]; !exists {
					t.Errorf("%s: expected violation for post %s with policy %s, but not found",
						tt.description, postID, expectedPolicy)
				} else if actualPolicy != expectedPolicy {
					t.Errorf("%s: expected policy %s for post %s, got %s",
						tt.description, expectedPolicy, postID, actualPolicy)
				}
			}
		})
	}
}

func TestBuildPolicyViolationMap_IstargetNilResolver(t *testing.T) {
	ctx := context.Background()

	// Test that nil resolver causes panic for istarget policy
	// This documents the current behavior - the code should ideally check for nil
	t.Run("nil resolver causes panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic with nil resolver for istarget policy, but no panic occurred")
			}
		}()

		policyMap := map[string]*model.Policy{
			"post1": {
				FeedId:   "post1",
				Policies: pq.StringArray{"istarget:premium"},
			},
		}

		service := NewFeed[MockPost](&mockStore{})
		_ = service.BuildPolicyViolationMap(ctx, "user1", policyMap, nil)
	})
}

func TestBuildPolicyViolationMap_MixedPoliciesWithIstarget(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Unix()

	tests := []struct {
		name               string
		userID             string
		policyMap          map[string]*model.Policy
		userAttrs          map[string][]string
		viewCounts         map[string]int64
		expectedViolations map[string]string
		description        string
	}{
		{
			name:   "exposure passes but istarget fails",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"exposure:1000", "istarget:premium"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"basic"},
			},
			viewCounts: map[string]int64{
				"post1": 500,
			},
			expectedViolations: map[string]string{
				"post1": "istarget:premium",
			},
			description: "Should fail on istarget when exposure passes",
		},
		{
			name:   "exposure fails before istarget check",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"exposure:1000", "istarget:premium"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"premium"},
			},
			viewCounts: map[string]int64{
				"post1": 1500,
			},
			expectedViolations: map[string]string{
				"post1": "exposure:1000",
			},
			description: "Should fail on exposure and not check istarget",
		},
		{
			name:   "all policies pass including istarget",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId: "post1",
					Policies: pq.StringArray{
						"exposure:1000",
						"istarget:premium",
						"inexpose:" + strconv.FormatInt(now-3600, 10),
						"unexpose:" + strconv.FormatInt(now+3600, 10),
					},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"premium", "verified"},
			},
			viewCounts: map[string]int64{
				"post1": 500,
			},
			expectedViolations: map[string]string{},
			description:        "Post should be visible when all policies pass",
		},
		{
			name:   "time-based and istarget policies mixed",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId: "post1",
					Policies: pq.StringArray{
						"inexpose:" + strconv.FormatInt(now-3600, 10),
						"istarget:premium",
						"unexpose:" + strconv.FormatInt(now+3600, 10),
					},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"basic"},
			},
			viewCounts: map[string]int64{},
			expectedViolations: map[string]string{
				"post1": "istarget:premium",
			},
			description: "Should evaluate istarget between time-based policies",
		},
		{
			name:   "multiple posts with different policy combinations",
			userID: "user1",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedId:   "post1",
					Policies: pq.StringArray{"istarget:premium"},
				},
				"post2": {
					FeedId:   "post2",
					Policies: pq.StringArray{"exposure:1000"},
				},
				"post3": {
					FeedId:   "post3",
					Policies: pq.StringArray{"istarget:verified", "exposure:500"},
				},
			},
			userAttrs: map[string][]string{
				"user1": {"premium", "basic"},
			},
			viewCounts: map[string]int64{
				"post2": 1500,
				"post3": 300,
			},
			expectedViolations: map[string]string{
				"post2": "exposure:1000",
				"post3": "istarget:verified",
			},
			description: "Each post should be evaluated independently",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &mockPolicyResolver{
				userAttrs:  tt.userAttrs,
				viewCounts: tt.viewCounts,
			}

			service := NewFeed[MockPost](&mockStore{})
			violations := service.BuildPolicyViolationMap(ctx, tt.userID, tt.policyMap, resolver)

			if len(violations) != len(tt.expectedViolations) {
				t.Errorf("%s: expected %d violations, got %d\nExpected: %v\nGot: %v",
					tt.description, len(tt.expectedViolations), len(violations),
					tt.expectedViolations, violations)
			}

			for postID, expectedPolicy := range tt.expectedViolations {
				if actualPolicy, exists := violations[postID]; !exists {
					t.Errorf("%s: expected violation for post %s with policy %s, but not found",
						tt.description, postID, expectedPolicy)
				} else if actualPolicy != expectedPolicy {
					t.Errorf("%s: expected policy %s for post %s, got %s",
						tt.description, expectedPolicy, postID, actualPolicy)
				}
			}
		})
	}
}
