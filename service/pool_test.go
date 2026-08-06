package service

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/A-pen-app/feed-sdk/model"
	"github.com/lib/pq"
)

// poolService builds a service whose store serves the given candidates for
// pool "pool-1".
func poolService(candidates []model.PoolCandidate) *Service[MockPost] {
	return NewFeed[MockPost](&mockStore{
		poolCandidates: map[string][]model.PoolCandidate{"pool-1": candidates},
	})
}

func TestPickFromPool_SingleCandidateAlwaysWins(t *testing.T) {
	ctx := context.Background()
	svc := poolService([]model.PoolCandidate{
		{FeedID: "post-a", Policies: pq.StringArray{}, Weight: 100},
	})
	resolver := &mockPolicyResolver{}

	picked, err := svc.PickFromPool(ctx, "user-1", "pool-1", resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if picked != "post-a" {
		t.Fatalf("expected post-a, got %q", picked)
	}
}

func TestPickFromPool_Sticky(t *testing.T) {
	ctx := context.Background()
	svc := poolService([]model.PoolCandidate{
		{FeedID: "post-a", Weight: 100},
		{FeedID: "post-b", Weight: 100},
		{FeedID: "post-c", Weight: 100},
	})
	resolver := &mockPolicyResolver{}

	first, err := svc.PickFromPool(ctx, "user-42", "pool-1", resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 0; i < 100; i++ {
		again, err := svc.PickFromPool(ctx, "user-42", "pool-1", resolver)
		if err != nil {
			t.Fatalf("unexpected error on run %d: %v", i, err)
		}
		if again != first {
			t.Fatalf("draw not sticky: run %d picked %q, first run picked %q", i, again, first)
		}
	}
}

func TestPickFromPool_WeightDistribution(t *testing.T) {
	ctx := context.Background()
	// 3:1 weights — expect roughly 75%/25% across a synthetic population.
	svc := poolService([]model.PoolCandidate{
		{FeedID: "post-heavy", Weight: 300},
		{FeedID: "post-light", Weight: 100},
	})
	resolver := &mockPolicyResolver{}

	counts := map[string]int{}
	const population = 5000
	for i := 0; i < population; i++ {
		picked, err := svc.PickFromPool(ctx, "user-"+strconv.Itoa(i), "pool-1", resolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[picked]++
	}

	heavyShare := float64(counts["post-heavy"]) / population
	lightShare := float64(counts["post-light"]) / population
	if heavyShare < 0.70 || heavyShare > 0.80 {
		t.Errorf("heavy candidate share %.3f outside [0.70, 0.80]", heavyShare)
	}
	if lightShare < 0.20 || lightShare > 0.30 {
		t.Errorf("light candidate share %.3f outside [0.20, 0.30]", lightShare)
	}
}

func TestPickFromPool_ExpiredCandidateNeverPicked(t *testing.T) {
	ctx := context.Background()
	// post-a's window ended at unix second 1 — violated for everyone, forever.
	svc := poolService([]model.PoolCandidate{
		{FeedID: "post-a", Policies: pq.StringArray{"unexpose:1"}, Weight: 1000000},
		{FeedID: "post-b", Weight: 1},
	})
	resolver := &mockPolicyResolver{}

	for i := 0; i < 200; i++ {
		picked, err := svc.PickFromPool(ctx, fmt.Sprintf("user-%d", i), "pool-1", resolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if picked != "post-b" {
			t.Fatalf("expired candidate got picked for user-%d", i)
		}
	}
}

func TestPickFromPool_AudienceMismatchNeverPicked(t *testing.T) {
	ctx := context.Background()
	svc := poolService([]model.PoolCandidate{
		{FeedID: "post-nurse", Policies: pq.StringArray{"istarget:nurse"}, Weight: 1000000},
		{FeedID: "post-anyone", Weight: 1},
	})
	// user-1 is a doctor: istarget:nurse is violated for them.
	resolver := &mockPolicyResolver{
		userAttrs: map[string][]string{"user-1": {"doctor"}},
	}

	picked, err := svc.PickFromPool(ctx, "user-1", "pool-1", resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if picked != "post-anyone" {
		t.Fatalf("audience-mismatched candidate got picked: %q", picked)
	}

	// A nurse, by contrast, can land on either — assert the targeted candidate
	// is at least reachable for them (weight 1000000:1 makes it near-certain).
	nurseResolver := &mockPolicyResolver{
		userAttrs: map[string][]string{"user-2": {"nurse"}},
	}
	picked, err = svc.PickFromPool(ctx, "user-2", "pool-1", nurseResolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if picked != "post-nurse" {
		t.Fatalf("expected the heavily-weighted targeted candidate for a nurse, got %q", picked)
	}
}

func TestPickFromPool_ZeroWeightIsPaused(t *testing.T) {
	ctx := context.Background()
	svc := poolService([]model.PoolCandidate{
		{FeedID: "post-paused", Weight: 0},
		{FeedID: "post-negative", Weight: -5},
		{FeedID: "post-live", Weight: 1},
	})
	resolver := &mockPolicyResolver{}

	for i := 0; i < 100; i++ {
		picked, err := svc.PickFromPool(ctx, fmt.Sprintf("user-%d", i), "pool-1", resolver)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if picked != "post-live" {
			t.Fatalf("paused candidate got picked for user-%d: %q", i, picked)
		}
	}
}

func TestPickFromPool_NoSurvivorsReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	svc := poolService([]model.PoolCandidate{
		{FeedID: "post-a", Policies: pq.StringArray{"unexpose:1"}, Weight: 100},
		{FeedID: "post-b", Weight: 0},
	})
	resolver := &mockPolicyResolver{}

	picked, err := svc.PickFromPool(ctx, "user-1", "pool-1", resolver)
	if err != nil {
		t.Fatalf("expected nil error for empty survivor set, got %v", err)
	}
	if picked != "" {
		t.Fatalf("expected empty pick, got %q", picked)
	}
}

func TestPickFromPool_EmptyPoolReturnsEmpty(t *testing.T) {
	ctx := context.Background()
	svc := poolService(nil)
	resolver := &mockPolicyResolver{}

	picked, err := svc.PickFromPool(ctx, "user-1", "pool-1", resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if picked != "" {
		t.Fatalf("expected empty pick for empty pool, got %q", picked)
	}
}

func TestPickFromPool_ScheduledCandidateBecomesEligible(t *testing.T) {
	ctx := context.Background()
	// post-b's window opens an hour from now: only post-a is eligible today.
	future := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	svc := poolService([]model.PoolCandidate{
		{FeedID: "post-a", Weight: 1},
		{FeedID: "post-b", Policies: pq.StringArray{"inexpose:" + future}, Weight: 1000000},
	})
	resolver := &mockPolicyResolver{}

	picked, err := svc.PickFromPool(ctx, "user-1", "pool-1", resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if picked != "post-a" {
		t.Fatalf("not-yet-open candidate got picked: %q", picked)
	}
}
