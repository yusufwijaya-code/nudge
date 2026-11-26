import { DatePipe } from '@angular/common';
import { Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AppSettings, AppSettingsService, MAX_DEVIATION, MIN_DEVIATION } from '../../../core/services/app-settings.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { JiraAccountComponent } from '../jira-account/jira-account.component';

/**
 * Pengaturan Aplikasi: values that used to be hard-coded, then the central Jira
 * account the bot acts as. The deviation drives the sprint summary: the share a
 * ticket's time may differ from its estimate and still count as within target,
 * and the Achievement sprint formula.
 */
@Component({
  selector: 'app-app-settings',
  standalone: true,
  imports: [FormsModule, DatePipe, IconComponent, JiraAccountComponent],
  templateUrl: './app-settings.component.html',
  styleUrl: './app-settings.component.scss',
})
export class AppSettingsComponent {
  private api = inject(AppSettingsService);
  private toast = inject(ToastService);

  protected readonly min = MIN_DEVIATION;
  protected readonly max = MAX_DEVIATION;
  protected settings = signal<AppSettings | null>(null);
  protected loadError = signal(false);
  protected input = signal('');
  protected saving = signal(false);
  protected error = signal<string | null>(null);

  /** The typed value as a number, NaN when it is not one. */
  protected value = computed(() => (this.input().trim() === '' ? NaN : Number(this.input().replace(',', '.'))));
  protected valid = computed(() => Number.isFinite(this.value()) && this.value() >= MIN_DEVIATION && this.value() <= MAX_DEVIATION);
  protected changed = computed(() => this.settings() !== null && this.value() !== this.settings()!.deviationCapPercent);
  protected canSave = computed(() => this.valid() && this.changed() && !this.saving());
  /** What the value means for a 10 hour estimate, written out. */
  protected example = computed(() => {
    const cap = this.valid() ? this.value() : this.settings()?.deviationCapPercent ?? 0;
    const fmt = (n: number) => n.toLocaleString('id-ID', { maximumFractionDigits: 2 });
    return `Tiket estimasi 10 jam masih sesuai kalau waktunya ${fmt(10 - cap / 10)}–${fmt(10 + cap / 10)} jam.`;
  });

  constructor() {
    this.load();
  }

  protected load(): void {
    this.loadError.set(false);
    this.api.get().subscribe({
      next: (s) => this.show(s),
      error: () => this.loadError.set(true),
    });
  }

  private show(s: AppSettings): void {
    this.settings.set(s);
    this.input.set(String(s.deviationCapPercent));
  }

  protected useDefault(): void {
    const s = this.settings();
    if (s) this.input.set(String(s.defaultPercent));
  }

  protected save(): void {
    if (!this.canSave()) return;
    this.saving.set(true);
    this.error.set(null);
    this.api.saveDeviation(this.value()).subscribe({
      next: (s) => {
        this.saving.set(false);
        this.show(s);
        this.toast.success('Deviasi disimpan', `Batas deviasi sekarang ${s.deviationCapPercent}%. Berlaku langsung di ringkasan sprint.`);
      },
      error: (err) => {
        this.saving.set(false);
        this.error.set(err?.error?.error ?? 'Gagal menyimpan, coba lagi.');
      },
    });
  }
}
