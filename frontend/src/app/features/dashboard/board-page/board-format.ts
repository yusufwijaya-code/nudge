// Pure display helpers shared by the list and column views.

/** Colour family of a status dot, from its name (Jira workflows differ per project). */
export type StatusTone = 'todo' | 'process' | 'review' | 'done' | 'other';

export function statusTone(status: string): StatusTone {
  const s = status.toLowerCase();
  if (/(complete|done|selesai|closed|resolved)/.test(s)) return 'done';
  if (/review/.test(s)) return 'review';
  if (/(process|progress|develop)/.test(s)) return 'process';
  if (/(to do|todo|backlog|open)/.test(s)) return 'todo';
  return 'other';
}

/**
 * From the largest non-zero unit down to the second; the sign is ignored.
 * 5415 → "1j 30m 15dtk", 2700 → "45m 0dtk", 7200 → "2j 0m 0dtk", 20 → "20dtk".
 */
export function shortDuration(totalSeconds: number): string {
  const total = Math.floor(Math.abs(totalSeconds));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h}j ${m}m ${s}dtk`;
  if (m > 0) return `${m}m ${s}dtk`;
  return `${s}dtk`;
}

/** Elapsed share of the budget, capped at 100 (0 when there is no budget). */
export function budgetPercent(card: { elapsedSeconds: number; budgetSeconds: number }): number {
  if (!card.budgetSeconds) return 0;
  return Math.min(100, Math.round((card.elapsedSeconds / card.budgetSeconds) * 100));
}
