/** The fields of a ticket card the live clock needs (a TicketCard fits). */
export interface LiveCard {
  hasBudgetDetail: boolean;
  elapsedSeconds: number;
  budgetSeconds: number;
  /** RFC3339: when elapsedSeconds was measured on the server. */
  serverNow?: string;
  /** Time counts right now (not lunch, a Daily Stand Up, evening, weekend, holiday, leave). */
  counting?: boolean;
  /** RFC3339: the next moment counting starts or stops. */
  nextChangeAt?: string;
}

export type LiveLevel = 'ok' | 'warn' | 'over';

export interface LiveTime {
  elapsed: number;
  remaining: number;
  /** Rounded share of the budget used (not capped: 120 = 20% over). */
  percent: number;
  /** For the bar: the used share capped at 100, with decimals so it glides. */
  barPercent: number;
  level: LiveLevel;
  /** The clock is ticking now. */
  running: boolean;
  /** The clock is stopped (time that does not count): the card is shown frozen. */
  frozen: boolean;
  /** The next counting change has passed: the card should be reloaded from the server. */
  stale: boolean;
}

/**
 * The elapsed / remaining time of a running ticket at `nowMs`, ticking by itself
 * between server answers: the server says when it measured the elapsed time,
 * whether time counts now and when that next changes. A card without those
 * fields (older answer, stopped clock) keeps the server's numbers.
 */
export function liveTime(c: LiveCard, nowMs: number): LiveTime {
  const base = c.elapsedSeconds;
  const t0 = c.serverNow ? Date.parse(c.serverNow) : NaN;
  let elapsed = base;
  let running = false;
  let frozen = false;
  let stale = false;
  if (c.hasBudgetDetail && Number.isFinite(t0)) {
    const next = c.nextChangeAt ? Date.parse(c.nextChangeAt) : Infinity;
    stale = nowMs >= next;
    if (c.counting) {
      elapsed = base + Math.max(0, Math.min(nowMs, next) - t0) / 1000;
      running = nowMs < next;
      // Past the end of a counted stretch the clock waits for the fresh answer.
      frozen = !running;
    } else {
      frozen = true;
    }
  }
  elapsed = Math.floor(elapsed);
  const budget = c.budgetSeconds;
  const fraction = budget > 0 ? elapsed / budget : 0;
  const percent = Math.round(fraction * 100);
  return {
    elapsed,
    remaining: budget - elapsed,
    percent,
    barPercent: Math.min(100, Math.round(fraction * 10000) / 100),
    // Over as soon as one second is past the budget (the percent is rounded, the seconds are not).
    level: budget > 0 && elapsed > budget ? 'over' : percent >= 80 ? 'warn' : 'ok',
    running,
    frozen,
    stale,
  };
}
