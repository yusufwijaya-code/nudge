/**
 * One way to write a duration, everywhere on the dashboard (same rule as the
 * server's duration_helper.Clock, used in reminders and chat): a stopwatch
 * reading H:MM:SS, M:SS when there is no full hour, and "S detik" when there is
 * no full minute either. No days: 45 hours is "45:00:00". The sign is ignored;
 * callers say "lewat" / "sisa" themselves.
 *
 *   8948 → "2:29:08"   2220 → "37:00"   52 → "52 detik"   0 → "0 detik"
 */
export function formatDuration(totalSeconds: number): string {
  const total = Math.floor(Math.abs(totalSeconds));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  const two = (n: number) => String(n).padStart(2, '0');
  if (hours > 0) return `${hours}:${two(minutes)}:${two(seconds)}`;
  if (minutes > 0) return `${minutes}:${two(seconds)}`;
  return `${seconds} detik`;
}

/** Decimal hours (the sprint report) in the same reading: 2.5 → "2:30:00". */
export function formatHours(hours: number): string {
  return formatDuration(Math.round(hours * 3600));
}
