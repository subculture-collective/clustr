package graph

import "testing"

func TestAutomationPolicyManualOverrideWinsBeforeKnownAndHeuristicRules(t *testing.T) {
	policy := NewAutomationPolicy([]string{"AutoModerator", "NewsHelper"})

	if got := policy.Classify("AutoModerator", 1, boolPointer(false)); got.Automated || got.Reason != "manual-allow" {
		t.Fatalf("manual allow classification = %#v", got)
	}
	if got := policy.Classify("NewsHelper", 1, nil); !got.Automated || got.Reason != "known-list" {
		t.Fatalf("known bot classification = %#v", got)
	}
	if got := policy.Classify("travel_bot", 20, nil); !got.Automated || got.Reason != "broad-bot-name" {
		t.Fatalf("heuristic bot classification = %#v", got)
	}
	if got := policy.Classify("robotics_fan", 100, nil); got.Automated {
		t.Fatalf("human classification = %#v", got)
	}
}

func boolPointer(value bool) *bool { return &value }
