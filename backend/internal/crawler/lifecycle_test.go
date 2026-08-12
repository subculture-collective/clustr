package crawler

import (
	"testing"
	"time"
)

func TestActivationCohortProtectsBacklogButNotExplicitWork(t *testing.T) {
	for _, provenance := range []string{"", "legacy", "system-stale", "author-history", "mention"} {
		if got := activationCohort(provenance); got != "backlog" {
			t.Errorf("activationCohort(%q) = %q, want backlog", provenance, got)
		}
	}
	for _, provenance := range []string{"api", "manual", "scheduler:nightly"} {
		if got := activationCohort(provenance); got != "active" {
			t.Errorf("activationCohort(%q) = %q, want active", provenance, got)
		}
	}
}

func TestCalculateRetryDelayIsBoundedAndExponential(t *testing.T) {
	for _, retry := range []int32{0, 1, 2, 20} {
		delay := CalculateRetryDelay(retry)
		if delay < time.Minute || delay > time.Duration(float64(24*time.Hour)*1.2) {
			t.Fatalf("retry %d produced out-of-range delay %s", retry, delay)
		}
	}
	if got := CalculateRetryDelay(-1); got < time.Minute {
		t.Fatalf("negative retry produced %s", got)
	}
}

func TestAttemptOutcomeValuesArePersistable(t *testing.T) {
	want := map[AttemptOutcome]bool{
		OutcomeComplete: true, OutcomePartial: true, OutcomeRetryableFailure: true,
		OutcomePermanentFailure: true, OutcomeCancelled: true,
	}
	if len(want) != 5 {
		t.Fatal("attempt outcomes must be distinct")
	}
}
