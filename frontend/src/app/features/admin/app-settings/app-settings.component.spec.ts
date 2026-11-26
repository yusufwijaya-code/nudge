import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { provideRouter } from '@angular/router';
import { AppSettingsComponent } from './app-settings.component';
import { AppSettings } from '../../../core/services/app-settings.service';
import { JiraAccountService } from '../../../core/services/jira-account.service';
import { ToastService } from '../../../core/ui/toast.service';
import { NEVER } from 'rxjs';

const SETTINGS: AppSettings = { deviationCapPercent: 12, defaultPercent: 12 };

describe('AppSettingsComponent', () => {
  let http: HttpTestingController;
  let fixture: ComponentFixture<AppSettingsComponent>;
  let toast: jasmine.SpyObj<ToastService>;
  const q = (s: string) => fixture.nativeElement.querySelector(s) as HTMLElement | null;
  const type = (v: string) => {
    const i = q('[data-deviation]') as HTMLInputElement;
    i.value = v;
    i.dispatchEvent(new Event('input'));
    fixture.detectChanges();
  };
  const save = () => q('[data-save]') as HTMLButtonElement;

  async function render(settings: AppSettings | null = SETTINGS) {
    toast = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);
    await TestBed.configureTestingModule({
      imports: [AppSettingsComponent],
      providers: [
        provideHttpClient(), provideHttpClientTesting(), provideRouter([]),
        { provide: ToastService, useValue: toast },
        // The Jira account card is tested on its own: keep it quiet here.
        { provide: JiraAccountService, useValue: { status: () => NEVER, test: () => NEVER, save: () => NEVER, saveExpiry: () => NEVER, reset: () => NEVER, expiringUsers: () => NEVER } },
      ],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(AppSettingsComponent);
    fixture.detectChanges();
    if (settings) http.expectOne((r) => r.url.endsWith('/admin/app-settings')).flush(settings);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
  }

  it('shows the deviation (12% by default) and the Jira account card below it', async () => {
    await render();
    expect((q('[data-deviation]') as HTMLInputElement).value).toBe('12');
    expect(q('[data-example]')!.textContent).toContain('8,8–11,2 jam');
    expect(fixture.nativeElement.querySelector('app-jira-account')).not.toBeNull();
    expect(save().disabled).toBeTrue(); // nothing changed yet
    expect(q('[data-default]')).toBeNull(); // already the default
  });

  it('saves a new value and tells the admin it applies right away', async () => {
    await render();
    type('15,5');
    expect(save().disabled).toBeFalse();
    expect(q('[data-example]')!.textContent).toContain('8,45–11,55 jam');
    save().click();
    const req = http.expectOne((r) => r.method === 'PUT' && r.url.endsWith('/app-settings/deviation'));
    expect(req.request.body).toEqual({ deviationCapPercent: 15.5 });
    req.flush({ deviationCapPercent: 15.5, defaultPercent: 12, updatedBy: 'boss@esb.co.id', updatedDate: '2026-11-27T02:00:00Z' });
    fixture.detectChanges();
    expect(toast.success).toHaveBeenCalled();
    expect(q('[data-updated]')!.textContent).toContain('boss@esb.co.id');
    expect(save().disabled).toBeTrue();
    expect(q('[data-default]')).not.toBeNull();
  });

  it('refuses values outside 1–100 and anything that is not a number, without calling the server', async () => {
    await render();
    for (const bad of ['0', '101', 'abc', '-3', '']) {
      type(bad);
      expect(save().disabled).withContext(bad).toBeTrue();
    }
    type('abc');
    expect(q('[data-invalid]')!.textContent).toContain('antara 1 dan 100');
    http.expectNone((r) => r.method === 'PUT');
  });

  it('"Pakai default" puts 12 back in the box', async () => {
    await render({ deviationCapPercent: 20, defaultPercent: 12, updatedBy: 'boss@esb.co.id' });
    expect((q('[data-deviation]') as HTMLInputElement).value).toBe('20');
    (q('[data-default]') as HTMLButtonElement).click();
    fixture.detectChanges();
    await fixture.whenStable(); // ngModel writes into the input asynchronously
    expect((q('[data-deviation]') as HTMLInputElement).value).toBe('12');
    expect(save().disabled).toBeFalse();
  });

  it('shows the server message when saving fails', async () => {
    await render();
    type('30');
    save().click();
    http.expectOne((r) => r.method === 'PUT').flush({ error: 'gagal menyimpan pengaturan' }, { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();
    expect(q('[data-error]')!.textContent).toContain('gagal menyimpan');
    expect(save().disabled).toBeFalse();
  });

  it('offers a retry when the settings cannot be loaded', async () => {
    await render(null);
    http.expectOne((r) => r.url.endsWith('/admin/app-settings')).flush({}, { status: 500, statusText: 'Server Error' });
    fixture.detectChanges();
    expect(q('[data-load-error]')).not.toBeNull();
    (q('[data-load-error] button') as HTMLButtonElement).click();
    http.expectOne((r) => r.url.endsWith('/admin/app-settings')).flush(SETTINGS);
    fixture.detectChanges();
    expect(q('[data-deviation]')).not.toBeNull();
  });
});
