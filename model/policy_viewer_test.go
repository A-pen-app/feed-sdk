package model

import (
	"context"
	"errors"
	"testing"
)

type viewerResolver struct {
	mockPolicyResolver
	today    map[string]int64
	todayErr error
	packs    map[string][]string
	packsErr error
	calls    []string
}

func (r *viewerResolver) GetViewerTodayExposure(ctx context.Context, feedID, userID string) (int64, error) {
	r.calls = append(r.calls, "exposure:"+feedID+":"+userID)
	return r.today[feedID], r.todayErr
}

func (r *viewerResolver) IsViewerInAudience(ctx context.Context, userID string, packIDs []string) (bool, error) {
	r.calls = append(r.calls, "audience:"+userID)
	if r.packsErr != nil {
		return false, r.packsErr
	}
	for _, mine := range r.packs[userID] {
		for _, want := range packIDs {
			if mine == want {
				return true, nil
			}
		}
	}
	return false, nil
}

func TestDailyCapPolicy(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		policy   PolicyType
		today    int64
		violated bool
	}{
		{"under cap", "dailycap:2", 1, false},
		{"never seen", "dailycap:2", 0, false},
		{"at cap", "dailycap:2", 2, true},
		{"over cap", "dailycap:2", 5, true},
		{"cap 1 after one view", "dailycap:1", 1, true},
		{"cap 10 at 9", "dailycap:10", 9, false},
		{"zero is invalid", "dailycap:0", 5, false},
		{"eleven is invalid", "dailycap:11", 50, false},
		{"not a number", "dailycap:two", 50, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &viewerResolver{today: map[string]int64{"f1": c.today}}
			if got := c.policy.Violated(ctx, "u1", "f1", r); got != c.violated {
				t.Fatalf("violated=%v want %v", got, c.violated)
			}
		})
	}
}

func TestDailyCapResolverBehaviour(t *testing.T) {
	ctx := context.Background()
	p := PolicyType("dailycap:2")

	if p.Violated(ctx, "u1", "f1", &mockPolicyResolver{}) {
		t.Fatal("a resolver without ExposureResolver must not enforce dailycap")
	}
	if p.Violated(ctx, "u1", "f1", nil) {
		t.Fatal("nil resolver must not enforce dailycap")
	}

	failing := &viewerResolver{todayErr: errors.New("redis down")}
	if p.Violated(ctx, "u1", "f1", failing) {
		t.Fatal("resolver error must fail open by default")
	}
	if !p.Violated(ctx, "u1", "f1", failing, WithFailClosed()) {
		t.Fatal("resolver error must hide the feed with WithFailClosed")
	}

	r := &viewerResolver{today: map[string]int64{"f1": 3}}
	p.Violated(ctx, "u1", "f1", r)
	if len(r.calls) != 1 || r.calls[0] != "exposure:f1:u1" {
		t.Fatalf("resolver must be asked for this feed and viewer, got %v", r.calls)
	}
}

func TestAudiencePolicy(t *testing.T) {
	ctx := context.Background()
	packs := map[string][]string{"u1": {"pack-a"}, "u2": {"pack-b", "pack-c"}, "u3": nil}
	cases := []struct {
		name     string
		policy   PolicyType
		user     string
		violated bool
	}{
		{"in the only pack", "audience:pack-a", "u1", false},
		{"not in the only pack", "audience:pack-a", "u2", true},
		{"union: in second pack", "audience:pack-a:pack-c", "u2", false},
		{"union: in first pack", "audience:pack-a:pack-c", "u1", false},
		{"union: in neither", "audience:pack-a:pack-c", "u3", true},
		{"no packs at all", "audience:pack-a", "u3", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &viewerResolver{packs: packs}
			if got := c.policy.Violated(ctx, c.user, "f1", r); got != c.violated {
				t.Fatalf("violated=%v want %v", got, c.violated)
			}
		})
	}
}

func TestAudienceResolverBehaviour(t *testing.T) {
	ctx := context.Background()
	p := PolicyType("audience:pack-a")

	if p.Violated(ctx, "u9", "f1", &mockPolicyResolver{}) {
		t.Fatal("a resolver without AudienceResolver must not enforce audience")
	}
	failing := &viewerResolver{packsErr: errors.New("db down")}
	if p.Violated(ctx, "u9", "f1", failing) {
		t.Fatal("resolver error must fail open by default")
	}
	if !p.Violated(ctx, "u9", "f1", failing, WithFailClosed()) {
		t.Fatal("resolver error must hide the feed with WithFailClosed")
	}
}

func TestUnknownPolicyStillIgnored(t *testing.T) {
	r := &viewerResolver{today: map[string]int64{"f1": 99}, packs: map[string][]string{}}
	for _, p := range []PolicyType{"weeklycap:1", "segment:pack-a", "dailycap"} {
		if p.Violated(context.Background(), "u1", "f1", r) {
			t.Fatalf("%s must be ignored, not enforced", p)
		}
	}
	if len(r.calls) != 0 {
		t.Fatalf("unknown policies must not reach the resolver, got %v", r.calls)
	}
}
