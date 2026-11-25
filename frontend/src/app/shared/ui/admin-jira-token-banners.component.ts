import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { ExpiringUser, JiraAccountService, JiraAccountStatus } from '../../core/services/jira-account.service';
import { expiryDateLabel, expiryDistance, tokenExpiry } from '../../core/jira-token-expiry';
import { IconComponent } from './icon.component';
import { JiraTokenBannerComponent } from './jira-token-banner.component';

/**
 * The admin's Jira-token warnings, on every admin page: the central account the
 * bot acts as, and the users whose personal token is about to expire or has.
 */
@Component({
  selector: 'app-admin-jira-token-banners',
  standalone: true,
  imports: [RouterLink, IconComponent, JiraTokenBannerComponent],
  template: `
    @if (account(); as a) {
      @if (a.source === 'none') {
        <section class="token-banner expired" data-no-account role="status">
          <app-icon name="alert-circle" [size]="20" />
          <p>
            <strong>Belum ada akun Jira untuk bot.</strong> Polling tiket, reminder, dan geser status tidak berjalan sampai akun diisi.
          </p>
          <a class="btn btn-primary btn-sm" routerLink="/admin/jira-account">Isi akun Jira</a>
        </section>
      } @else {
        <app-jira-token-banner subject="bot" [email]="a.email" [expiresAt]="a.expiresAt" link="/admin/jira-account" linkLabel="Ganti token" />
      }
    }
    @if (users().length; as n) {
      <section class="token-banner" [class.expired]="anyExpired()" data-users role="status">
        <app-icon [name]="anyExpired() ? 'alert-circle' : 'key'" [size]="20" />
        <p>
          <strong>{{ n }} user punya Jira token yang {{ anyExpired() ? 'sudah atau hampir habis' : 'akan habis' }}:</strong>
          {{ summary() }}
        </p>
        <a class="btn btn-primary btn-sm" routerLink="/admin/users">Lihat user</a>
      </section>
    }
  `,
})
export class AdminJiraTokenBannersComponent implements OnInit {
  private api = inject(JiraAccountService);

  protected readonly account = signal<JiraAccountStatus | null>(null);
  protected readonly users = signal<ExpiringUser[]>([]);

  protected readonly anyExpired = computed(() => this.users().some((u) => tokenExpiry(u.expiresAt).level === 'expired'));

  /** "Keva (3 hari lagi), Eren (30 Nov 2026, kemarin)": the first few, then a count. */
  protected readonly summary = computed(() => {
    const MAX = 4;
    const parts = this.users()
      .slice(0, MAX)
      .map((u) => {
        const { daysLeft } = tokenExpiry(u.expiresAt);
        const name = u.name || u.email;
        return daysLeft === null ? name : `${name} (${expiryDateLabel(u.expiresAt)}, ${expiryDistance(daysLeft)})`;
      });
    const rest = this.users().length - MAX;
    return rest > 0 ? `${parts.join(', ')}, dan ${rest} lainnya` : parts.join(', ');
  });

  ngOnInit(): void {
    // A failure just leaves the banner out: it must never get in the admin's way.
    this.api.status().subscribe({ next: (s) => this.account.set(s), error: () => undefined });
    this.api.expiringUsers().subscribe({ next: (res) => this.users.set(res.users ?? []), error: () => undefined });
  }
}
