import { LiveCard, liveTime, targetPercent } from './live-time';

const T0 = Date.parse('2026-11-26T10:00:00+07:00');
const at = (s: number) => T0 + s * 1000;

const running = (over: Partial<LiveCard> = {}): LiveCard => ({
  hasBudgetDetail: true, elapsedSeconds: 3600, budgetSeconds: 14400,
  serverNow: '2026-11-26T10:00:00+07:00', counting: true, nextChangeAt: '2026-11-26T12:00:00+07:00', ...over,
});

describe('liveTime', () => {
  it('counts one second per second while time counts', () => {
    expect(liveTime(running(), at(0)).elapsed).toBe(3600);
    const t = liveTime(running(), at(90));
    expect(t.elapsed).toBe(3690);
    expect(t.remaining).toBe(14400 - 3690);
    expect(t.running).toBeTrue();
    expect(t.frozen).toBeFalse();
    expect(t.stale).toBeFalse();
  });

  it('never runs backwards when the clock is a hair behind the answer', () => {
    expect(liveTime(running(), at(-2)).elapsed).toBe(3600);
  });

  it('stops at the end of the counted stretch (lunch) and asks for fresh data', () => {
    const t = liveTime(running(), at(2 * 3600 + 30));
    expect(t.elapsed).toBe(3600 + 2 * 3600); // 7200 s of stretch, not a second more
    expect(t.running).toBeFalse();
    expect(t.frozen).toBeTrue();
    expect(t.stale).toBeTrue();
  });

  it('stays frozen while time does not count, then asks for fresh data when it resumes', () => {
    const paused = running({ counting: false, nextChangeAt: '2026-11-26T13:00:00+07:00' });
    const during = liveTime(paused, at(600));
    expect(during.elapsed).toBe(3600);
    expect(during.frozen).toBeTrue();
    expect(during.running).toBeFalse();
    expect(during.stale).toBeFalse();
    expect(liveTime(paused, at(3 * 3600 + 1)).stale).toBeTrue();
  });

  it('a pause with no known end stays frozen and never asks for a reload', () => {
    const t = liveTime(running({ counting: false, nextChangeAt: undefined }), at(99999));
    expect(t.frozen).toBeTrue();
    expect(t.stale).toBeFalse();
  });

  it('derives percent, bar and level from the live elapsed time', () => {
    const near = liveTime(running({ elapsedSeconds: 11500 }), at(0));
    expect(near.percent).toBe(80);
    expect(near.level).toBe('warn');
    const over = liveTime(running({ elapsedSeconds: 14390 }), at(30));
    expect(over.percent).toBe(100); // 14420 / 14400, rounded; the level still says over
    expect(liveTime(running({ elapsedSeconds: 14400 }), at(0)).level).toBe('warn');
    expect(over.level).toBe('over');
    expect(over.barPercent).toBe(100);
    expect(over.remaining).toBe(-20);
    expect(liveTime(running(), at(0)).barPercent).toBe(25);
  });

  it('keeps the server numbers for a card without the live fields or without a running clock', () => {
    const old = liveTime({ hasBudgetDetail: true, elapsedSeconds: 600, budgetSeconds: 3600 }, at(500));
    expect(old.elapsed).toBe(600);
    expect(old.running).toBeFalse();
    expect(old.frozen).toBeFalse();
    const stopped = liveTime({ hasBudgetDetail: false, elapsedSeconds: 600, budgetSeconds: 3600, serverNow: '2026-11-26T10:00:00+07:00', counting: true }, at(500));
    expect(stopped.elapsed).toBe(600);
    expect(stopped.frozen).toBeFalse();
  });

  it('copes with no budget', () => {
    const t = liveTime({ hasBudgetDetail: true, elapsedSeconds: 0, budgetSeconds: 0 }, at(0));
    expect(t.percent).toBe(0);
    expect(t.barPercent).toBe(0);
  });
});

describe('targetPercent', () => {
  it('is 100% anywhere inside the allowed deviation, above or below the budget', () => {
    expect(targetPercent(3600, 3600, 12)).toBe(100);
    expect(targetPercent(3600 * 0.88, 3600, 12)).toBe(100);
    expect(targetPercent(3600 * 1.12, 3600, 12)).toBe(100);
  });
  it('falls as cap ÷ deviation × 100 outside it', () => {
    expect(targetPercent(3600 * 1.24, 3600, 12)).toBe(50); // 24% off, cap 12
    expect(targetPercent(3600 * 0.76, 3600, 12)).toBe(50); // early counts the same as late
    expect(targetPercent(0, 3600, 12)).toBe(12); // 100% off
  });
  it('follows the cap an admin sets', () => {
    expect(targetPercent(3600 * 1.2, 3600, 12)).toBe(60);
    expect(targetPercent(3600 * 1.2, 3600, 25)).toBe(100);
  });
  it('is 0 without a budget', () => expect(targetPercent(100, 0, 12)).toBe(0));
});

describe('liveTime target moves with the clock', () => {
  const card: LiveCard = {
    hasBudgetDetail: true, elapsedSeconds: 1800, budgetSeconds: 3600,
    serverNow: '2026-11-26T10:00:00+07:00', counting: true, nextChangeAt: '2026-11-26T14:00:00+07:00',
  };
  it('rises second by second until the ticket is inside the deviation, then holds 100%', () => {
    const at = (s: number) => liveTime(card, T0 + s * 1000, 12).target;
    expect(at(0)).toBe(Math.round((12 / 50) * 100)); // 1800 of 3600: 50% off
    expect(at(60)).toBeGreaterThan(at(0)); // a minute later it is already better
    expect(at(1368 + 1)).toBe(100); // elapsed passes 88% of the budget (3168 s): inside the band
    expect(at(1800)).toBe(100); // exactly at budget
    expect(at(1800 + 432)).toBe(100); // 112% of the budget: the edge of the band
    expect(at(1800 + 432 + 600)).toBeLessThan(100); // past the band: falling again
  });
  it('stands still while the clock is frozen', () => {
    const frozen = { ...card, counting: false };
    expect(liveTime(frozen, T0, 12).target).toBe(liveTime(frozen, T0 + 3_000_000, 12).target);
  });
});

