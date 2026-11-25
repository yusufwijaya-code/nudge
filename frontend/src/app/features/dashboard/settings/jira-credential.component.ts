import { Component, OnInit, inject, input, output } from '@angular/core';
import { FormBuilder, FormGroup, ReactiveFormsModule, Validators, FormControl } from '@angular/forms';
import { NgIf } from '@angular/common';
import { SettingsApi } from '../../../core/services/settings-api';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { expiryDateLabel, expiryDistance, expiryMaxDay, tokenExpiry } from '../../../core/jira-token-expiry';
import { DateRange, DateRangePickerComponent } from '../../../shared/ui/date-range-picker.component';

@Component({
  selector: 'app-jira-credential',
  standalone: true,
  imports: [ReactiveFormsModule, NgIf, IconComponent, DateRangePickerComponent],
  templateUrl: './jira-credential.component.html',
})
export class JiraCredentialComponent implements OnInit {
  hasCredential = input<boolean>(false);
  credentialMasked = input<string | null>(null);
  /** Day the saved token expires ("YYYY-MM-DD"), "" when unknown. */
  credentialExpiresAt = input<string | null>('');
  credentialChanged = output<void>();

  protected form!: FormGroup<{ jiraEmail: FormControl<string>; apiToken: FormControl<string>; expiresAt: FormControl<string> }>;
  protected submitting = false;
  protected deleting = false;
  protected showToken = false;
  protected submitAttempted = false;
  protected error: string | null = null;

  private fb = inject(FormBuilder);
  private boardService = inject(SettingsApi);
  private confirm = inject(ConfirmService);
  private toast = inject(ToastService);

  ngOnInit(): void {
    this.form = this.fb.group({
      jiraEmail: ['', [Validators.required, Validators.email]],
      apiToken: ['', Validators.required],
      // The token is opaque and Jira does not report its expiry: ask for the date Atlassian shows.
      expiresAt: ['', Validators.required],
    }) as FormGroup<{ jiraEmail: FormControl<string>; apiToken: FormControl<string>; expiresAt: FormControl<string> }>;
  }

  /**
   * A field shows its error only once the user has typed in it and left it,
   * or after a submit attempt — merely clicking into an empty field and
   * clicking away again must not paint the form red.
   */
  protected showError(name: 'jiraEmail' | 'apiToken' | 'expiresAt'): boolean {
    const control = this.form.controls[name];
    return control.invalid && ((control.dirty && control.touched) || this.submitAttempted);
  }

  protected onSubmit(): void {
    this.submitAttempted = true;
    if (this.form.invalid || this.submitting) {
      return;
    }
    this.submitting = true;
    this.error = null;

    const value = this.form.getRawValue();
    this.boardService.saveCredential(value.jiraEmail, value.apiToken, value.expiresAt).subscribe({
      next: () => {
        this.submitting = false;
        this.toast.success('Credential tersimpan', 'Aksi Jira sekarang memakai akun kamu sendiri.');
        this.credentialChanged.emit();
      },
      error: (err) => {
        this.submitting = false;
        this.error = err?.status === 503 ? 'fitur ini belum dikonfigurasi server, hubungi admin' : err?.status === 400 && err?.error?.error ? err.error.error : 'failed to save credential';
      },
    });
  }

  /** The date typed in the inline editor; null until the user types, so it starts from the saved one. */
  protected draft: string | null = null;
  protected savingExpiry = false;

  protected get expiryValue(): string {
    return this.draft ?? this.credentialExpiresAt() ?? '';
  }

  protected get expiry() {
    return tokenExpiry(this.credentialExpiresAt());
  }

  protected readonly expiryDateLabel = expiryDateLabel;
  protected readonly expiryDistance = expiryDistance;

  protected readonly maxDay = expiryMaxDay();

  /** The day picked in the setup form. */
  protected onExpiresPicked(range: DateRange): void {
    const control = this.form.controls.expiresAt;
    control.setValue(range.from);
    control.markAsDirty();
    control.markAsTouched();
  }

  /** The day picked in the inline editor ("Hapus" gives '' = forget the date). */
  protected onExpiryPicked(range: DateRange): void {
    this.draft = range.from;
  }

  /** Changes only the date of the saved token. */
  protected onSaveExpiry(): void {
    if (this.savingExpiry) return;
    this.savingExpiry = true;
    this.error = null;
    this.boardService.saveCredentialExpiry(this.expiryValue).subscribe({
      next: () => {
        this.savingExpiry = false;
        this.draft = null;
        this.toast.success('Tanggal tersimpan', this.expiryValue ? 'Nudge akan mengingatkan sebelum token habis.' : 'Tanggal kedaluwarsa dihapus.');
        this.credentialChanged.emit();
      },
      error: (err) => {
        this.savingExpiry = false;
        this.error = err?.status === 400 ? 'tanggal kedaluwarsa tidak valid' : 'gagal menyimpan tanggal';
      },
    });
  }

  protected async onDelete(): Promise<void> {
    const confirmed = await this.confirm.ask({
      title: 'Hapus credential?',
      message: 'Aksi Jira kamu akan kembali memakai akun bot pusat. Kamu bisa menambahkannya lagi kapan saja.',
      confirmText: 'Hapus',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }

    this.deleting = true;
    this.boardService.deleteCredential().subscribe({
      next: () => {
        this.deleting = false;
        this.toast.success('Credential dihapus');
        this.credentialChanged.emit();
      },
      error: () => {
        this.deleting = false;
        this.error = 'failed to delete credential';
      },
    });
  }
}
