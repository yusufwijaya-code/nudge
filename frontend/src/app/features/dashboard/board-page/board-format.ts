// Pure display helpers shared by the list and column views.
import { formatDuration } from '../../../core/duration-format';

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

/** Words without zero units, e.g. "1 jam 34 menit 20 detik" (see core/duration-format.ts); the sign is ignored. */
export function shortDuration(totalSeconds: number): string {
  return formatDuration(totalSeconds);
}

/** Elapsed share of the budget, capped at 100 (0 when there is no budget). */
export function budgetPercent(card: { elapsedSeconds: number; budgetSeconds: number }): number {
  if (!card.budgetSeconds) return 0;
  return Math.min(100, Math.round((card.elapsedSeconds / card.budgetSeconds) * 100));
}
