// Pure display helpers for dataset values. Kept out of the component module so
// they can be unit-tested without dragging React and next/link into the test
// environment.

// yearsLabel renders a span of release years.
//
// `floor` is the earliest year the dataset actually covers, reported by the API
// (DatasetStats.earliestReleaseYear). Anything below it cannot be real data for
// this dataset — a missing year arrives as 0, and a mistyped one would be some
// other implausible value — so such years are treated as absent rather than
// rendered. Taking the floor from the dataset rather than hardcoding it matters
// because the catalogue grows backwards as earlier seasons are added: a frozen
// constant would start hiding genuine years.
//
// Note the limit of this rule: because the floor IS the dataset's minimum, it
// cannot reject a single bogus outlier, which would simply become the new
// minimum. Catching that belongs in the builder's validation, where a year that
// could not be a real release should fail the build.
export function yearsLabel(first: number, latest: number, floor = 0): string {
  const start = first >= floor ? first : 0;
  const end = latest >= floor ? latest : 0;

  if (!start && !end) return '—';
  // Either end may be missing on its own; a span is only meaningful with both,
  // otherwise the absent end renders as a literal 0 ("0–2020").
  if (!start || !end || start === end) return String(start || end);
  return `${start}–${end}`;
}

// plural renders a count with its noun. Every count on the browse pages comes
// straight from the dataset and one-episode entries are common, so "1 episodes"
// would show up all over the catalogue without this.
export function plural(count: number, noun: string, pluralForm = `${noun}s`): string {
  return `${count.toLocaleString()} ${count === 1 ? noun : pluralForm}`;
}
