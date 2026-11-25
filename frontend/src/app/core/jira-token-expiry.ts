/**
 * Expiry of a Jira API token. The token string is opaque and Jira's API does
 * not report when it expires, so the user (or an admin) enters the date the
 * Atlassian "API tokens" page shows; this turns that day into how close it is.
 */

/** Days before the expiry day that a token starts to warn (same as the server's ExpiryWarnDays). */
export const JIRA_TOKEN_WARN_DAYS = 14;

export type TokenExpiryLevel = 'unknown' | 'ok' | 'soon' | 'expired';

export interface TokenExpiry {
  level: TokenExpiryLevel;
  /** Whole WIB calendar days from today to the expiry day; negative once expired; null when unknown. */
  daysLeft: number | null;
}

const WIB = 'Asia/Jakarta';
const DAY_MS = 86_400_000;

/** "YYYY-MM-DD" → UTC midnight of that calendar day; NaN when not a real date. */
function dayStart(isoDay: string): number {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(isoDay.trim());
  if (!match) return NaN;
  const [, y, m, d] = match;
  const at = Date.UTC(Number(y), Number(m) - 1, Number(d));
  const back = new Date(at);
  // 2027-02-31 rolls over to March and 2026-13-01 to January: not real days.
  return back.getUTCMonth() === Number(m) - 1 && back.getUTCDate() === Number(d) ? at : NaN;
}

/** How close `expiresAt` ("YYYY-MM-DD", WIB) is: expired once its day has passed, soon within the warning window. */
export function tokenExpiry(expiresAt: string | null | undefined, now: Date = new Date()): TokenExpiry {
  if (!expiresAt) return { level: 'unknown', daysLeft: null };
  const target = dayStart(expiresAt);
  if (Number.isNaN(target)) return { level: 'unknown', daysLeft: null };

  const today = dayStart(now.toLocaleDateString('en-CA', { timeZone: WIB }));
  const daysLeft = Math.round((target - today) / DAY_MS);
  if (daysLeft < 0) return { level: 'expired', daysLeft };
  return { level: daysLeft <= JIRA_TOKEN_WARN_DAYS ? 'soon' : 'ok', daysLeft };
}

/** "2026-11-30" → "30 Nov 2026" (fixed Indonesian month names, no locale dependency). */
export function expiryDateLabel(isoDay: string): string {
  const months = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des'];
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(isoDay.trim());
  if (!match) return isoDay;
  return `${Number(match[3])} ${months[Number(match[2]) - 1]} ${match[1]}`;
}

/** "5 hari lagi", "hari ini", "kemarin", "3 hari lalu" — the distance in words. */
export function expiryDistance(daysLeft: number): string {
  if (daysLeft === 0) return 'hari ini';
  if (daysLeft === 1) return 'besok';
  if (daysLeft > 1) return `${daysLeft} hari lagi`;
  if (daysLeft === -1) return 'kemarin';
  return `${-daysLeft} hari lalu`;
}
