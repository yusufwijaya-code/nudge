import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { JiraAccountService } from './jira-account.service';

describe('JiraAccountService', () => {
  let service: JiraAccountService;
  let http: HttpTestingController;
  const base = '/api/dashboard/admin/jira-account';

  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
    service = TestBed.inject(JiraAccountService);
    http = TestBed.inject(HttpTestingController);
  });
  afterEach(() => http.verify());

  const url = (suffix = '') => (req: { url: string }) => req.url.endsWith(base + suffix);

  it('reads the account in force', () => {
    service.status().subscribe((s) => expect(s.email).toBe('yusuf.wijaya@esb.co.id'));
    const req = http.expectOne(url());
    expect(req.request.method).toBe('GET');
    req.flush({ source: 'env', email: 'yusuf.wijaya@esb.co.id', tokenHint: '••••••••abcd', encryptionReady: true });
  });

  it('asks Jira who the credential is with a POST to /test', () => {
    service.test().subscribe((r) => expect(r.ok).toBeTrue());
    const req = http.expectOne(url('/test'));
    expect(req.request.method).toBe('POST');
    req.flush({ ok: true, account: { displayName: 'Yusuf Wijaya', emailAddress: 'yusuf.wijaya@esb.co.id', accountId: 'x', active: true } });
  });

  it('saves with PUT and sends the token only in the body', () => {
    service.save('bot@esb.co.id', 'secret').subscribe();
    const req = http.expectOne(url());
    expect(req.request.method).toBe('PUT');
    expect(req.request.body).toEqual({ email: 'bot@esb.co.id', apiToken: 'secret' });
    expect(req.request.url).not.toContain('secret');
    req.flush({ ok: true, account: { displayName: 'Bot', emailAddress: 'bot@esb.co.id', accountId: 'y', active: true } });
  });

  it('resets with DELETE', () => {
    service.reset().subscribe();
    const req = http.expectOne(url());
    expect(req.request.method).toBe('DELETE');
    req.flush(null);
  });
});
