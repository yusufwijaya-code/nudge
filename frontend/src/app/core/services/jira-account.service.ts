import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

/** Who Jira says a credential belongs to (GET /myself). */
export interface JiraIdentity {
  displayName: string;
  emailAddress: string;
  accountId: string;
  active: boolean;
}

/** The central Jira account the bot acts as (Admin › Akun Jira). The token itself is never sent. */
export interface JiraAccountStatus {
  /** 'database' = saved by an admin, 'env' = JIRA_EMAIL / JIRA_API_TOKEN on the server. */
  source: 'database' | 'env';
  email: string;
  /** "••••••••aB3f": only the last characters. */
  tokenHint: string;
  updatedBy?: string;
  updatedDate?: string;
  /** False when the server has no CREDENTIAL_ENCRYPTION_KEY: a token cannot be stored. */
  encryptionReady: boolean;
}

export type JiraAccountCheck = { ok: true; account: JiraIdentity } | { ok: false; message: string };

@Injectable({ providedIn: 'root' })
export class JiraAccountService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/dashboard/admin/jira-account`;

  status(): Observable<JiraAccountStatus> {
    return this.http.get<JiraAccountStatus>(this.base);
  }

  /** Asks Jira who the credential in force is; a Jira-side problem comes back as ok:false, not an HTTP error. */
  test(): Observable<JiraAccountCheck> {
    return this.http.post<JiraAccountCheck>(`${this.base}/test`, {});
  }

  /** Verified against Jira, stored encrypted and in force at once. */
  save(email: string, apiToken: string): Observable<{ ok: true; account: JiraIdentity }> {
    return this.http.put<{ ok: true; account: JiraIdentity }>(this.base, { email, apiToken });
  }

  /** Removes the saved account: the server's env credential is used again. */
  reset(): Observable<void> {
    return this.http.delete<void>(this.base);
  }
}
