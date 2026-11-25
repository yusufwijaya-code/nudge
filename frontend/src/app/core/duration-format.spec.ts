import { formatDuration, formatHours } from './duration-format';

describe('formatDuration', () => {
  it('writes words from hours down and leaves out every zero unit', () => {
    expect(formatDuration(5660)).toBe('1 jam 34 menit 20 detik');
    expect(formatDuration(8948)).toBe('2 jam 29 menit 8 detik');
    expect(formatDuration(9000)).toBe('2 jam 30 menit');
    expect(formatDuration(3600)).toBe('1 jam');
    expect(formatDuration(3630)).toBe('1 jam 30 detik');
    expect(formatDuration(2220)).toBe('37 menit');
    expect(formatDuration(1800)).toBe('30 menit');
    expect(formatDuration(2233)).toBe('37 menit 13 detik');
    expect(formatDuration(60)).toBe('1 menit');
    expect(formatDuration(52)).toBe('52 detik');
    expect(formatDuration(0)).toBe('0 detik');
  });

  it('never shows days: two days are 48 hours', () => {
    expect(formatDuration(2 * 24 * 3600)).toBe('48 jam');
    // "1 hari 21 jam 52 menit 36 detik" in the sprint summary
    expect(formatDuration(24 * 3600 + 21 * 3600 + 52 * 60 + 36)).toBe('45 jam 52 menit 36 detik');
    expect(formatDuration(1_000_000)).toBe('277 jam 46 menit 40 detik');
  });

  it('ignores the sign and the fraction of a second', () => {
    expect(formatDuration(-8948)).toBe('2 jam 29 menit 8 detik');
    expect(formatDuration(52.9)).toBe('52 detik');
  });

  it('writes decimal hours the same way', () => {
    expect(formatHours(2.5)).toBe('2 jam 30 menit');
    expect(formatHours(2 + 29 / 60 + 8 / 3600)).toBe('2 jam 29 menit 8 detik');
    expect(formatHours(0)).toBe('0 detik');
  });
});
