import { DestroyRef, Injectable, NgZone, inject, signal } from '@angular/core';

/**
 * One shared clock that ticks every second, on the server's time: pages read
 * `now()` so the elapsed / remaining time of a running ticket counts by itself
 * instead of waiting for the next poll.
 */
@Injectable({ providedIn: 'root' })
export class LiveClockService {
  private skew = 0;
  /** Milliseconds since the epoch, corrected to the server's clock. */
  readonly now = signal(Date.now());

  constructor() {
    // Outside the zone so the endless timer never keeps the app "unstable";
    // each tick re-enters it so the views update.
    const zone = inject(NgZone);
    const tick = () => zone.run(() => this.now.set(Date.now() + this.skew));
    const timer = zone.runOutsideAngular(() =>
      setInterval(() => {
        if (!document.hidden) tick();
      }, 1000),
    );
    const onVisible = () => {
      if (!document.hidden) tick();
    };
    document.addEventListener('visibilitychange', onVisible);
    inject(DestroyRef).onDestroy(() => {
      clearInterval(timer);
      document.removeEventListener('visibilitychange', onVisible);
    });
  }

  /** Aligns the clock with the server's time stamp of a fresh answer (ignored when unusable). */
  calibrate(serverNowIso?: string): void {
    const t = serverNowIso ? Date.parse(serverNowIso) : NaN;
    if (!Number.isFinite(t)) return;
    this.skew = t - Date.now();
    this.now.set(Date.now() + this.skew);
  }
}
