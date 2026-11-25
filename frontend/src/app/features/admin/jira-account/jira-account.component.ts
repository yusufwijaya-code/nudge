import { Component, OnInit, inject } from '@angular/core';
import { DatePipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { JiraAccountCheck, JiraAccountService, JiraAccountStatus } from '../../../core/services/jira-account.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { expiryDateLabel, expiryDistance, expiryMaxDay, tokenExpiry } from '../../../core/jira-token-expiry';
import { DateRange, DateRangePickerComponent } from '../../../shared/ui/date-range-picker.component';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';

/**
 * Admin › Akun Jira: the central Jira account the bot reads and acts with.
 * A new one is verified against Jira before it is stored (encrypted) and takes
 * effect at once; without one, the server's env credential is used.
 */
@Component({
  selector: 'app-jira-account',
  standalone: true,
  imports: [FormsModule, DatePipe, IconComponent, DateRangePickerComponent],
  templateUrl: './jira-account.component.html',
  styleUrl: './jira-account.component.scss',
})
export class JiraAccountComponent implements OnInit {
  private api = inject(JiraAccountService);
  private confirm = inject(ConfirmService);
  private toast = inject(ToastService);

  protected status: JiraAccountStatus | null = null;
  protected loadError = false;
  /** null = not checked yet / checking. */
  protected check: JiraAccountCheck | null = null;
  protected checking = false;

  protected email = '';
  protected token = '';
  /** Day the new token expires ("YYYY-MM-DD"): required with a new token, so the banner can warn. */
  protected newExpiresAt = '';
  /** The date typed in the inline editor; null until typed, so it starts from the saved one. */
  protected draft: string | null = null;
  protected savingExpiry = false;
  protected showToken = false;
  protected saving = false;
  protected resetting = false;
  protected error: string | null = null;

  ngOnInit(): void {
    this.refresh();
  }

  protected get canSave(): boolean {
    return !!this.status?.encryptionReady && this.email.trim().includes('@') && this.token.trim().length > 0 && !!this.newExpiresAt && !this.saving;
  }

  protected get expiry() {
    return tokenExpiry(this.status?.expiresAt);
  }

  protected get expiryValue(): string {
    return this.draft ?? this.status?.expiresAt ?? '';
  }

  protected readonly expiryDateLabel = expiryDateLabel;
  protected readonly expiryDistance = expiryDistance;

  protected readonly maxDay = expiryMaxDay();

  /** The day picked in the inline editor ("Hapus" gives '' = forget the date). */
  protected onExpiryPicked(range: DateRange): void {
    this.draft = range.from;
  }

  /** Changes only the expiry day of the token in force (the env token's date is entered here too). */
  protected onSaveExpiry(): void {
    if (this.savingExpiry) return;
    this.savingExpiry = true;
    this.api.setExpiry(this.expiryValue).subscribe({
      next: () => {
        this.savingExpiry = false;
        this.draft = null;
        this.toast.success('Tanggal tersimpan', this.expiryValue ? 'Nudge akan mengingatkan sebelum token habis.' : 'Tanggal kedaluwarsa dihapus.');
        this.refresh();
      },
      error: (err) => {
        this.savingExpiry = false;
        this.toast.error('Gagal menyimpan tanggal', err?.status === 400 ? 'Tanggal kedaluwarsa tidak valid.' : 'Coba lagi sebentar.');
      },
    });
  }

  /** No encryption key on the server: nothing can be saved. */
  protected get locked(): boolean {
    return !!this.status && !this.status.encryptionReady;
  }

  protected get sourceLabel(): string {
    return this.status?.source === 'database' ? 'Terpasang' : 'Belum diatur';
  }

  /** Reloads the account in force and asks Jira who it is. */
  protected refresh(): void {
    this.api.status().subscribe({
      next: (status) => {
        this.status = status;
        this.loadError = false;
        // Nothing to ask Jira about until an account is set up.
        if (status.source === 'none') this.check = null;
        else this.runCheck();
      },
      error: () => (this.loadError = true),
    });
  }

  protected runCheck(): void {
    this.checking = true;
    this.check = null;
    this.api.test().subscribe({
      next: (res) => {
        this.check = res;
        this.checking = false;
      },
      error: () => {
        this.check = { ok: false, message: 'Belum bisa menghubungi server.' };
        this.checking = false;
      },
    });
  }

  protected async onSave(): Promise<void> {
    if (!this.canSave) return;
    const confirmed = await this.confirm.ask({
      title: 'Ganti akun Jira?',
      message: `Semua aksi bot ke Jira akan memakai ${this.email.trim()}.`,
      points: [
        'Geser status dan tarik data tiket tercatat atas nama akun ini.',
        'Berlaku langsung, tanpa restart.',
        'User yang punya Jira API key pribadi tetap memakai akunnya sendiri.',
      ],
      note: 'Email dan token diverifikasi ke Jira dulu. Kalau Jira menolak, akun lama tetap dipakai.',
      confirmText: 'Simpan & aktifkan',
      icon: 'key',
    });
    if (!confirmed) return;

    this.saving = true;
    this.error = null;
    this.api.save(this.email.trim(), this.token.trim(), this.newExpiresAt).subscribe({
      next: (res) => {
        this.saving = false;
        this.token = '';
        this.showToken = false;
        this.email = '';
        this.newExpiresAt = '';
        this.draft = null;
        this.toast.success('Akun Jira diganti', `Bot sekarang bertindak sebagai ${res.account.displayName}.`);
        this.refresh();
      },
      error: (err) => {
        this.saving = false;
        this.error = err?.error?.error ?? 'Gagal menyimpan akun Jira, coba lagi.';
      },
    });
  }

  protected async onReset(): Promise<void> {
    const confirmed = await this.confirm.ask({
      title: 'Hapus akun Jira?',
      message: 'Bot berhenti membaca tiket, mengirim reminder, dan menggeser status sampai kamu memasang akun baru.',
      confirmText: 'Hapus akun',
      tone: 'danger',
      icon: 'trash',
    });
    if (!confirmed) return;

    this.resetting = true;
    this.api.reset().subscribe({
      next: () => {
        this.resetting = false;
        this.toast.success('Akun Jira dihapus', 'Pasang akun baru supaya bot bisa membaca Jira lagi.');
        this.refresh();
      },
      error: () => {
        this.resetting = false;
        this.toast.error('Gagal menghapus akun', 'Coba lagi sebentar.');
      },
    });
  }
}
