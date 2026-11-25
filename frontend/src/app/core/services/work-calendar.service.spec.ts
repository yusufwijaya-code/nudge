import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { WorkCalendarService } from './work-calendar.service';

describe('WorkCalendarService', () => {
  let service: WorkCalendarService;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
    service = TestBed.inject(WorkCalendarService);
    http = TestBed.inject(HttpTestingController);
  });
  afterEach(() => http.verify());

  it('asks for one month and the selected board', () => {
    service.month('2026-12', 26).subscribe();
    const req = http.expectOne((r) => r.url.endsWith('/api/dashboard/calendar'));
    expect(req.request.method).toBe('GET');
    expect(req.request.params.get('month')).toBe('2026-12');
    expect(req.request.params.get('board')).toBe('26');
    req.flush({ month: '2026-12', days: [] });
  });

  it('leaves the board out when none is selected', () => {
    service.month('2026-12', null).subscribe();
    const req = http.expectOne((r) => r.url.endsWith('/api/dashboard/calendar'));
    expect(req.request.params.has('board')).toBeFalse();
    req.flush({ month: '2026-12', days: [] });
  });
});
