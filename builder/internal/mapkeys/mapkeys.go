// Package mapkeys orders map keys so that output built by ranging over a map
// is the same on every run.
//
// Go randomises map iteration, so a report or log line assembled straight from
// a range is a different line each time — which turns a diff of two builds into
// noise and makes a CI failure irreproducible. Both the build reports and the
// builder's own progress output need that guarantee over maps with different
// key types, so the ordering lives here rather than being written once per
// package.
package mapkeys

import (
	"cmp"
	"sort"
)

// Sorted returns a map's keys in ascending order.
func Sorted[K cmp.Ordered, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
