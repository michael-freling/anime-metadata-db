// Pure display helpers for dataset values. Kept out of the component module so
// they can be unit-tested without dragging React and next/link into the test
// environment.

// yearsLabel renders a span of release years, collapsing a single-year span and
// saying nothing at all when the dataset carries no year — a work with no year
// must not render as "0".
export function yearsLabel(first: number, latest: number): string {
  if (!first && !latest) return '—';
  // Either end may be missing on its own; a span is only meaningful with both,
  // otherwise the absent end renders as a literal 0 ("0–2020").
  if (!first || !latest || first === latest) return String(first || latest);
  return `${first}–${latest}`;
}

// plural renders a count with its noun. Every count on the browse pages comes
// straight from the dataset and one-episode entries are common, so "1 episodes"
// would show up all over the catalogue without this.
export function plural(count: number, noun: string, pluralForm = `${noun}s`): string {
  return `${count.toLocaleString()} ${count === 1 ? noun : pluralForm}`;
}
