import { LiveCard, liveTime } from './live-time';

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
