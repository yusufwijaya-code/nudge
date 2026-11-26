import { Injectable, inject } from '@angular/core';
import { CanActivate, Router } from '@angular/router';
import { Observable, of } from 'rxjs';
import { catchError, map, take } from 'rxjs/operators';
import { AuthService, CurrentUser } from '../services/auth.service';

@Injectable({ providedIn: 'root' })
export class AdminGuard implements CanActivate {
  private authService = inject(AuthService);
  private router = inject(Router);

  canActivate(): Observable<boolean> {
    // Angular runs the guards of a route in parallel, so on a page refresh this
    // one can run before AuthGuard has loaded the session: the user would look
    // like a non-admin and be sent to the dashboard. Load the session first.
    if (!this.authService.checked) {
      return this.authService.checkSession().pipe(
        map((user) => this.decide(user)),
        // No session: AuthGuard sends the user to the login page.
        catchError(() => of(false)),
      );
    }
    return this.authService.currentUser$.pipe(
      take(1),
      map((user) => this.decide(user)),
    );
  }

  private decide(user: CurrentUser | null): boolean {
    if (user?.isAdmin) {
      return true;
    }
    this.router.navigate(['/dashboard']);
    return false;
  }
}
