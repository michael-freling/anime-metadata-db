package mapkeys

import "testing"

// Sorted exists so that output built by ranging over a map is reproducible.
// Go randomises map iteration, so a test that ordered one map once could pass
// by luck; these use enough keys, inserted out of order, that an unsorted
// result would have to be a coincidence rather than a near-miss.
func TestSorted(t *testing.T) {
	ints := Sorted(map[int]string{30: "c", 10: "a", 20: "b", 5: "z", 99: "q"})
	if want := []int{5, 10, 20, 30, 99}; !equal(ints, want) {
		t.Errorf("int keys: got %v, want %v", ints, want)
	}

	// The value type is irrelevant to the ordering, and both call sites use a
	// different one, so it is worth pinning that the constraint is on the key.
	strs := Sorted(map[string]int{"pear": 1, "apple": 2, "fig": 3})
	if want := []string{"apple", "fig", "pear"}; !equal(strs, want) {
		t.Errorf("string keys: got %v, want %v", strs, want)
	}

	// Empty rather than nil: the result is ranged over directly at both call
	// sites, and a length-0 slice keeps that a no-op without a nil check.
	if got := Sorted(map[int]int{}); got == nil || len(got) != 0 {
		t.Errorf("an empty map gives an empty slice, got %#v", got)
	}
}

func equal[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
