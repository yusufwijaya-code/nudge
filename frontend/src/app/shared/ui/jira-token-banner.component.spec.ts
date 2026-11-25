import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { JiraTokenBannerComponent } from './jira-token-banner.component';

/** "YYYY-MM-DD" of today +/- offset days on the WIB calendar. */
const day = (offset: number) => new Date(Date.now() + offset * 86_400_000).toLocaleDateString('en-CA', { timeZone: 'Asia/Jakarta' });

describe('JiraTokenBannerComponent', () => {
  let fixture: ComponentFixture<JiraTokenBannerComponent>;
  const el = () => fixture.nativeElement as HTMLElement;
  const text = () => el().querySelector('[data-message]')?.textContent?.replace(/\s+/g, ' ').trim() ?? '';
  const show = (inputs: Record<string, unknown>) => {
    Object.entries(inputs).forEach(([k, v]) => fixture.componentRef.setInput(k, v));
    fixture.detectChanges();
  };

  beforeEach(async () => {
    await TestBed.configureTestingModule({ imports: [JiraTokenBannerComponent], providers: [provideRouter([])] }).compileComponents();
    fixture = TestBed.createComponent(JiraTokenBannerComponent);
  });

  it('is silent when the date is unknown or far away', () => {
    show({ expiresAt: '' });
    expect(el().querySelector('.token-banner')).toBeNull();
    show({ expiresAt: day(200) });
    expect(el().querySelector('.token-banner')).toBeNull();
  });

  it('warns inside the window with how many days are left', () => {
    show({ subject: 'mine', expiresAt: day(5) });
    expect(el().querySelector('.token-banner')).not.toBeNull();
    expect(el().querySelector('.token-banner.expired')).toBeNull();
    expect(text()).toContain('akan habis 5 hari lagi');
  });

  it('says "hari ini" on the last day', () => {
    show({ subject: 'mine', expiresAt: day(0) });
    expect(text()).toContain('akan habis hari ini');
  });

  it('turns red and says it is expired once the day has passed', () => {
    show({ subject: 'mine', expiresAt: day(-3) });
    expect(el().querySelector('.token-banner.expired')).not.toBeNull();
    expect(text()).toContain('sudah kedaluwarsa');
    expect(text()).toContain('3 hari lalu');
  });

  it('speaks about the bot account, with its email, for the central token', () => {
    show({ subject: 'bot', email: 'yusuf.wijaya@esb.co.id', expiresAt: day(-1), link: '/admin/jira-account', linkLabel: 'Ganti token' });
    expect(text()).toContain('akun bot (yusuf.wijaya@esb.co.id) sudah kedaluwarsa');
    expect(text()).toContain('reminder');
    const action = el().querySelector('[data-action]') as HTMLAnchorElement;
    expect(action.textContent).toContain('Ganti token');
    expect(action.getAttribute('href')).toBe('/admin/jira-account');
  });
});
