import { Component, computed, input } from '@angular/core';
import { RouterLink } from '@angular/router';
import { expiryDateLabel, expiryDistance, tokenExpiry } from '../../core/jira-token-expiry';
import { IconComponent } from './icon.component';

/**
 * Warns that a Jira API token is about to expire (or has). Shown only inside
 * the warning window and after the expiry day; silent when the date is
 * unknown, far away, or the token is fine.
 *
 *  - subject "mine": the signed-in user's own token (Pengaturan).
 *  - subject "bot": the central account the bot acts as (Admin › Akun Jira).
 */
@Component({
  selector: 'app-jira-token-banner',
  standalone: true,
  imports: [RouterLink, IconComponent],
  template: `
    @if (view(); as v) {
      <section class="token-banner" [class.expired]="v.expired" [attr.data-subject]="subject()" role="status">
        <app-icon [name]="v.expired ? 'alert-circle' : 'key'" [size]="20" />
        <p data-message>
          @if (subject() === 'mine') {
            @if (v.expired) {
              <strong>Jira token kamu sudah kedaluwarsa</strong> ({{ v.date }}, {{ v.distance }}). Aksi Jira atas namamu, seperti geser status, bisa gagal sampai kamu menyimpan token baru.
            } @else {
              <strong>Jira token kamu akan habis {{ v.distance }}</strong> ({{ v.date }}). Buat token baru dan simpan lagi sebelum itu supaya aksi Jira atas namamu tidak gagal.
            }
          } @else {
            @if (v.expired) {
              <strong>Jira token akun bot{{ email() ? ' (' + email() + ')' : '' }} sudah kedaluwarsa</strong> ({{ v.date }}, {{ v.distance }}). Polling tiket dan reminder tidak bisa membaca Jira sampai token diganti.
            } @else {
              <strong>Jira token akun bot{{ email() ? ' (' + email() + ')' : '' }} akan habis {{ v.distance }}</strong> ({{ v.date }}). Ganti sebelum itu supaya polling tiket dan reminder tidak berhenti.
            }
          }
        </p>
        <a class="btn btn-primary btn-sm" [routerLink]="link()" data-action>{{ linkLabel() }}</a>
      </section>
    }
  `,
})
export class JiraTokenBannerComponent {
  readonly subject = input<'mine' | 'bot'>('mine');
  /** "YYYY-MM-DD", "" when unknown. */
  readonly expiresAt = input<string | null | undefined>('');
  /** The bot account's email, shown in the "bot" message. */
  readonly email = input('');
  readonly link = input('/dashboard/settings');
  readonly linkLabel = input('Perbarui token');

  protected readonly view = computed(() => {
    const expiresAt = this.expiresAt();
    const { level, daysLeft } = tokenExpiry(expiresAt);
    if ((level !== 'soon' && level !== 'expired') || daysLeft === null || !expiresAt) return null;
    return { expired: level === 'expired', date: expiryDateLabel(expiresAt), distance: expiryDistance(daysLeft) };
  });
}
