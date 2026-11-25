import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { JiraCredentialComponent } from './jira-credential.component';
import { BoardService } from '../../../core/services/board.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';

describe('JiraCredentialComponent', () => {
  let fixture: ComponentFixture<JiraCredentialComponent>;
  let boardService: jasmine.SpyObj<BoardService>;
  let confirmService: jasmine.SpyObj<ConfirmService>;
  let toastService: jasmine.SpyObj<ToastService>;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', ['saveCredential', 'saveCredentialExpiry', 'deleteCredential']);
    confirmService = jasmine.createSpyObj('ConfirmService', ['ask']);
    toastService = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);

    await TestBed.configureTestingModule({
      imports: [JiraCredentialComponent],
      providers: [
        { provide: BoardService, useValue: boardService },
        { provide: ConfirmService, useValue: confirmService },
        { provide: ToastService, useValue: toastService },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(JiraCredentialComponent);
  });

  it('shows the form when there is no credential yet', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('form')).not.toBeNull();
    expect(fixture.nativeElement.querySelector('.cred-status')).toBeNull();
  });

  it('shows the masked credential and Hapus button when a credential already exists', () => {
    fixture.componentRef.setInput('hasCredential', true);
    fixture.componentRef.setInput('credentialMasked', '••••ab3f');
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('form')).toBeNull();
    expect(fixture.nativeElement.querySelector('.cred-status').textContent).toContain('••••ab3f');
  });

  it('emits credentialChanged after a successful save', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();
    boardService.saveCredential.and.returnValue(of({ status: 'ok' }));
    const emitted = jasmine.createSpy('emitted');
    fixture.componentInstance.credentialChanged.subscribe(emitted);

    fixture.componentInstance['form'].setValue({ jiraEmail: 'a@esb.co.id', apiToken: 'tok123', expiresAt: '2027-10-26' });
    fixture.componentInstance['onSubmit']();

    expect(boardService.saveCredential).toHaveBeenCalledWith('a@esb.co.id', 'tok123', '2027-10-26');
    expect(emitted).toHaveBeenCalled();
    expect(toastService.success).toHaveBeenCalled();
  });

  it('shows the encryption-not-configured message on a 503 response', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();
    boardService.saveCredential.and.returnValue(throwError(() => ({ status: 503 })));

    fixture.componentInstance['form'].setValue({ jiraEmail: 'a@esb.co.id', apiToken: 'tok123', expiresAt: '2027-10-26' });
    fixture.componentInstance['onSubmit']();
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('fitur ini belum dikonfigurasi server, hubungi admin');
  });

  it('shows a generic error message on any other save failure', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();
    boardService.saveCredential.and.returnValue(throwError(() => ({ status: 500 })));

    fixture.componentInstance['form'].setValue({ jiraEmail: 'a@esb.co.id', apiToken: 'tok123', expiresAt: '2027-10-26' });
    fixture.componentInstance['onSubmit']();
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('failed to save credential');
  });

  it('emits credentialChanged after a confirmed delete', async () => {
    fixture.componentRef.setInput('hasCredential', true);
    fixture.componentRef.setInput('credentialMasked', '••••ab3f');
    fixture.detectChanges();
    confirmService.ask.and.resolveTo(true);
    boardService.deleteCredential.and.returnValue(of({ status: 'ok' }));
    const emitted = jasmine.createSpy('emitted');
    fixture.componentInstance.credentialChanged.subscribe(emitted);

    await fixture.componentInstance['onDelete']();

    expect(confirmService.ask).toHaveBeenCalledWith(jasmine.objectContaining({ tone: 'danger' }));
    expect(emitted).toHaveBeenCalled();
    expect(toastService.success).toHaveBeenCalledWith('Credential dihapus');
  });

  it('does not delete anything when the confirmation dialog is cancelled', async () => {
    fixture.componentRef.setInput('hasCredential', true);
    fixture.detectChanges();
    confirmService.ask.and.resolveTo(false);

    await fixture.componentInstance['onDelete']();

    expect(boardService.deleteCredential).not.toHaveBeenCalled();
  });

  it('does not show field errors when an empty field is merely focused and left', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();

    fixture.componentInstance['form'].controls.jiraEmail.markAsTouched();
    fixture.componentInstance['form'].controls.apiToken.markAsTouched();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelectorAll('.field-error').length).toBe(0);
  });

  it('shows the field errors after a submit attempt with an empty form, without calling the API', () => {
    fixture.componentRef.setInput('hasCredential', false);
    fixture.detectChanges();

    fixture.componentInstance['onSubmit']();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelectorAll('.field-error').length).toBe(3);
    expect(boardService.saveCredential).not.toHaveBeenCalled();
  });

  describe('Berlaku sampai (token expiry)', () => {
    const el = () => fixture.nativeElement as HTMLElement;

    it('asks for the expiry date in the setup form and will not save without it', () => {
      fixture.componentRef.setInput('hasCredential', false);
      fixture.detectChanges();
      expect(el().querySelector('app-date-range-picker#expiresAt')).not.toBeNull();
      expect(el().querySelector('input[type="date"]')).toBeNull();

      fixture.componentInstance['form'].setValue({ jiraEmail: 'a@esb.co.id', apiToken: 'tok123', expiresAt: '' });
      fixture.componentInstance['onSubmit']();
      fixture.detectChanges();

      expect(boardService.saveCredential).not.toHaveBeenCalled();
      expect(el().textContent).toContain('Tanggal kedaluwarsa wajib diisi');
    });

    it('shows the server message when the date is refused', () => {
      fixture.componentRef.setInput('hasCredential', false);
      fixture.detectChanges();
      boardService.saveCredential.and.returnValue(throwError(() => ({ status: 400, error: { error: 'tanggal kedaluwarsa tidak valid' } })));
      fixture.componentInstance['form'].setValue({ jiraEmail: 'a@esb.co.id', apiToken: 'tok123', expiresAt: '2027-10-26' });
      fixture.componentInstance['onSubmit']();
      fixture.detectChanges();
      expect(el().textContent).toContain('tanggal kedaluwarsa tidak valid');
    });

    it('shows the saved date with how close it is', () => {
      fixture.componentRef.setInput('hasCredential', true);
      fixture.componentRef.setInput('credentialMasked', '••••ab3f');
      fixture.componentRef.setInput('credentialExpiresAt', '2099-12-31');
      fixture.detectChanges();
      const pill = el().querySelector('[data-expiry-pill]') as HTMLElement;
      expect(pill.textContent).toContain('31 Des 2099');
      expect(pill.className).toContain('ok');
    });

    it('says the date is missing when none was saved', () => {
      fixture.componentRef.setInput('hasCredential', true);
      fixture.componentRef.setInput('credentialExpiresAt', '');
      fixture.detectChanges();
      expect(el().querySelector('[data-expiry-unknown]')?.textContent).toContain('Belum diisi');
    });

    it('changes only the date, without asking for the token again', () => {
      fixture.componentRef.setInput('hasCredential', true);
      fixture.componentRef.setInput('credentialExpiresAt', '');
      fixture.detectChanges();
      boardService.saveCredentialExpiry.and.returnValue(of({ status: 'ok', expiresAt: '2027-01-15' }));
      const emitted = jasmine.createSpy('emitted');
      fixture.componentInstance.credentialChanged.subscribe(emitted);

      fixture.componentInstance['onExpiryPicked']({ from: '2027-01-15', to: '' });
      fixture.detectChanges();
      (el().querySelector('[data-save-expiry]') as HTMLButtonElement).click();

      expect(boardService.saveCredentialExpiry).toHaveBeenCalledWith('2027-01-15');
      expect(boardService.saveCredential).not.toHaveBeenCalled();
      expect(emitted).toHaveBeenCalled();
    });
  });
});
