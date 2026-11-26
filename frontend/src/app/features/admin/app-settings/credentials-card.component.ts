import { Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AppCredential, AppCredentialService, CredentialTest } from '../../../core/services/app-credential.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';
import { expiryDateLabel, expiryDistance, expiryMaxDay, tokenExpiry } from '../../../core/jira-token-expiry';
import { DateRange, DateRangePickerComponent } from '../../../shared/ui/date-range-picker.component';
import { IconComponent } from '../../../shared/ui/icon.component';

type Mode = 'value' | 'expiry';

/**
 * Kredensial & token: what used to be fixed in the server environment. Text
 * credentials (Gemini API key, Google Chat webhook) are changed here and apply at
 * once; the others (OAuth tokens, the service account key, client secrets) are only
 * monitored: their status is shown and the day they expire is kept, which drives the
 * banner an admin sees before one stops working. A secret is never shown, only its end.
 */
@Component({
  selector: 'app-credentials-card',
  standalone: true,
  imports: [FormsModule, IconComponent, DateRangePickerComponent],
  templateUrl: './credentials-card.component.html',
  styleUrl: './credentials-card.component.scss',
})
export class CredentialsCardComponent {
  private api = inject(AppCredentialService);
  private toast = inject(ToastService);
  private confirm = inject(ConfirmService);

  protected items = signal<AppCredential[] | null>(null);
  protected loadError = signal(false);
  /** The credential being edited, and what for. */
  protected editing = signal<{ key: string; mode: Mode } | null>(null);
  protected value = signal('');
  protected showValue = signal(false);
  protected day = signal('');
  protected saving = signal(false);
  protected error = signal<string | null>(null);
  protected testing = signal<string | null>(null);
  protected results = signal<Record<string, CredentialTest>>({});
  protected readonly maxDay = expiryMaxDay();

  protected readonly label = expiryDateLabel;
  protected readonly distance = expiryDistance;

  protected editedItem = computed(() => this.items()?.find((i) => i.key === this.editing()?.key) ?? null);
  protected canSave = computed(() => {
    const e = this.editing();
    if (!e || this.saving()) return false;
    return e.mode === 'expiry' || this.value().trim().length >= 8;
  });

  constructor() {
    this.load();
  }

  protected load(): void {
    this.loadError.set(false);
    this.api.list().subscribe({
      next: (res) => this.items.set(res.items ?? []),
      error: () => this.loadError.set(true),
    });
  }

  protected expiry(it: AppCredential) {
    return tokenExpiry(it.expiresAt);
  }

  protected sourceLabel(it: AppCredential): string {
    switch (it.source) {
      case 'database': return 'Tersimpan di sini';
      case 'env': return 'Masih di env server';
      case 'file': return 'Berkas di server';
      default: return 'Belum diisi';
    }
  }

  protected edit(it: AppCredential, mode: Mode): void {
    this.editing.set({ key: it.key, mode });
    this.value.set('');
    this.showValue.set(false);
    this.day.set(it.expiresAt);
    this.error.set(null);
  }

  protected cancel(): void {
    this.editing.set(null);
    this.error.set(null);
  }

  protected pick(range: DateRange): void {
    this.day.set(range.from);
  }

  protected save(): void {
    const e = this.editing();
    if (!e || !this.canSave()) return;
    this.saving.set(true);
    this.error.set(null);
    const req = e.mode === 'value' ? this.api.setValue(e.key, this.value().trim(), this.day()) : this.api.setExpiry(e.key, this.day());
    req.subscribe({
      next: (it) => {
        this.saving.set(false);
        this.items.update((l) => (l ?? []).map((x) => (x.key === it.key ? it : x)));
        this.editing.set(null);
        this.results.update((r) => ({ ...r, [it.key]: undefined as unknown as CredentialTest }));
        this.toast.success(e.mode === 'value' ? `${it.name} disimpan` : 'Tanggal kedaluwarsa disimpan', e.mode === 'value' ? 'Berlaku langsung, tanpa restart.' : undefined);
      },
      error: (err) => {
        this.saving.set(false);
        this.error.set(err?.error?.error ?? 'Gagal menyimpan, coba lagi.');
      },
    });
  }

  protected async remove(it: AppCredential): Promise<void> {
    const ok = await this.confirm.ask({
      title: `Hapus ${it.name}?`,
      message: 'Nilainya dihapus dari sini dan tidak dipakai lagi, termasuk yang masih ada di env server. Fitur yang memakainya berhenti sampai diisi lagi.',
      confirmText: 'Hapus',
      icon: 'trash',
    });
    if (!ok) return;
    this.api.clear(it.key).subscribe({
      next: (updated) => {
        this.items.update((l) => (l ?? []).map((x) => (x.key === updated.key ? updated : x)));
        this.toast.success(`${it.name} dihapus`);
      },
      error: (err) => this.toast.error('Gagal menghapus', err?.error?.error ?? 'Coba lagi.'),
    });
  }

  protected runTest(it: AppCredential): void {
    if (this.testing()) return;
    this.testing.set(it.key);
    this.api.test(it.key).subscribe({
      next: (r) => {
        this.testing.set(null);
        this.results.update((m) => ({ ...m, [it.key]: r }));
      },
      error: () => {
        this.testing.set(null);
        this.results.update((m) => ({ ...m, [it.key]: { ok: false, message: 'Pengujian gagal dijalankan, coba lagi.' } }));
      },
    });
  }
}
