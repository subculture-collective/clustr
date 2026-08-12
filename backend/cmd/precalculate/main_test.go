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
