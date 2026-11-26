import { signal } from '@angular/core';
import { ComponentFixture, TestBed, fakeAsync, tick } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ReportComponent, presetRange } from './report.component';
import { BoardContextService } from '../../../core/services/board-context.service';
import { ReportProgress, TimeReport, dayLabel, hm, pct, signedHm, thisMonth } from '../../../core/services/report.service';
import { ToastService } from '../../../core/ui/toast.service';

const REPORT: TimeReport = {
  generatedAt: '2026-11-27T09:15:00+07:00', userName: 'Yusuf Wijaya', userEmail: 'y@esb.co.id', boardId: 26, boardName: 'ESB Core Board',
  projects: ['CORE'], from: '2026-11-01', to: '2026-11-27', capPercent: 12, statuses: { CORE: ['Code Process', 'CODE REVIEW'] },
  summary: { ticketCount: 3, scoredCount: 2, withinCount: 1, estimateSeconds: 8 * 3600, rawSeconds: 30 * 3600, deductedSeconds: 20 * 3600, countedSeconds: 10 * 3600, aggregateDeviationPercent: 25, scorePercent: 125, achievementPercent: 48, withinPercent: 50, within: false },
  deductions: [
    { key: 'outside', label: 'Di luar jam kerja', category: 'outside', seconds: 15 * 3600 },
    { key: 'lunch', label: 'Istirahat', category: 'lunch', seconds: 2 * 3600 },
    { key: 'leave', label: 'Cuti', category: 'leave', seconds: 2 * 3600 },
    { key: 'slot:Daily Stand Up', label: 'Daily Stand Up', category: 'slot', seconds: 3600 },
  ],
  tickets: [
    { key: 'CORE-1', summary: 'First', type: 'Task', status: 'Code Completed', url: 'https://jira/browse/CORE-1', created: '2026-11-03T10:00:00+07:00', sprints: ['Core Sprint 23'], storyPoints: 4,
      estimateSeconds: 4 * 3600, rawSeconds: 12 * 3600, countedSeconds: 4 * 3600 + 600, deductions: [{ key: 'lunch', label: 'Istirahat', category: 'lunch', seconds: 3600 }], deductedSeconds: 8 * 3600 - 600,
      statusTimes: [{ status: 'Code Process', rawSeconds: 9 * 3600, countedSeconds: 3 * 3600 }, { status: 'CODE REVIEW', rawSeconds: 3 * 3600, countedSeconds: 3600 + 600 }],
      running: false, deviationPercent: 4.17, scorePercent: 104.17, within: true },
    { key: 'CORE-2', summary: 'Second', type: 'Bug', status: 'Code Process', url: 'https://jira/browse/CORE-2', created: '2026-11-10T10:00:00+07:00', sprints: [], storyPoints: 4,
      estimateSeconds: 4 * 3600, rawSeconds: 18 * 3600, countedSeconds: 6 * 3600, deductions: [], deductedSeconds: 12 * 3600,
      statusTimes: [{ status: 'Code Process', rawSeconds: 18 * 3600, countedSeconds: 6 * 3600 }], running: true, deviationPercent: 50, scorePercent: 150, within: false },
  ],
  excluded: [{ key: 'CORE-3', summary: 'No estimate', status: 'Done', url: 'https://jira/browse/CORE-3', reason: 'Belum ada Story Point, jadi tidak ada estimasi untuk dibandingkan', countedSeconds: 3600 }],
  truncated: false,
};

const progress = (over: Partial<ReportProgress>): ReportProgress => ({
  id: 'job1', state: 'running', step: 0, steps: ['Sinkron data Nudge dengan Jira', 'Mengambil tiket dari Jira', 'Membaca riwayat status tiket', 'Menghitung waktu, pengurang, dan deviasi', 'Menyusun laporan'],
  done: 0, total: 0, percent: 0, params: { from: '2026-11-01', to: '2026-11-27', boardId: 26 }, ...over,
});

describe('ReportComponent', () => {
  let fixture: ComponentFixture<ReportComponent>;
  let http: HttpTestingController;
  let toast: jasmine.SpyObj<ToastService>;
  let selectedId: ReturnType<typeof signal<number | null>>;
  const q = (s: string) => fixture.nativeElement.querySelector(s) as HTMLElement | null;
  const qa = (s: string) => Array.from(fixture.nativeElement.querySelectorAll(s)) as HTMLElement[];
  const jobs = (r: { url: string }) => r.url.endsWith('/api/dashboard/reports/jobs');
  const job = (suffix = '') => (r: { url: string }) => r.url.endsWith('/api/dashboard/reports/jobs/job1' + suffix);

  beforeEach(async () => {
    toast = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);
    selectedId = signal<number | null>(26);
    const boards = { selectedId, selected: () => (selectedId() === 26 ? { boardId: 26, name: 'ESB Core Board' } : null) };
    await TestBed.configureTestingModule({
      imports: [ReportComponent],
      providers: [provideHttpClient(), provideHttpClientTesting(), { provide: ToastService, useValue: toast }, { provide: BoardContextService, useValue: boards }],
    }).compileComponents();
    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(ReportComponent);
    fixture.detectChanges();
  });

  /** Runs a report to the end: start, two progress answers, done. */
  function runToDone(button: 'show' | 'download') {
    (q(`[data-${button}]`) as HTMLButtonElement).click();
    const start = http.expectOne((r) => r.method === 'POST' && jobs(r));
    expect(start.request.body).toEqual({ from: thisMonth().from, to: thisMonth().to, boardId: 26 });
    start.flush(progress({}));
    fixture.detectChanges();
    expect(q('[data-progress]')).not.toBeNull();
    tick(600);
    http.expectOne(job()).flush(progress({ step: 2, done: 3, total: 8, current: 'CORE-7', percent: 40 }));
    fixture.detectChanges();
    expect(q('[data-detail]')!.textContent).toContain('3 dari 8 tiket · CORE-7');
    expect(q('[data-percent]')!.textContent).toContain('40%');
    expect(q('[data-step-state="done"]')).not.toBeNull();
    expect(q('[data-step-state="active"]')!.textContent).toContain('Membaca riwayat status tiket');
    tick(600);
    http.expectOne(job()).flush(progress({ state: 'done', step: 4, percent: 100, report: REPORT }));
    fixture.detectChanges();
  }

  it('starts on this month, for the board picked in the navbar, with a two-month range picker', () => {
    expect(q('[data-range]')).not.toBeNull();
    expect(q('[data-board]')!.textContent).toContain('ESB Core Board');
    expect(q('[data-preset="month"]')!.classList).toContain('on');
    expect(q('[data-card="intro"]')).not.toBeNull();
  });

  it('shows the report after a progress pop up that names the step and the ticket being read', fakeAsync(() => {
    runToDone('show');
    tick(500);
    fixture.detectChanges();
    expect(q('[data-progress]')).toBeNull();
    expect(q('[data-tile="score"] strong')!.textContent).toContain('125%');
    expect(q('[data-tile="within"] strong')!.textContent).toContain('1/2');
    expect(q('[data-tile="achievement"]')!.getAttribute('data-tone')).toBe('bad');
    // From raw to counted: raw, every reason, counted.
    expect(qa('[data-fall]').map((li) => li.getAttribute('data-fall'))).toEqual(['raw', 'outside', 'lunch', 'leave', 'slot:Daily Stand Up', 'counted']);
    expect(q('[data-fall="leave"]')!.textContent).toContain('–2j');
    expect(qa('[data-ticket]').length).toBe(2);
    expect(q('[data-excluded="CORE-3"]')!.textContent).toContain('Story Point');
    http.expectNone(job('/pdf'));
  }));

  it('fits the card without any horizontal scroll: a fixed-width table that is not clipped by a scrolling wrapper', fakeAsync(() => {
    runToDone('show');
    tick(500);
    fixture.detectChanges();
    const wrap = q('.rpt-table-wrap')!;
    expect(getComputedStyle(wrap).overflowX).not.toBe('auto');
    expect(getComputedStyle(wrap).overflowX).not.toBe('scroll');
    const table = q('table.rpt-table') as HTMLTableElement;
    expect(getComputedStyle(table).tableLayout).toBe('fixed');
    expect(table.scrollWidth).toBeLessThanOrEqual(wrap.clientWidth + 1);
    // Every column is there to read.
    expect(qa('thead th').map((th) => th.textContent!.trim())).toEqual(['Tiket', 'Estimasi', 'Kotor', 'Potongan', 'Dihitung', 'Deviasi', 'Skor', 'Hasil']);
  }));

  it('opens a ticket to show its time per status, every deduction and the comparison', fakeAsync(() => {
    runToDone('show');
    tick(500);
    fixture.detectChanges();
    (q('[data-ticket="CORE-1"]') as HTMLElement).click();
    fixture.detectChanges();
    const detail = q('[data-detail="CORE-1"]')!;
    expect(detail.textContent).toContain('Waktu kotor di Code Process');
    expect(detail.textContent).toContain('Waktu kotor di CODE REVIEW');
    expect(detail.textContent).toContain('Istirahat');
    expect(detail.textContent).toContain('+10m');
    expect(detail.querySelector('a')!.getAttribute('href')).toBe('https://jira/browse/CORE-1');
    (q('[data-ticket="CORE-1"]') as HTMLElement).click();
    fixture.detectChanges();
    expect(q('[data-detail="CORE-1"]')).toBeNull();
  }));

  it('downloads the PDF of a fresh report, and still shows the data', fakeAsync(() => {
    spyOn(URL, 'createObjectURL').and.returnValue('blob:x');
    spyOn(URL, 'revokeObjectURL');
    const clicks: string[] = [];
    spyOn(HTMLAnchorElement.prototype, 'click').and.callFake(function (this: HTMLAnchorElement) { clicks.push(this.download); });
    runToDone('download');
    expect(q('[data-headline]')!.textContent).toContain('mengunduh PDF');
    http.expectOne(job('/pdf')).flush(new Blob(['%PDF-1.4']));
    fixture.detectChanges();
    expect(clicks).toEqual(['Laporan-Nudge-Yusuf-Wijaya-2026-11-01_2026-11-27.pdf']);
    expect(toast.success).toHaveBeenCalled();
    expect(q('[data-progress]')).toBeNull();
    expect(q('[data-card="summary"]')).not.toBeNull();
    tick(3000);
  }));

  it('every show reads Jira again (a new job each time)', fakeAsync(() => {
    runToDone('show');
    tick(500);
    fixture.detectChanges();
    (q('[data-show]') as HTMLButtonElement).click();
    http.expectOne((r) => r.method === 'POST' && jobs(r)).flush(progress({ id: 'job1' }));
    tick(600);
    http.expectOne(job()).flush(progress({ state: 'done', percent: 100, report: REPORT }));
    tick(500);
  }));

  it('can cancel while it runs', fakeAsync(() => {
    (q('[data-show]') as HTMLButtonElement).click();
    http.expectOne((r) => r.method === 'POST' && jobs(r)).flush(progress({}));
    fixture.detectChanges();
    (q('[data-cancel]') as HTMLButtonElement).click();
    fixture.detectChanges();
    http.expectOne((r) => r.method === 'DELETE' && job()(r)).flush(null);
    expect(q('[data-progress]')).toBeNull();
    tick(1000);
    http.expectNone(job());
  }));

  it('shows why a report failed', fakeAsync(() => {
    (q('[data-show]') as HTMLButtonElement).click();
    http.expectOne((r) => r.method === 'POST' && jobs(r)).flush(progress({}));
    tick(600);
    http.expectOne(job()).flush(progress({ state: 'failed', error: 'Gagal membaca data dari Jira. Coba lagi sebentar lagi.' }));
    fixture.detectChanges();
    expect(q('[data-progress]')).toBeNull();
    expect(q('[data-error]')!.textContent).toContain('Gagal membaca data dari Jira');
  }));

  it('refuses a bad range up front with the server message', () => {
    (q('[data-show]') as HTMLButtonElement).click();
    http.expectOne((r) => r.method === 'POST' && jobs(r)).flush({ error: 'invalid_range', message: 'Rentang tanggal tidak valid' }, { status: 400, statusText: 'Bad Request' });
    fixture.detectChanges();
    expect(q('[data-error]')!.textContent).toContain('Rentang tanggal tidak valid');
  });

  it('says when the navbar board changed after a report was shown', fakeAsync(() => {
    runToDone('show');
    tick(500);
    fixture.detectChanges();
    expect(q('[data-stale]')).toBeNull();
    selectedId.set(null);
    fixture.detectChanges();
    expect(q('[data-stale]')!.textContent).toContain('ESB Core Board');
    expect(q('[data-board]')!.textContent).toContain('Semua board');
  }));

  it('switches the range with the quick picks', () => {
    (q('[data-preset="last30"]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(q('[data-preset="last30"]')!.classList).toContain('on');
    expect(q('[data-preset="month"]')!.classList).not.toContain('on');
  });
});

describe('report helpers', () => {
  it('formats hours and minutes, signs and percentages', () => {
    expect(hm(0)).toBe('0m');
    expect(hm(45 * 60)).toBe('45m');
    expect(hm(3 * 3600)).toBe('3j');
    expect(hm(3 * 3600 + 5 * 60)).toBe('3j 05m');
    expect(signedHm(600)).toBe('+10m');
    expect(signedHm(-3900)).toBe('-1j 05m');
    expect(pct(12)).toBe('12%');
    expect(pct(12.5)).toBe('12,5%');
    expect(pct(94.126, 2)).toBe('94,13%');
    expect(dayLabel('2026-11-03')).toBe('3 Nov 2026');
    expect(dayLabel('2026-11-03T23:30:00+07:00')).toBe('3 Nov 2026');
  });

  it('works out the quick ranges on the WIB calendar', () => {
    const now = new Date('2026-11-27T03:00:00Z'); // 10:00 WIB
    expect(presetRange('month', now)).toEqual({ from: '2026-11-01', to: '2026-11-27' });
    expect(presetRange('lastMonth', now)).toEqual({ from: '2026-10-01', to: '2026-10-31' });
    expect(presetRange('last30', now)).toEqual({ from: '2026-10-29', to: '2026-11-27' });
    expect(presetRange('quarter', now)).toEqual({ from: '2026-10-01', to: '2026-11-27' });
    expect(presetRange('lastMonth', new Date('2027-01-15T03:00:00Z'))).toEqual({ from: '2026-12-01', to: '2026-12-31' });
  });
});
