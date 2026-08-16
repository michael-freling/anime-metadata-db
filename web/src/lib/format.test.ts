import { describe, expect, it } from 'vitest';
import { plural, yearsLabel } from './format';

describe('yearsLabel', () => {
  it('collapses a single-year span', () => {
    expect(yearsLabel(2026, 2026)).toBe('2026');
  });

  it('renders a real span', () => {
    expect(yearsLabel(2019, 2025)).toBe('2019–2025');
  });

  // A dataset entry with no release year must not render as "0" or "0–0";
  // plenty of works in the catalogue carry no year at all.
  it('renders a dash when nothing carries a year', () => {
    expect(yearsLabel(0, 0)).toBe('—');
  });

  it('falls back to whichever year exists', () => {
    expect(yearsLabel(2020, 0)).toBe('2020');
    expect(yearsLabel(0, 2020)).toBe('2020');
  });
});

describe('plural', () => {
  // The bug this exists to prevent: one-episode entries are common, so the
  // catalogue was rendering "1 episodes" all over.
  it('uses the singular for exactly one', () => {
    expect(plural(1, 'episode')).toBe('1 episode');
    expect(plural(1, 'work')).toBe('1 work');
  });

  it('uses the plural otherwise', () => {
    expect(plural(0, 'episode')).toBe('0 episodes');
    expect(plural(2, 'work')).toBe('2 works');
  });

  it('groups large counts', () => {
    expect(plural(3229, 'episode')).toBe('3,229 episodes');
  });

  it('accepts an irregular plural', () => {
    expect(plural(2, 'series', 'series')).toBe('2 series');
  });
});
