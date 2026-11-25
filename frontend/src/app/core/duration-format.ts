/**
 * One way to write a duration, everywhere on the dashboard (same rule as the
 * server's duration_helper.Format, used in reminders and chat): words, starting
 * from hours, and any unit that is zero is left out. No days: two days are
 * "48 jam". The sign is ignored; callers say "lewat" / "sisa" themselves.
 *
 *   5660 -> "1 jam 34 menit 20 detik"   1800 -> "30 menit"   3630 -> "1 jam 30 detik"
 *   52 -> "52 detik"                    0 -> "0 detik"      172800 -> "48 jam"
 */
export function formatDuration(totalSeconds: number): string {
  const total = Math.floor(Math.abs(totalSeconds));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  const parts: string[] = [];
  if (hours > 0) parts.push(`${hours} jam`);
  if (minutes > 0) parts.push(`${minutes} menit`);
  if (seconds > 0) parts.push(`${seconds} detik`);
  return parts.length ? parts.join(' ') : '0 detik';
}

/** Decimal hours (the sprint report) in the same words: 2.5 -> "2 jam 30 menit". */
export function formatHours(hours: number): string {
  return formatDuration(Math.round(hours * 3600));
}
