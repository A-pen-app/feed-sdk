package service

import (
	"context"
	"errors"
	"os"
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
	viewCounts    map[string]int64
	userAttrs     map[string][]string
	err           error
	userAttrsErr  error
}

func (m *mockPolicyResolver) GetPostViewCount(ctx context.Context, postID string) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	if count, exists := m.viewCounts[postID]; exists {
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
				{FeedID: "post1", Position: 0},
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
				{FeedID: "post1", Position: 1},
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
				{FeedID: "post2", Position: 2},
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
				{FeedID: "post1", Position: 0},
				{FeedID: "post4", Position: 2},
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
					if policies[i].FeedID != "" {
						t.Errorf("at index %d: expected empty FeedID, got %s", i, policies[i].FeedID)
					}
				}
			},
		},
		{
			name:         "mix used and empty positions",
			maxPositions: 5,
			usedPolicies: []model.Policy{
				{FeedID: "feed1", FeedType: model.TypePost, Position: 1},
				{FeedID: "feed2", FeedType: model.TypePost, Position: 3},
			},
			expectedCount: 5,
			validateResult: func(t *testing.T, policies []model.Policy) {
				// Position 0: empty
				if policies[0].FeedID != "" || policies[0].Position != 0 {
					t.Errorf("position 0: expected empty policy")
				}
				// Position 1: feed1
				if policies[1].FeedID != "feed1" || policies[1].Position != 1 {
					t.Errorf("position 1: expected feed1")
				}
				// Position 2: empty
				if policies[2].FeedID != "" || policies[2].Position != 2 {
					t.Errorf("position 2: expected empty policy")
				}
				// Position 3: feed2
				if policies[3].FeedID != "feed2" || policies[3].Position != 3 {
					t.Errorf("position 3: expected feed2")
				}
				// Position 4: empty
				if policies[4].FeedID != "" || policies[4].Position != 4 {
					t.Errorf("position 4: expected empty policy")
				}
			},
		},
		{
			name:         "consecutive used positions",
			maxPositions: 3,
			usedPolicies: []model.Policy{
				{FeedID: "feed1", FeedType: model.TypePost, Position: 0},
				{FeedID: "feed2", FeedType: model.TypePost, Position: 1},
				{FeedID: "feed3", FeedType: model.TypePost, Position: 2},
			},
			expectedCount: 3,
			validateResult: func(t *testing.T, policies []model.Policy) {
				for i := 0; i < 3; i++ {
					expectedID := "feed" + string(rune('1'+i))
					if policies[i].FeedID != expectedID {
						t.Errorf("at position %d: expected %s, got %s", i, expectedID, policies[i].FeedID)
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
		resolver           PolicyResolver
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
					FeedID:   "post1",
					Policies: pq.StringArray{"exposure-1000"},
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
					FeedID:   "post1",
					Policies: pq.StringArray{"exposure-1000"},
				},
			},
			resolver: &mockPolicyResolver{
				viewCounts: map[string]int64{"post1": 1500},
			},
			expectedViolations: map[string]string{
				"post1": "exposure-1000",
			},
		},
		{
			name: "inexpose policy - not yet exposed (violation)",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedID:   "post1",
					Policies: pq.StringArray{"inexpose-9999999999"}, // Far future timestamp
				},
			},
			resolver: &mockPolicyResolver{},
			expectedViolations: map[string]string{
				"post1": "inexpose-9999999999",
			},
		},
		{
			name: "inexpose policy - can be exposed (no violation)",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedID:   "post1",
					Policies: pq.StringArray{"inexpose-1000000000"}, // Old timestamp
				},
			},
			resolver:           &mockPolicyResolver{},
			expectedViolations: map[string]string{},
		},
		{
			name: "unexpose policy - already expired (violation)",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedID:   "post1",
					Policies: pq.StringArray{"unexpose-1000000000"}, // Old timestamp
				},
			},
			resolver: &mockPolicyResolver{},
			expectedViolations: map[string]string{
				"post1": "unexpose-1000000000",
			},
		},
		{
			name: "unexpose policy - not expired (no violation)",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedID:   "post1",
					Policies: pq.StringArray{"unexpose-9999999999"}, // Far future
				},
			},
			resolver:           &mockPolicyResolver{},
			expectedViolations: map[string]string{},
		},
		{
			name: "multiple policies with first violation",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedID: "post1",
					Policies: pq.StringArray{
						"exposure-1000",
						"unexpose-9999999999",
					},
				},
			},
			resolver: &mockPolicyResolver{
				viewCounts: map[string]int64{"post1": 1500},
			},
			expectedViolations: map[string]string{
				"post1": "exposure-1000", // First violation stops checking
			},
		},
		{
			name: "multiple feeds with mixed violations",
			policyMap: map[string]*model.Policy{
				"post1": {
					FeedID:   "post1",
					Policies: pq.StringArray{"exposure-1000"},
				},
				"post2": {
					FeedID:   "post2",
					Policies: pq.StringArray{"exposure-1000"},
				},
				"post3": {
					FeedID:   "post3",
					Policies: pq.StringArray{"inexpose-1000000000"},
				},
			},
			resolver: &mockPolicyResolver{
				viewCounts: map[string]int64{
					"post1": 500,  // No violation
					"post2": 1500, // Violation
				},
			},
			expectedViolations: map[string]string{
				"post2": "exposure-1000",
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
		resolver           PolicyResolver
		expectedViolation  bool
		expectedPolicyName string
	}{
		{
			name:              "invalid policy format - no dash",
			feedID:            "post1",
			policies:          []string{"invalid"},
			resolver:          &mockPolicyResolver{},
			expectedViolation: false,
		},
		{
			name:              "invalid policy setting - not a number",
			feedID:            "post1",
			policies:          []string{"exposure-abc"},
			resolver:          &mockPolicyResolver{},
			expectedViolation: false,
		},
		{
			name:              "unknown policy type",
			feedID:            "post1",
			policies:          []string{"unknown-1000"},
			resolver:          &mockPolicyResolver{},
			expectedViolation: false,
		},
		{
			name:              "exposure with nil resolver",
			feedID:            "post1",
			policies:          []string{"exposure-1000"},
			resolver:          nil,
			expectedViolation: false,
		},
		{
			name:     "exposure with resolver error",
			feedID:   "post1",
			policies: []string{"exposure-1000"},
			resolver: &mockPolicyResolver{
				err: errors.New("resolver error"),
			},
			expectedViolation: false,
		},
		{
			name:     "inexpose - current time before threshold",
			feedID:   "post1",
			policies: []string{"inexpose-" + string(rune(now+10000))},
			resolver: &mockPolicyResolver{},
			// This test might be flaky with actual time, but demonstrates the logic
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := &mockStore{}
			svc := NewFeed[MockPost](mockStore)

			violation := make(map[string]string)
			svc.checkPolicyViolation(ctx, "test-user", &violation, tt.feedID, tt.policies, tt.resolver)

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
