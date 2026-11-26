import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';

/** The Google identity waiting to register (GET /api/signup/me). */
export interface SignupMe {
  email: string;
  name: string;
  /** An account already exists: nothing left to register. */
  registered: boolean;
  adminEmail?: string;
}

/** Body of a failed POST /api/signup. */
export interface SignupError {
  error: string;
  message: string;
  /** The admin to chat with when the registration cannot be completed. */
  adminEmail?: string;
}

/** Registration, the step between "Masuk dengan Google" and having an account. */
@Injectable({ providedIn: 'root' })
export class SignupService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/signup`;

  me() { return this.http.get<SignupMe>(`${this.base}/me`); }
  register() { return this.http.post<{ ok: boolean }>(this.base, {}); }
}

/** Link that opens a Google Chat DM with the admin. */
export function chatAdminUrl(email: string): string {
  return `https://mail.google.com/chat/u/0/#chat/dm/${encodeURIComponent(email)}`;
}
