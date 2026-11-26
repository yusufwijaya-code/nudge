import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { CredentialsCardComponent } from './credentials-card.component';
import { AppCredential } from '../../../core/services/app-credential.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';

const day = (offset: number) => new Date(Date.now() + offset * 86_400_000).toLocaleDateString('en-CA', { timeZone: 'Asia/Jakarta' });
const cred = (over: Partial<AppCredential>): AppCredential => ({
  key: 'gemini_api_key', name: 'Gemini API Key', help: 'Dipakai Buat Card.', editable: true, testable: true, configured: true,
  source: 'database', hint: '••••1234', expiresAt: '', ...over,
});
const LIST: AppCredential[] = [
  cred({}),
  cred({ key: 'groups_oauth_token', name: 'Token OAuth Google Groups', editable: false, testable: true, source: 'file', hint: 'groups.json', detail: 'ada refresh token', expiresAt: day(5) }),
  cred({ key: 'chat_app_service_account', name: 'Kunci service account Chat App', editable: false, testable: false, configured: false, source: 'file', detail: 'berkas tidak ada: chat-app-sa.json', expiresAt: day(-2) }),
  cred({ key: 'google_chat_webhook_url', name: 'Webhook Google Chat', source: 'none', configured: false, hint: undefined, expiresAt: '' }),
];

describe('CredentialsCardComponent', () => {
  let http: HttpTestingController;
  let fixture: ComponentFixture<CredentialsCardComponent>;
  let toast: jasmine.SpyObj<ToastService>;
  let confirm: jasmine.SpyObj<ConfirmService>;
  const q = (s: string) => fixture.nativeElement.querySelector(s) as HTMLElement | null;
  const row = (key: string) => q(`[data-cred="${key}"]`) as HTMLElement;
  const url = (suffix = '') => (r: { url: string }) => r.url.endsWith('/admin/app-credentials' + suffix);

  async function render(list: AppCredential[] | null = LIST) {
    toast = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);
    confirm = jasmine.createSpyObj('ConfirmService', ['ask']);
    await TestBed.configureTestingModule({
      imports: [CredentialsCardComponent],
      providers: [provideHttpClient(), provideHttpClientTesting(), { provide: ToastService, useValue: toast }, { provide: ConfirmService, useValue: confirm }],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(CredentialsCardComponent);
    fixture.detectChanges();
    if (list) http.expectOne(url()).flush({ items: list });
    fixture.detectChanges();
  }

  it('lists every credential with its status, its end and its expiry, never a secret', async () => {
    await render();
    expect(fixture.nativeElement.querySelectorAll('[data-cred]').length).toBe(4);
    expect(row('gemini_api_key').querySelector('[data-hint]')!.textContent).toBe('••••1234');
    expect(row('gemini_api_key').querySelector('[data-status]')!.textContent).toContain('Tersimpan di sini');
    expect(row('groups_oauth_token').querySelector('[data-detail]')!.textContent).toContain('ada refresh token');
    expect(row('groups_oauth_token').querySelector('[data-expiry]')!.getAttribute('data-level')).toBe('soon');
    expect(row('chat_app_service_account').querySelector('[data-expiry]')!.getAttribute('data-level')).toBe('expired');
    expect(row('chat_app_service_account').querySelector('[data-status]')!.textContent).toContain('Bermasalah');
    expect(row('google_chat_webhook_url').querySelector('[data-status]')!.textContent).toContain('Belum diisi');
    expect(row('gemini_api_key').querySelector('[data-expiry]')!.getAttribute('data-level')).toBe('unknown');
  });

  it('only a text credential can be changed or removed; a monitored one only gets its date', async () => {
    await render();
    expect(row('gemini_api_key').querySelector('[data-edit-value]')).not.toBeNull();
    expect(row('gemini_api_key').querySelector('[data-remove]')).not.toBeNull();
    expect(row('groups_oauth_token').querySelector('[data-edit-value]')).toBeNull();
    expect(row('groups_oauth_token').querySelector('[data-remove]')).toBeNull();
    expect(row('groups_oauth_token').querySelector('[data-edit-expiry]')).not.toBeNull();
    expect(row('google_chat_webhook_url').querySelector('[data-edit-value]')!.textContent).toContain('Isi');
    expect(row('gemini_api_key').querySelector('[data-edit-value]')!.textContent).toContain('Ganti');
  });

  it('saves a new value with its expiry day, and the card shows the new end at once', async () => {
    await render();
    (row('gemini_api_key').querySelector('[data-edit-value]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect((q('[data-save]') as HTMLButtonElement).disabled).toBeTrue(); // nothing typed yet
    const input = q('[data-value]') as HTMLInputElement;
    expect(input.type).toBe('password');
    input.value = 'brand-new-key-9999';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    (q('[data-save]') as HTMLButtonElement).click();
    const req = http.expectOne((r) => r.method === 'PUT' && url('/gemini_api_key')(r));
    expect(req.request.body).toEqual({ value: 'brand-new-key-9999', expiresAt: '' });
    req.flush(cred({ hint: '••••9999', updatedBy: 'boss@esb.co.id' }));
    fixture.detectChanges();
    expect(row('gemini_api_key').querySelector('[data-hint]')!.textContent).toBe('••••9999');
    expect(q('[data-editor]')).toBeNull();
    expect(toast.success).toHaveBeenCalled();
  });

  it('shows why the server refused a value and keeps the editor open', async () => {
    await render();
    (row('gemini_api_key').querySelector('[data-edit-value]') as HTMLButtonElement).click();
    fixture.detectChanges();
    const input = q('[data-value]') as HTMLInputElement;
    input.value = 'wrong-key-00000';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    (q('[data-save]') as HTMLButtonElement).click();
    http.expectOne((r) => r.method === 'PUT').flush({ error: 'Gemini API Key tidak diterima: key ditolak Google (HTTP 400)' }, { status: 400, statusText: 'Bad Request' });
    fixture.detectChanges();
    expect(q('[data-error]')!.textContent).toContain('ditolak Google');
    expect(q('[data-editor]')).not.toBeNull();
  });

  it('records only the expiry day of a monitored credential', async () => {
    await render();
    (row('groups_oauth_token').querySelector('[data-edit-expiry]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(q('[data-value]')).toBeNull(); // no value box for a monitored credential
    (q('[data-save]') as HTMLButtonElement).click();
    const req = http.expectOne((r) => r.method === 'PUT' && url('/groups_oauth_token/expiry')(r));
    expect(req.request.body).toEqual({ expiresAt: day(5) });
    req.flush(cred({ key: 'groups_oauth_token', name: 'Token OAuth Google Groups', editable: false, source: 'file', expiresAt: day(5) }));
    fixture.detectChanges();
    expect(q('[data-editor]')).toBeNull();
  });

  it('cancels an edit without calling the server', async () => {
    await render();
    (row('gemini_api_key').querySelector('[data-edit-value]') as HTMLButtonElement).click();
    fixture.detectChanges();
    (q('[data-cancel]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(q('[data-editor]')).toBeNull();
    http.expectNone((r) => r.method === 'PUT');
  });

  it('tests a credential and shows the result next to it', async () => {
    await render();
    (row('groups_oauth_token').querySelector('[data-test]') as HTMLButtonElement).click();
    http.expectOne((r) => r.method === 'POST' && url('/groups_oauth_token/test')(r)).flush({ ok: false, message: 'Token atau akses grup bermasalah: invalid_grant' });
    fixture.detectChanges();
    const result = row('groups_oauth_token').querySelector('[data-result]') as HTMLElement;
    expect(result.textContent).toContain('invalid_grant');
    expect(result.classList).toContain('bad');
    expect(row('chat_app_service_account').querySelector('[data-test]')).toBeNull();
  });

  it('removes a stored value only after confirming', async () => {
    await render();
    confirm.ask.and.resolveTo(false);
    (row('gemini_api_key').querySelector('[data-remove]') as HTMLButtonElement).click();
    await fixture.whenStable();
    http.expectNone((r) => r.method === 'DELETE');

    confirm.ask.and.resolveTo(true);
    (row('gemini_api_key').querySelector('[data-remove]') as HTMLButtonElement).click();
    await fixture.whenStable();
    http.expectOne((r) => r.method === 'DELETE' && url('/gemini_api_key')(r)).flush(cred({ configured: false, source: 'none', hint: undefined }));
    fixture.detectChanges();
    expect(row('gemini_api_key').querySelector('[data-status]')!.textContent).toContain('Belum diisi');
  });

  it('offers a retry when the list cannot be loaded', async () => {
    await render(null);
    http.expectOne(url()).flush({}, { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();
    (q('[data-load-error] button') as HTMLButtonElement).click();
    http.expectOne(url()).flush({ items: LIST });
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelectorAll('[data-cred]').length).toBe(4);
  });
});
