import { JIRA_TOKEN_WARN_DAYS, expiryDateLabel, expiryDistance, expiryMaxDay, tokenExpiry } from './jira-token-expiry';

describe('tokenExpiry', () => {
  // 25 Nov 2026, 10:00 WIB
  const now = new Date('2026-11-25T03:00:00Z');

  it('is unknown without a date, or with something that is not a real day', () => {
    for (const bad of [null, undefined, '', 'besok', '2026-13-01', '2027-02-31', '26-11-2026']) {
      expect(tokenExpiry(bad, now)).toEqual({ level: 'unknown', daysLeft: null });
    }
  });

  it('is ok while the expiry is further away than the warning window', () => {
    expect(tokenExpiry('2027-10-26', now).level).toBe('ok');
    expect(tokenExpiry('2026-12-10', now)).toEqual({ level: 'ok', daysLeft: 15 });
  });

  it('warns from the last day of the window until the expiry day itself', () => {
    expect(tokenExpiry('2026-12-09', now)).toEqual({ level: 'soon', daysLeft: JIRA_TOKEN_WARN_DAYS });
    expect(tokenExpiry('2026-11-30', now)).toEqual({ level: 'soon', daysLeft: 5 });
    expect(tokenExpiry('2026-11-25', now)).toEqual({ level: 'soon', daysLeft: 0 });
  });

  it('is expired once the expiry day has passed', () => {
    expect(tokenExpiry('2026-11-24', now)).toEqual({ level: 'expired', daysLeft: -1 });
    expect(tokenExpiry('2026-11-23', now)).toEqual({ level: 'expired', daysLeft: -2 });
  });

  // 00:30 WIB on the 26th is still the 25th in UTC: the WIB calendar decides.
  it('counts days on the WIB calendar, not the browser or UTC one', () => {
    const justAfterMidnightWib = new Date('2026-11-25T17:30:00Z'); // 26 Nov 00:30 WIB
    expect(tokenExpiry('2026-11-26', justAfterMidnightWib)).toEqual({ level: 'soon', daysLeft: 0 });
    expect(tokenExpiry('2026-11-25', justAfterMidnightWib)).toEqual({ level: 'expired', daysLeft: -1 });
  });
});

describe('expiry labels', () => {
  it('writes the day in Indonesian', () => {
    expect(expiryDateLabel('2026-11-30')).toBe('30 Nov 2026');
    expect(expiryDateLabel('2027-05-03')).toBe('3 Mei 2027');
    expect(expiryDateLabel('garbage')).toBe('garbage');
  });

  it('writes the distance in words', () => {
    expect(expiryDistance(0)).toBe('hari ini');
    expect(expiryDistance(1)).toBe('besok');
    expect(expiryDistance(5)).toBe('5 hari lagi');
    expect(expiryDistance(-1)).toBe('kemarin');
    expect(expiryDistance(-3)).toBe('3 hari lalu');
  });
});

describe('expiryMaxDay', () => {
  it('is the same day ten years ahead on the WIB calendar (the latest the server accepts)', () => {
    expect(expiryMaxDay(new Date('2026-11-25T03:00:00Z'))).toBe('2036-11-25');
    // 00:30 WIB on the 26th is still the 25th in UTC
    expect(expiryMaxDay(new Date('2026-11-25T17:30:00Z'))).toBe('2036-11-26');
  });
});
