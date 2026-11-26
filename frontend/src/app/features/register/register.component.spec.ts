import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { RegisterComponent } from './register.component';

describe('RegisterComponent', () => {
  let http: HttpTestingController;
  let router: Router;

  function render() {
    TestBed.configureTestingModule({ imports: [RegisterComponent], providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])] });
    http = TestBed.inject(HttpTestingController);
    router = TestBed.inject(Router);
    spyOn(router, 'navigateByUrl').and.resolveTo(true);
    const f = TestBed.createComponent(RegisterComponent);
    f.detectChanges();
    return f;
  }
  const me = () => http.expectOne((r) => r.url.endsWith('/api/signup/me'));
  const q = (f: { nativeElement: HTMLElement }, s: string) => f.nativeElement.querySelector(s) as HTMLElement | null;

  it('shows who is registering and the steps, then registers and goes to the dashboard', () => {
    const f = render();
    me().flush({ email: 'new@esb.co.id', name: 'New', registered: false, adminEmail: 'boss@esb.co.id' });
    f.detectChanges();
    expect(q(f, '[data-who]')!.textContent).toContain('new@esb.co.id');
    expect(f.nativeElement.querySelectorAll('.reg-steps li').length).toBe(4);

    (q(f, '[data-register]') as HTMLButtonElement).click();
    f.detectChanges();
    expect((q(f, '[data-register]') as HTMLButtonElement).disabled).toBeTrue();
    http.expectOne((r) => r.method === 'POST' && r.url.endsWith('/api/signup')).flush({ ok: true });
    expect(router.navigateByUrl).toHaveBeenCalledWith('/dashboard');
  });

  it('shows the reason and a chat-admin link when the group step fails, and lets the user retry', () => {
    const f = render();
    me().flush({ email: 'new@esb.co.id', name: 'New', registered: false, adminEmail: 'boss@esb.co.id' });
    f.detectChanges();
    (q(f, '[data-register]') as HTMLButtonElement).click();
    http.expectOne((r) => r.method === 'POST').flush(
      { error: 'group_unavailable', message: 'Akses ke Google Group Nudge bermasalah. Hubungi admin lewat chat.', adminEmail: 'boss@esb.co.id' },
      { status: 503, statusText: 'Service Unavailable' },
    );
    f.detectChanges();

    expect(q(f, '[data-error-message]')!.textContent).toContain('Hubungi admin lewat chat');
    const chat = q(f, '[data-chat-admin]') as HTMLAnchorElement;
    expect(chat.textContent).toContain('boss@esb.co.id');
    expect(chat.getAttribute('href')).toContain('boss%40esb.co.id');
    expect(router.navigateByUrl).not.toHaveBeenCalled();
    const retry = q(f, '[data-register]') as HTMLButtonElement;
    expect(retry.disabled).toBeFalse();
    expect(retry.textContent).toContain('Coba lagi');
  });

  it('asks to sign in again when the sign-up window is gone', () => {
    const f = render();
    me().flush({ message: 'x' }, { status: 401, statusText: 'Unauthorized' });
    f.detectChanges();
    expect(q(f, '[data-expired]')).not.toBeNull();
    expect(q(f, '[data-register]')).toBeNull();
  });

  it('also handles the window ending while registering', () => {
    const f = render();
    me().flush({ email: 'new@esb.co.id', name: 'New', registered: false });
    f.detectChanges();
    (q(f, '[data-register]') as HTMLButtonElement).click();
    http.expectOne((r) => r.method === 'POST').flush({}, { status: 401, statusText: 'Unauthorized' });
    f.detectChanges();
    expect(q(f, '[data-expired]')).not.toBeNull();
  });

  it('leaves for the login when an account already exists', () => {
    render();
    me().flush({ email: 'dev@esb.co.id', name: 'Dev', registered: true });
    expect(router.navigateByUrl).toHaveBeenCalledWith('/');
  });
});
