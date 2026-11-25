import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { BehaviorSubject, Observable, tap } from 'rxjs';
import { environment } from '../../../environments/environment';

export interface CurrentUser {
  email: string;
  isAdmin: boolean;
  /** The owner: may promote admins, and is the account other admins cannot change. */
  isOwner?: boolean;
}

@Injectable({ providedIn: 'root' })
export class AuthService {
  private http = inject(HttpClient);

  readonly currentUser$ = new BehaviorSubject<CurrentUser | null>(null);
  checked = false;

  checkSession(): Observable<CurrentUser> {
    return this.http.get<CurrentUser>(`${environment.apiBasePath}/api/auth/me`).pipe(
      tap((user) => {
        this.currentUser$.next(user);
        this.checked = true;
      }),
    );
  }

  clearSession(): void {
    this.currentUser$.next(null);
    this.checked = true;
  }
}
