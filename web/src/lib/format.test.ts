import { describe, expect, it } from 'vitest';
import { humanizeId, plural, yearsLabel } from './format';

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

  // The floor is the earliest year the dataset actually covers. A year below it
  // cannot be real data for this dataset, so it must not be rendered.
  describe('with a dataset floor', () => {
    const FLOOR = 2006;

    it('renders years at or above the floor', () => {
      expect(yearsLabel(2006, 2026, FLOOR)).toBe('2006–2026');
      expect(yearsLabel(2006, 2006, FLOOR)).toBe('2006');
    });

    it('drops a start below the floor rather than showing it', () => {
      expect(yearsLabel(1900, 2026, FLOOR)).toBe('2026');
      expect(yearsLabel(0, 2026, FLOOR)).toBe('2026');
    });

    it('drops an end below the floor', () => {
      expect(yearsLabel(2020, 1200, FLOOR)).toBe('2020');
    });

    it('renders a dash when neither year clears the floor', () => {
      expect(yearsLabel(12, 1900, FLOOR)).toBe('—');
    });

    // Omitting the floor must not start hiding real years.
    it('defaults to no floor', () => {
      expect(yearsLabel(1917, 1918)).toBe('1917–1918');
    });
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

describe('humanizeId', () => {
  it('turns a slug into words', () => {
    expect(humanizeId('demon-slayer')).toBe('Demon Slayer');
    expect(humanizeId('tanjiro-kamado')).toBe('Tanjiro Kamado');
  });

  it('leaves numeric segments alone', () => {
    expect(humanizeId('mob-psycho-100')).toBe('Mob Psycho 100');
  });

  it('survives odd slugs without producing empty words', () => {
    expect(humanizeId('a--b')).toBe('A B');
    expect(humanizeId('')).toBe('');
    expect(humanizeId('single')).toBe('Single');
  });
});
