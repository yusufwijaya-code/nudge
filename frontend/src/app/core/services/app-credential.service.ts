import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

/** One application credential (Pengaturan Aplikasi › Kredensial & token). A secret itself never reaches the browser. */
export interface AppCredential {
  key: string;
  name: string;
  help: string;
  /** The value can be changed here (a text credential); false = only monitored. */
  editable: boolean;
  /** "Uji" can check it right now. */
  testable: boolean;
  configured: boolean;
  /** 'database' = kept here, 'env' = still in the server environment, 'file' = a file on the server. */
  source: 'database' | 'env' | 'file' | 'none';
  /** "••••abcd", or the file name. */
  hint?: string;
  /** A short fact: "ada refresh token", the service account email, why it is not healthy. */
  detail?: string;
  /** "YYYY-MM-DD" or "" when unknown. */
  expiresAt: string;
  updatedBy?: string;
  updatedDate?: string;
}

/** A credential close to (or past) its expiry day: the admin banner. */
export interface ExpiringCredential {
  key: string;
  name: string;
  expiresAt: string;
}

export interface CredentialTest {
  ok: boolean;
  message: string;
}

@Injectable({ providedIn: 'root' })
export class AppCredentialService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/dashboard/admin/app-credentials`;

  list(): Observable<{ items: AppCredential[] }> {
    return this.http.get<{ items: AppCredential[] }>(this.base);
  }

  expiring(): Observable<{ items: ExpiringCredential[] }> {
    return this.http.get<{ items: ExpiringCredential[] }>(`${this.base}/expiring`);
  }

  setValue(key: string, value: string, expiresAt: string): Observable<AppCredential> {
    return this.http.put<AppCredential>(`${this.base}/${key}`, { value, expiresAt });
  }

  setExpiry(key: string, expiresAt: string): Observable<AppCredential> {
    return this.http.put<AppCredential>(`${this.base}/${key}/expiry`, { expiresAt });
  }

  clear(key: string): Observable<AppCredential> {
    return this.http.delete<AppCredential>(`${this.base}/${key}`);
  }

  test(key: string): Observable<CredentialTest> {
    return this.http.post<CredentialTest>(`${this.base}/${key}/test`, {});
  }
}
