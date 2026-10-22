package heartbeat

import (
	"testing"
	"time"

	"boAIrd/internal/agent"
)

func TestDeriveFollowsDeclaredPrecedence(t *testing.T) {
	policy, err := NewPolicy(90*time.Second, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	seen := func(offset time.Duration) *time.Time {
		value := base.Add(offset)
		return &value
	}
	open := Activity{Sessions: 1, SessionID: "session-1"}

	cases := []struct {
		name     string
		activity Activity
		want     agent.Status
	}{
		{name: "never connected", activity: Activity{}, want: agent.StatusIdle},
		{name: "previous session expired", activity: Activity{Sessions: 2}, want: agent.StatusOffline},
		{name: "fresh session", activity: withSeen(open, seen(0)), want: agent.StatusOnline},
		{name: "one nanosecond before stale", activity: withSeen(open, seen(-(90*time.Second - time.Nanosecond))), want: agent.StatusOnline},
		{name: "stale boundary", activity: withSeen(open, seen(-90*time.Second)), want: agent.StatusStale},
		{name: "one nanosecond before offline", activity: withSeen(open, seen(-(5*time.Minute - time.Nanosecond))), want: agent.StatusStale},
		{name: "offline boundary", activity: withSeen(open, seen(-5*time.Minute)), want: agent.StatusOffline},
		{name: "working task", activity: withWork(withSeen(open, seen(0)), 1, 0, nil), want: agent.StatusBusy},
		{name: "review task", activity: withWork(withSeen(open, seen(0)), 0, 1, nil), want: agent.StatusWaiting},
		{name: "recent failure", activity: withWork(withSeen(open, seen(0)), 0, 0, seen(-30*time.Second)), want: agent.StatusError},
		{name: "old failure", activity: withWork(withSeen(open, seen(0)), 0, 0, seen(-2*time.Minute)), want: agent.StatusOnline},
		{name: "silence outranks work", activity: withWork(withSeen(open, seen(-2*time.Minute)), 1, 1, seen(-time.Minute)), want: agent.StatusStale},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			observed := Derive(base, policy, testCase.activity)
			if observed.Status != testCase.want {
				t.Fatalf("Derive() = %s, want %s", observed.Status, testCase.want)
			}
		})
	}
}

func TestNewPolicyRejectsUnorderedThresholds(t *testing.T) {
	for name, values := range map[string][2]time.Duration{
		"stale below minimum":   {500 * time.Millisecond, time.Minute},
		"stale equals offline":  {time.Minute, time.Minute},
		"stale above offline":   {2 * time.Minute, time.Minute},
		"offline above maximum": {time.Minute, 25 * time.Hour},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewPolicy(values[0], values[1]); err == nil {
				t.Fatal("NewPolicy() accepted unordered thresholds")
			}
		})
	}
}

func withSeen(activity Activity, seen *time.Time) Activity {
	activity.LastSeenAt = seen
	return activity
}

func withWork(activity Activity, working, review int, failedAt *time.Time) Activity {
	activity.Working, activity.Review, activity.FailedAt = working, review, failedAt
	return activity
}
