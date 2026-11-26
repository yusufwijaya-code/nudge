import { Component, effect, inject, input, signal, untracked } from '@angular/core';
import { AutoDoneService, AutoDoneStatus } from '../../../core/services/auto-done.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';

/**
 * Geser otomatis ke selesai: when a ticket's time is used up it moves to the
 * done status of its own board (Aturan Board), recorded in Jira under the
 * user's name. Needs the personal Jira API key, so the switch stays off until
 * one is saved; `hasCredential` (from Pengaturan) reloads the state when the
 * key is added or removed.
 */
@Component({
  selector: 'app-auto-done-card',
  standalone: true,
  imports: [IconComponent],
  templateUrl: './auto-done-card.component.html',
  styleUrl: './auto-done-card.component.scss',
})
export class AutoDoneCardComponent {
  private api = inject(AutoDoneService);
  private toast = inject(ToastService);

  /** The page's view of the personal key; a change reloads the state. */
  hasCredential = input<boolean>(false);

  protected status = signal<AutoDoneStatus | null>(null);
  protected loadError = signal(false);
  protected saving = signal(false);
  protected error = signal<string | null>(null);

  constructor() {
    effect(() => {
      this.hasCredential();
      untracked(() => this.load());
    });
  }

  protected load(): void {
    this.loadError.set(false);
    this.api.get().subscribe({
      next: (s) => this.status.set(s),
      error: () => this.loadError.set(true),
    });
  }

  protected toggle(box: HTMLInputElement): void {
    const s = this.status();
    if (!s || this.saving()) {
      box.checked = !!s?.enabled;
      return;
    }
    const want = box.checked;
    if (want && !s.hasCredential) {
      box.checked = false;
      this.error.set('Isi Jira API Key Pribadi dulu (kartu di atas), supaya perpindahan status tercatat atas namamu.');
      return;
    }
    this.error.set(null);
    this.saving.set(true);
    this.api.set(want).subscribe({
      next: (res) => {
        this.saving.set(false);
        this.status.set(res);
        this.toast.success(res.enabled ? 'Geser otomatis aktif' : 'Geser otomatis dimatikan');
      },
      error: (err) => {
        this.saving.set(false);
        box.checked = !want;
        this.error.set(err?.error?.message ?? err?.error?.error ?? 'Gagal menyimpan, coba lagi.');
      },
    });
  }
}
