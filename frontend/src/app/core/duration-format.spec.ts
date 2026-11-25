import { formatDuration, formatHours } from './duration-format';

describe('formatDuration', () => {
  it('reads like a stopwatch: H:MM:SS, then M:SS, then seconds only', () => {
    expect(formatDuration(8948)).toBe('2:29:08');
    expect(formatDuration(9000)).toBe('2:30:00');
    expect(formatDuration(3600)).toBe('1:00:00');
    expect(formatDuration(3599)).toBe('59:59');
    expect(formatDuration(2233)).toBe('37:13');
    expect(formatDuration(2220)).toBe('37:00');
    expect(formatDuration(60)).toBe('1:00');
    expect(formatDuration(59)).toBe('59 detik');
    expect(formatDuration(7)).toBe('7 detik');
    expect(formatDuration(0)).toBe('0 detik');
  });

  it('never shows days: the hours keep counting', () => {
    // "1 hari 21 jam 52 menit 36 detik" in the sprint summary
    expect(formatDuration(24 * 3600 + 21 * 3600 + 52 * 60 + 36)).toBe('45:52:36');
    expect(formatDuration(1_000_000)).toBe('277:46:40');
  });

  it('ignores the sign and the fraction of a second', () => {
    expect(formatDuration(-8948)).toBe('2:29:08');
    expect(formatDuration(52.9)).toBe('52 detik');
  });

  it('writes decimal hours the same way', () => {
    expect(formatHours(2.5)).toBe('2:30:00');
    expect(formatHours(2 + 29 / 60 + 8 / 3600)).toBe('2:29:08');
    expect(formatHours(0)).toBe('0 detik');
  });
});
