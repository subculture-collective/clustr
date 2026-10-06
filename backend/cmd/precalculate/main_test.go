package main

import "testing"

func TestPositiveIntEnv(t *testing.T) {
	t.Setenv("PRECALC_TEST_POOL", "7")
	if got := positiveIntEnv("PRECALC_TEST_POOL", 5); got != 7 {
		t.Fatalf("got %d", got)
	}
	for _, value := range []string{"0", "-1", "invalid"} {
		t.Setenv("PRECALC_TEST_POOL", value)
		if got := positiveIntEnv("PRECALC_TEST_POOL", 5); got != 5 {
			t.Fatalf("value %q yielded %d", value, got)
		}
	}
}

func TestForceClearEnabled(t *testing.T) {
	for _, test := range []struct {
		value string
		want  bool
	}{
		{value: "true", want: true},
		{value: "1", want: true},
		{value: "false", want: false},
		{value: "", want: false},
	} {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv("PRECALC_FORCE_CLEAR", test.value)
			if got := forceClearEnabled(); got != test.want {
				t.Fatalf("forceClearEnabled() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestForceClearRunOptions(t *testing.T) {
	if !effectiveFullRebuild(false, true) {
		t.Fatal("force clear did not imply an effective full rebuild")
	}
	if effectiveFullRebuild(false, false) {
		t.Fatal("empty rebuild request unexpectedly became full")
	}
	if err := validateRunOptions(true, true); err == nil {
		t.Fatal("force clear with publish-only was accepted")
	}
	if err := validateRunOptions(false, true); err != nil {
		t.Fatalf("force clear without publish-only rejected: %v", err)
	}
}
