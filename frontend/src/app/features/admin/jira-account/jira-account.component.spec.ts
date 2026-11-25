import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { of, throwError } from 'rxjs';
import { JiraAccountComponent } from './jira-account.component';
import { JiraAccountService, JiraAccountStatus } from '../../../core/services/jira-account.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';

const ENV: JiraAccountStatus = { source: 'env', email: 'yusuf.wijaya@esb.co.id', tokenHint: '••••••••abcd', encryptionReady: true };
const SAVED: JiraAccountStatus = {
  source: 'database', email: 'bot@esb.co.id', tokenHint: '••••••••WXYZ', encryptionReady: true,
  updatedBy: 'yusuf.wijaya@esb.co.id', updatedDate: '2026-11-25T14:30:00Z',
};
const YUSUF = { displayName: 'Yusuf Wijaya', emailAddress: 'yusuf.wijaya@esb.co.id', accountId: 'acc-1', active: true };

describe('JiraAccountComponent', () => {
  let fixture: ComponentFixture<JiraAccountComponent>;
  let api: jasmine.SpyObj<JiraAccountService>;
  let confirm: jasmine.SpyObj<ConfirmService>;
  let toast: jasmine.SpyObj<ToastService>;
  const el = () => fixture.nativeElement as HTMLElement;
  const q = (selector: string) => el().querySelector(selector);
  const component = () => fixture.componentInstance as any;

  const create = (status: JiraAccountStatus = ENV) => {
    api.status.and.returnValue(of(status));
    fixture = TestBed.createComponent(JiraAccountComponent);
    fixture.detectChanges();
  };
  const fill = (email: string, token: string) => {
    component().email = email;
    component().token = token;
    fixture.detectChanges();
  };

  beforeEach(async () => {
    api = jasmine.createSpyObj('JiraAccountService', ['status', 'test', 'save', 'reset']);
    api.test.and.returnValue(of({ ok: true, account: YUSUF }));
    confirm = jasmine.createSpyObj('ConfirmService', ['ask']);
    confirm.ask.and.resolveTo(true);
    toast = jasmine.createSpyObj('ToastService', ['show', 'success', 'error', 'info']);
    await TestBed.configureTestingModule({
      imports: [JiraAccountComponent],
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: JiraAccountService, useValue: api },
        { provide: ConfirmService, useValue: confirm },
        { provide: ToastService, useValue: toast },
      ],
    }).compileComponents();
  });

  it('shows the account in force, its source, the token hint and who Jira says it is', () => {
    create();
    expect(q('[data-email]')?.textContent).toContain('yusuf.wijaya@esb.co.id');
    expect(q('[data-hint]')?.textContent).toBe('••••••••abcd');
    expect(q('[data-source]')?.textContent).toContain('Dari env server');
    expect(q('[data-account]')?.textContent).toBe('Yusuf Wijaya');
    expect(q('[data-reset]')).toBeNull();
  });

  it('shows who saved the account and offers to go back to env', () => {
    create(SAVED);
    expect(q('[data-source]')?.textContent).toContain('Disimpan admin');
    expect(q('[data-updated]')?.textContent).toContain('yusuf.wijaya@esb.co.id');
    expect(q('[data-reset]')).not.toBeNull();
  });

  it('shows the reason when Jira does not accept the credential in force', () => {
    api.test.and.returnValue(of({ ok: false, message: 'Jira menolak credential ini (token salah atau sudah kedaluwarsa).' }));
    create();
    expect(q('[data-check-error]')?.textContent).toContain('Jira menolak');
    expect(q('[data-account]')).toBeNull();
  });

  it('checks the connection again on demand', () => {
    create();
    api.test.calls.reset();
    (q('[data-test]') as HTMLButtonElement).click();
    expect(api.test).toHaveBeenCalledTimes(1);
  });

  it('shows an error when the account cannot be loaded', () => {
    api.status.and.returnValue(throwError(() => ({ status: 500 })));
    fixture = TestBed.createComponent(JiraAccountComponent);
    fixture.detectChanges();
    expect(q('[data-load-error]')).not.toBeNull();
  });

  it('keeps Simpan disabled until a valid email and a token are filled', () => {
    create();
    const save = () => (q('[data-save]') as HTMLButtonElement).disabled;
    expect(save()).toBeTrue();
    fill('bot@esb.co.id', '');
    expect(save()).toBeTrue();
    fill('not-an-email', 'tok');
    expect(save()).toBeTrue();
    fill('bot@esb.co.id', 'tok');
    expect(save()).toBeFalse();
  });

  it('saves after confirmation, clears the token and reloads the account', async () => {
    create();
    api.save.and.returnValue(of({ ok: true, account: { ...YUSUF, displayName: 'Bot Nudge' } }));
    fill(' bot@esb.co.id ', ' tok ');
    api.status.calls.reset();
    api.test.calls.reset();

    await component().onSave();

    expect(confirm.ask).toHaveBeenCalled();
    expect(api.save).toHaveBeenCalledWith('bot@esb.co.id', 'tok');
    expect(component().token).toBe('');
    expect(toast.success).toHaveBeenCalledWith('Akun Jira diganti', jasmine.stringContaining('Bot Nudge'));
    expect(api.status).toHaveBeenCalledTimes(1);
    expect(api.test).toHaveBeenCalledTimes(1);
  });

  it('does nothing when the confirmation is cancelled', async () => {
    create();
    confirm.ask.and.resolveTo(false);
    fill('bot@esb.co.id', 'tok');
    await component().onSave();
    expect(api.save).not.toHaveBeenCalled();
    expect(component().token).toBe('tok');
  });

  it('shows the server message and keeps the typed values when Jira rejects the credential', async () => {
    create();
    api.save.and.returnValue(throwError(() => ({ status: 422, error: { error: 'Jira menolak email/API token ini. Akun lama tetap dipakai.' } })));
    fill('bot@esb.co.id', 'wrong');
    await component().onSave();
    fixture.detectChanges();

    expect(q('[data-save-error]')?.textContent).toContain('Akun lama tetap dipakai');
    expect(component().token).toBe('wrong');
    expect(toast.success).not.toHaveBeenCalled();
  });

  it('blocks the form and says why when the server has no encryption key', () => {
    create({ ...ENV, encryptionReady: false });
    expect(q('[data-no-key]')?.textContent).toContain('CREDENTIAL_ENCRYPTION_KEY');
    expect((q('#ja-email') as HTMLInputElement).disabled).toBeTrue();
    fill('bot@esb.co.id', 'tok');
    expect((q('[data-save]') as HTMLButtonElement).disabled).toBeTrue();
  });

  it('goes back to the env account after confirmation', async () => {
    create(SAVED);
    api.reset.and.returnValue(of(undefined));
    api.status.and.returnValue(of(ENV));
    await component().onReset();
    expect(api.reset).toHaveBeenCalled();
    expect(toast.success).toHaveBeenCalled();
  });

  it('does not reset when the confirmation is cancelled', async () => {
    create(SAVED);
    confirm.ask.and.resolveTo(false);
    await component().onReset();
    expect(api.reset).not.toHaveBeenCalled();
  });
});
