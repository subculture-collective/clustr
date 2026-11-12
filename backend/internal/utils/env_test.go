package utils

import "testing"

func TestAtoi32(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
		ok   bool
	}{
		{"0", 0, true},
		{"-7", -7, true},
		{"2147483647", 2147483647, true},
		{"2147483648", 0, false},
		{"-2147483649", 0, false},
		{"4294967297", 0, false},
		{"", 0, false},
		{"12abc", 0, false},
	} {
		got, err := Atoi32(tc.in)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("Atoi32(%q) = %d, %v; want %d, ok=%v", tc.in, got, err, tc.want, tc.ok)
		}
	}
}

func TestGetEnvAsIntRejectsOutOfRange(t *testing.T) {
	t.Setenv("ATOI32_TEST_VALUE", "4294967297")
	if got := GetEnvAsInt("ATOI32_TEST_VALUE", 25); got != 25 {
		t.Errorf("GetEnvAsInt = %d; want the default 25", got)
	}
}
