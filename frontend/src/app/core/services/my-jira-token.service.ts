import { Injectable, inject, signal } from '@angular/core';
import { BoardService } from './board.service';

/**
 * The signed-in user's own Jira token, as far as the expiry banner needs it:
 * whether one is saved and the day it expires. Always the caller's own (never
 * the admin-edited user), so the shell banner and Pengaturan share one source.
 */
@Injectable({ providedIn: 'root' })
export class MyJiraTokenService {
  private board = inject(BoardService);

  readonly hasCredential = signal(false);
  /** "YYYY-MM-DD", or "" when unknown. */
  readonly expiresAt = signal('');

  /** Reads the caller's settings; a failure leaves the banner as it was. */
  refresh(): void {
    this.board.getSettings().subscribe({
      next: (res) => {
        this.hasCredential.set(!!res.credential?.hasCredential);
        this.expiresAt.set(res.credential?.expiresAt ?? '');
      },
      error: () => undefined,
    });
  }
}
