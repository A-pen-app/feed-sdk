package model

import (
	"testing"

	"github.com/lib/pq"
)

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
	policy := Policy{
		FeedId:   "feed123",
		FeedType: TypePost,
		Position: 5,
		Policies: pq.StringArray{"exposure-1000", "inexpose-1234567890"},
	}

	if policy.FeedId != "feed123" {
		t.Errorf("expected FeedId 'feed123', got '%s'", policy.FeedId)
	}
	if policy.FeedType != TypePost {
		t.Errorf("expected FeedType 'post', got '%s'", policy.FeedType)
	}
	if policy.Position != 5 {
		t.Errorf("expected Position 5, got %d", policy.Position)
	}
	if len(policy.Policies) != 2 {
		t.Errorf("expected 2 policies, got %d", len(policy.Policies))
	}
}

func TestFeedStruct(t *testing.T) {
	mockData := MockPost{
		id:       "data123",
		feedType: TypePost,
		score:    100.0,
	}

	feed := Feed[MockPost]{
		ID:   "feed123",
		Type: TypePost,
		Data: mockData,
	}

	if feed.ID != "feed123" {
		t.Errorf("expected ID 'feed123', got '%s'", feed.ID)
	}
	if feed.Type != TypePost {
		t.Errorf("expected Type 'post', got '%s'", feed.Type)
	}
	if feed.Data.GetID() != "data123" {
		t.Errorf("expected Data ID 'data123', got '%s'", feed.Data.GetID())
	}
	if feed.Data.Score() != 100.0 {
		t.Errorf("expected Data Score 100.0, got %f", feed.Data.Score())
	}
}
