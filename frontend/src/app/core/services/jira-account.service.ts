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
  /** 'database' = an account is set up here; 'none' = nothing is set up and the bot cannot read Jira. */
  source: 'database' | 'none';
  email: string;
  /** "••••••••aB3f": only the last characters. */
  tokenHint: string;
  updatedBy?: string;
  updatedDate?: string;
  /** An account is stored but cannot be decrypted (the encryption key changed): it is not in force. */
  unreadable: boolean;
  /** False when the server has no CREDENTIAL_ENCRYPTION_KEY: a token cannot be stored. */
  encryptionReady: boolean;
  /** Day the token in force expires ("YYYY-MM-DD"), entered by an admin because Jira does not report it; "" = unknown. */
  expiresAt: string;
}

/** A user whose personal Jira token expires soon (or already has). */
export interface ExpiringUser {
  name: string;
  email: string;
  expiresAt: string;
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

  /** Verified against Jira, stored encrypted and in force at once; expiresAt is the date Atlassian shows for the token. */
  save(email: string, apiToken: string, expiresAt = ''): Observable<{ ok: true; account: JiraIdentity }> {
    return this.http.put<{ ok: true; account: JiraIdentity }>(this.base, { email, apiToken, expiresAt });
  }

  /** Changes only the expiry day of the token in force ("" forgets it). */
  setExpiry(expiresAt: string): Observable<{ ok: true }> {
    return this.http.put<{ ok: true }>(`${this.base}/expiry`, { expiresAt });
  }

  /** Users whose personal token expires within the warning window or already has. */
  expiringUsers(): Observable<{ users: ExpiringUser[]; warnDays: number }> {
    return this.http.get<{ users: ExpiringUser[]; warnDays: number }>(`${this.base}/expiring-users`);
  }

  /** Removes the account: the bot cannot read Jira until a new one is saved. */
  reset(): Observable<void> {
    return this.http.delete<void>(this.base);
  }
}
