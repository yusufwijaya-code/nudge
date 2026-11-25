import { Component, OnInit, inject } from '@angular/core';
import { MyJiraTokenService } from '../../core/services/my-jira-token.service';
import { JiraTokenBannerComponent } from './jira-token-banner.component';

/** The expiry banner for the signed-in user's own Jira token, on every dashboard page. */
@Component({
  selector: 'app-my-jira-token-banner',
  standalone: true,
  imports: [JiraTokenBannerComponent],
  template: `
    @if (token.hasCredential()) {
      <app-jira-token-banner subject="mine" [expiresAt]="token.expiresAt()" link="/dashboard/settings" linkLabel="Buka Pengaturan" />
    }
  `,
})
export class MyJiraTokenBannerComponent implements OnInit {
  protected readonly token = inject(MyJiraTokenService);

  ngOnInit(): void {
    this.token.refresh();
  }
}
