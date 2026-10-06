package macrocluster

import "testing"

func TestRepairFixesDuplicateAndMissing(t *testing.T) {
	// 282 communities, 8 groups, index 204 duplicated (groups 2 and 5),
	// index 150 missing.
	groups := make([][]int, 8)
	for i := 0; i < 8; i++ {
		groups[i] = []int{}
	}
	for i := 0; i < 282; i++ {
		if i == 150 {
			continue // missing
		}
		g := i % 8
		groups[g] = append(groups[g], i)
	}
	groups[5] = append(groups[5], 204) // duplicate

	repaired, err := repair(groups, 282)
	if err != nil {
		t.Fatalf("repair failed: %v", err)
	}
	if err := Validate(repaired, 282); err != nil {
		t.Fatalf("repaired partition invalid: %v", err)
	}
	count := map[int]int{}
	for _, group := range repaired {
		for _, index := range group {
			count[index]++
		}
	}
	if count[204] != 1 {
		t.Fatalf("index 204 appears %d times, want 1", count[204])
	}
	if count[150] != 1 {
		t.Fatalf("index 150 appears %d times, want 1", count[150])
	}
}

func TestRepairRejectsWrongGroupCount(t *testing.T) {
	// 8 communities but only 2 groups -> repair should fail.
	if _, err := repair([][]int{{0, 1, 2, 3}, {4, 5, 6, 7}}, 8); err == nil {
		t.Fatal("expected error for too few groups")
	}
}
