import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { AutoDoneCardComponent } from './auto-done-card.component';
import { AutoDoneStatus } from '../../../core/services/auto-done.service';
import { ToastService } from '../../../core/ui/toast.service';

describe('AutoDoneCardComponent', () => {
  let http: HttpTestingController;
  let fixture: ComponentFixture<AutoDoneCardComponent>;
  let toast: jasmine.SpyObj<ToastService>;
  const q = (s: string) => fixture.nativeElement.querySelector(s) as HTMLElement | null;
  const box = () => q('[data-auto-done-switch]') as HTMLInputElement;
  const url = (r: { url: string }) => r.url.endsWith('/settings/auto-done');

  async function render(status: AutoDoneStatus, hasCredential = status.hasCredential) {
    toast = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);
    await TestBed.configureTestingModule({
      imports: [AutoDoneCardComponent],
      providers: [provideHttpClient(), provideHttpClientTesting(), { provide: ToastService, useValue: toast }],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(AutoDoneCardComponent);
    fixture.componentRef.setInput('hasCredential', hasCredential);
    fixture.detectChanges();
    http.expectOne((r) => r.method === 'GET' && url(r)).flush(status);
    fixture.detectChanges();
  }

  it('is off by default and says why it cannot be turned on without the personal key', async () => {
    await render({ enabled: false, hasCredential: false, active: false });
    expect(box().checked).toBeFalse();
    expect(q('[data-needs-key]')!.textContent).toContain('Jira API Key Pribadi');

    box().checked = true;
    box().dispatchEvent(new Event('change'));
    fixture.detectChanges();
    expect(box().checked).toBeFalse(); // snapped back
    expect(q('[data-error]')!.textContent).toContain('Isi Jira API Key Pribadi dulu');
    http.expectNone((r) => r.method === 'PUT');
  });

  it('turns on with a personal key and tells the user it is active', async () => {
    await render({ enabled: false, hasCredential: true, active: false });
    expect(q('[data-needs-key]')).toBeNull();
    box().checked = true;
    box().dispatchEvent(new Event('change'));
    const req = http.expectOne((r) => r.method === 'PUT' && url(r));
    expect(req.request.body).toEqual({ enabled: true });
    req.flush({ enabled: true, hasCredential: true, active: true });
    fixture.detectChanges();
    expect(toast.success).toHaveBeenCalledWith('Geser otomatis aktif');
    expect(q('.toggle-text strong')!.textContent).toContain('Geser otomatis aktif');
  });

  it('turns off again', async () => {
    await render({ enabled: true, hasCredential: true, active: true });
    expect(box().checked).toBeTrue();
    box().checked = false;
    box().dispatchEvent(new Event('change'));
    http.expectOne((r) => r.method === 'PUT').flush({ enabled: false, hasCredential: true, active: false });
    fixture.detectChanges();
    expect(toast.success).toHaveBeenCalledWith('Geser otomatis dimatikan');
  });

  it('shows "waiting for the key" when it is on but the key was removed', async () => {
    await render({ enabled: true, hasCredential: false, active: false });
    expect(q('.toggle-text strong')!.textContent).toContain('Menunggu Jira API Key Pribadi');
    expect(q('[data-needs-key]')).not.toBeNull();
  });

  it('puts the switch back and shows the message when the server refuses', async () => {
    await render({ enabled: false, hasCredential: true, active: false });
    box().checked = true;
    box().dispatchEvent(new Event('change'));
    http.expectOne((r) => r.method === 'PUT').flush(
      { error: 'credential_required', message: 'Isi Jira API Key Pribadi dulu.' },
      { status: 412, statusText: 'Precondition Failed' },
    );
    fixture.detectChanges();
    expect(box().checked).toBeFalse();
    expect(q('[data-error]')!.textContent).toContain('Isi Jira API Key Pribadi dulu');
  });

  it('reloads the state when the personal key is added or removed', async () => {
    await render({ enabled: false, hasCredential: false, active: false }, false);
    fixture.componentRef.setInput('hasCredential', true);
    fixture.detectChanges();
    TestBed.flushEffects();
    http.expectOne((r) => r.method === 'GET' && url(r)).flush({ enabled: false, hasCredential: true, active: false });
    fixture.detectChanges();
    expect(q('[data-needs-key]')).toBeNull();
  });

  it('offers a retry when the state cannot be loaded', async () => {
    toast = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);
    await TestBed.configureTestingModule({
      imports: [AutoDoneCardComponent],
      providers: [provideHttpClient(), provideHttpClientTesting(), { provide: ToastService, useValue: toast }],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(AutoDoneCardComponent);
    fixture.detectChanges();
    http.expectOne((r) => url(r)).flush({}, { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();
    (q('[data-load-error] button') as HTMLButtonElement).click();
    http.expectOne((r) => url(r)).flush({ enabled: false, hasCredential: true, active: false });
    fixture.detectChanges();
    expect(box()).not.toBeNull();
  });
});
