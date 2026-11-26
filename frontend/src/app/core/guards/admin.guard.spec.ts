import { TestBed } from '@angular/core/testing';
import { Router } from '@angular/router';
import { BehaviorSubject, of, throwError } from 'rxjs';
import { AdminGuard } from './admin.guard';
import { AuthService, CurrentUser } from '../services/auth.service';

describe('AdminGuard', () => {
  let guard: AdminGuard;
  let currentUser$: BehaviorSubject<CurrentUser | null>;
  let router: jasmine.SpyObj<Router>;
  let auth: { currentUser$: BehaviorSubject<CurrentUser | null>; checked: boolean; checkSession: jasmine.Spy };

  beforeEach(() => {
    currentUser$ = new BehaviorSubject<CurrentUser | null>(null);
    router = jasmine.createSpyObj('Router', ['navigate']);
    auth = { currentUser$, checked: true, checkSession: jasmine.createSpy('checkSession') };

    TestBed.configureTestingModule({
      providers: [AdminGuard, { provide: AuthService, useValue: auth }, { provide: Router, useValue: router }],
    });

    guard = TestBed.inject(AdminGuard);
  });

  it('allows navigation when the current user is admin', (done) => {
    currentUser$.next({ email: 'admin@esb.co.id', isAdmin: true });

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(true);
      expect(router.navigate).not.toHaveBeenCalled();
      done();
    });
  });

  it('redirects to Settings and denies navigation when the current user is not admin', (done) => {
    currentUser$.next({ email: 'dev@esb.co.id', isAdmin: false });

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(false);
      expect(router.navigate).toHaveBeenCalledWith(['/dashboard']);
      done();
    });
  });

  // Page refresh on /admin: the session is not loaded yet when the guard runs.
  it('loads the session first on a refresh and lets an admin through', (done) => {
    auth.checked = false;
    auth.checkSession.and.returnValue(of({ email: 'admin@esb.co.id', isAdmin: true }));

    guard.canActivate().subscribe((result) => {
      expect(auth.checkSession).toHaveBeenCalled();
      expect(result).toBe(true);
      expect(router.navigate).not.toHaveBeenCalled();
      done();
    });
  });

  it('on a refresh, still sends a non-admin to the dashboard', (done) => {
    auth.checked = false;
    auth.checkSession.and.returnValue(of({ email: 'dev@esb.co.id', isAdmin: false }));

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(false);
      expect(router.navigate).toHaveBeenCalledWith(['/dashboard']);
      done();
    });
  });

  it('on a refresh without a session, denies without redirecting (AuthGuard sends to login)', (done) => {
    auth.checked = false;
    auth.checkSession.and.returnValue(throwError(() => ({ status: 401 })));

    guard.canActivate().subscribe((result) => {
      expect(result).toBe(false);
      expect(router.navigate).not.toHaveBeenCalled();
      done();
    });
  });
});
