import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of, throwError } from 'rxjs';
import { AdminJiraTokenBannersComponent } from './admin-jira-token-banners.component';
import { JiraAccountService, JiraAccountStatus } from '../../core/services/jira-account.service';

const day = (offset: number) => new Date(Date.now() + offset * 86_400_000).toLocaleDateString('en-CA', { timeZone: 'Asia/Jakarta' });
const status = (expiresAt: string): JiraAccountStatus => ({ source: 'database', email: 'yusuf.wijaya@esb.co.id', tokenHint: '••••••••abcd', encryptionReady: true, unreadable: false, expiresAt });
const none: JiraAccountStatus = { source: 'none', email: '', tokenHint: '', encryptionReady: true, unreadable: false, expiresAt: '' };

describe('AdminJiraTokenBannersComponent', () => {
  let fixture: ComponentFixture<AdminJiraTokenBannersComponent>;
  let api: jasmine.SpyObj<JiraAccountService>;
  const el = () => fixture.nativeElement as HTMLElement;

  const create = (account: JiraAccountStatus, users: { name: string; email: string; expiresAt: string }[]) => {
    api.status.and.returnValue(of(account));
    api.expiringUsers.and.returnValue(of({ users, warnDays: 14 }));
    fixture = TestBed.createComponent(AdminJiraTokenBannersComponent);
    fixture.detectChanges();
  };

  beforeEach(async () => {
    api = jasmine.createSpyObj('JiraAccountService', ['status', 'expiringUsers']);
    await TestBed.configureTestingModule({
      imports: [AdminJiraTokenBannersComponent],
      providers: [provideRouter([]), { provide: JiraAccountService, useValue: api }],
    }).compileComponents();
  });

  it('shows nothing when the bot token is fine and no user token is close', () => {
    create(status(day(300)), []);
    expect(el().querySelector('.token-banner')).toBeNull();
  });

  it('tells the admin loudly when no Jira account is set up at all', () => {
    create(none, []);
    const banner = el().querySelector('[data-no-account]') as HTMLElement;
    expect(banner.textContent).toContain('Belum ada akun Jira');
    expect(banner.classList).toContain('expired');
    expect((banner.querySelector('a') as HTMLAnchorElement).getAttribute('href')).toBe('/admin/jira-account');
    expect(el().querySelector('.token-banner[data-subject="bot"]')).toBeNull();
  });

  it('warns about the central account when its token is about to expire', () => {
    create(status(day(6)), []);
    const banner = el().querySelector('.token-banner[data-subject="bot"]');
    expect(banner?.textContent).toContain('yusuf.wijaya@esb.co.id');
    expect(banner?.textContent).toContain('6 hari lagi');
    expect(el().querySelector('[data-users]')).toBeNull();
  });

  it('lists the users whose personal token expires soon, with their date', () => {
    create(status(''), [
      { name: 'Keva Damar Galih', email: 'keva.galih@esb.co.id', expiresAt: day(3) },
      { name: '', email: 'eren.dhoheiri@esb.co.id', expiresAt: day(-2) },
    ]);
    const users = el().querySelector('[data-users]') as HTMLElement;
    expect(users.textContent).toContain('2 user');
    expect(users.textContent).toContain('Keva Damar Galih');
    expect(users.textContent).toContain('3 hari lagi');
    expect(users.textContent).toContain('eren.dhoheiri@esb.co.id');
    expect(users.textContent).toContain('2 hari lalu');
    expect(users.classList).toContain('expired');
    expect((users.querySelector('a') as HTMLAnchorElement).getAttribute('href')).toBe('/admin/users');
  });

  it('shortens a long list to the first few and a count', () => {
    const many = Array.from({ length: 7 }, (_, i) => ({ name: `User ${i + 1}`, email: `u${i + 1}@esb.co.id`, expiresAt: day(2) }));
    create(status(''), many);
    const text = el().querySelector('[data-users]')?.textContent ?? '';
    expect(text).toContain('7 user');
    expect(text).toContain('User 4');
    expect(text).not.toContain('User 5');
    expect(text).toContain('dan 3 lainnya');
  });

  it('stays out of the way when the data cannot be loaded', () => {
    api.status.and.returnValue(throwError(() => ({ status: 500 })));
    api.expiringUsers.and.returnValue(throwError(() => ({ status: 500 })));
    fixture = TestBed.createComponent(AdminJiraTokenBannersComponent);
    fixture.detectChanges();
    expect(el().querySelector('.token-banner')).toBeNull();
  });
});
