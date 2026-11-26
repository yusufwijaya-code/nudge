import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { provideRouter } from '@angular/router';
import { BehaviorSubject, of } from 'rxjs';
import { HomeComponent } from './home.component';
import { DashboardService, TicketCard } from '../../../core/services/dashboard.service';
import { BoardContextService } from '../../../core/services/board-context.service';
import { LiveClockService } from '../../../core/services/live-clock.service';
import { AuthService } from '../../../core/services/auth.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';

const T0 = Date.parse('2026-11-26T10:00:00+07:00');

function running(over: Partial<TicketCard> = {}): TicketCard {
  return {
    issueKey: 'CORE-1', summary: 'Running', status: 'Code Process', issueURL: 'https://jira/browse/CORE-1', boardId: 26,
    hasBudgetDetail: true, storyPoints: 4, elapsedSeconds: 3600, budgetSeconds: 14400, elapsedLabel: '', remainingLabel: '', deadlineLabel: '',
    serverNow: '2026-11-26T10:00:00+07:00', counting: true, nextChangeAt: '2026-11-26T12:00:00+07:00', ...over,
  };
}

describe('HomeComponent · live clock', () => {
  let fixture: ComponentFixture<HomeComponent>;
  let dashboard: jasmine.SpyObj<DashboardService>;
  let clock: LiveClockService;
  const el = () => fixture.nativeElement as HTMLElement;
  const row = () => el().querySelector('[data-card="active"] .ticket-row') as HTMLElement;
  const meta = (name: string) => row().querySelector(`[data-meta="${name}"]`)?.textContent?.replace(/\s+/g, ' ').trim();

  async function open(card: TicketCard, cap?: number) {
    dashboard = jasmine.createSpyObj('DashboardService', ['getBoard', 'getSprintReport', 'transitionTicket', 'backfillSprint']);
    dashboard.getBoard.and.returnValue(of({ columns: [{ status: 'Code Process', cards: [card] }], related: [], deviationCapPercent: cap } as any));
    dashboard.getSprintReport.and.returnValue(of({ availableSprints: [], capPercent: 12, report: null } as any));
    await TestBed.configureTestingModule({
      imports: [HomeComponent],
      providers: [
        provideHttpClient(), provideHttpClientTesting(), provideRouter([]),
        { provide: DashboardService, useValue: dashboard },
        {
          provide: BoardContextService,
          useValue: { boards: signal([]), loaded: signal(true), selectedId: signal<number | null>(null), selected: () => null, avatarUrl: () => '', select: () => undefined, load: () => undefined },
        },
        { provide: ConfirmService, useValue: jasmine.createSpyObj('ConfirmService', ['ask']) },
        { provide: ToastService, useValue: jasmine.createSpyObj('ToastService', ['success', 'error', 'info']) },
        { provide: AuthService, useValue: { currentUser$: new BehaviorSubject({ email: 'a@esb.co.id', isAdmin: false }) } },
      ],
    }).compileComponents();
    clock = TestBed.inject(LiveClockService);
    clock.now.set(T0);
    fixture = TestBed.createComponent(HomeComponent);
    fixture.detectChanges();
  }
  const tickTo = (seconds: number) => {
    clock.now.set(T0 + seconds * 1000);
    fixture.detectChanges();
    TestBed.flushEffects();
  };

  it('counts the elapsed and remaining time by itself, second by second, with the bar moving', async () => {
    await open(running());
    expect(meta('elapsed')).toContain('1 jam');
    expect(row().classList).toContain('running');
    expect(row().classList).not.toContain('frozen');
    expect(row().querySelector('[data-freeze]')).toBeNull();
    const bar = () => row().querySelector('.budget-fill') as HTMLElement;
    expect(bar().style.width).toBe('25%');

    tickTo(61);
    expect(meta('elapsed')).toContain('1 jam 1 menit 1 detik');
    expect(meta('remaining')).toContain('2 jam 58 menit 59 detik');
    expect(parseFloat(bar().style.width)).toBeGreaterThan(25);

    tickTo(62);
    expect(meta('elapsed')).toContain('1 jam 1 menit 2 detik');
    expect(dashboard.getBoard).toHaveBeenCalledTimes(1);
  });

  it('freezes the card while time does not count, still showing the move buttons', async () => {
    await open(running({ counting: false, nextChangeAt: '2026-11-26T13:00:00+07:00' }));
    expect(row().classList).toContain('frozen');
    expect(row().classList).not.toContain('running');
    expect(row().querySelector('[data-freeze]')?.textContent).toContain('Waktu dihentikan');
    expect(row().querySelectorAll('.ticket-actions button').length).toBeGreaterThan(0);
    expect(Array.from(row().querySelectorAll('.ticket-actions button')).every((b) => !(b as HTMLButtonElement).disabled)).toBeTrue();

    tickTo(600);
    expect(meta('elapsed')).toContain('1 jam');
    expect(meta('elapsed')).not.toContain('menit');
  });

  it('freezes at the end of a counted stretch and reloads the fresh state once', async () => {
    await open(running());
    tickTo(2 * 3600 + 5);
    expect(row().classList).toContain('frozen');
    expect(dashboard.getBoard).toHaveBeenCalledTimes(2);
    // The throttle: a second later it does not reload again.
    tickTo(2 * 3600 + 6);
    expect(dashboard.getBoard).toHaveBeenCalledTimes(2);
  });

  it('shows a Target percentage that updates by itself and is 100% inside the deviation', async () => {
    await open(running({ elapsedSeconds: 1800, nextChangeAt: '2026-11-26T18:00:00+07:00' })); // 1800 s of a 4 h budget
    const chip = () => row().querySelector('[data-meta="target"]') as HTMLElement;
    const first = Number(chip().textContent!.match(/(\d+)%/)![1]);
    expect(first).toBeLessThan(100);
    expect(chip().classList).not.toContain('hit');
    tickTo(120);
    expect(Number(chip().textContent!.match(/(\d+)%/)![1])).toBeGreaterThanOrEqual(first);
    // Inside the band: 4 h * 0.88 = 12672 s, i.e. 10872 s after the answer.
    tickTo(10872 + 5);
    expect(chip().textContent).toContain('Target 100%');
    expect(chip().classList).toContain('hit');
  });

  it('uses the deviation cap the server sends for the Target', async () => {
    // 4 h budget, 5 h used: 25% over. Cap 12 → 48%; cap 25 → inside the band → 100%.
    await open(running({ elapsedSeconds: 18000, counting: false }), 25);
    expect(row().querySelector('[data-meta="target"]')!.textContent).toContain('Target 100%');
  });

  it('falls back to the 12% cap when the server does not send one', async () => {
    await open(running({ elapsedSeconds: 18000, counting: false }));
    expect(row().querySelector('[data-meta="target"]')!.textContent).toContain('Target 48%');
  });

  it('turns over budget while running: the label switches to "Lewat"', async () => {
    await open(running({ elapsedSeconds: 14395 }));
    expect(meta('remaining')).toContain('Sisa');
    tickTo(10);
    expect(meta('remaining')).toContain('Lewat 5 detik');
    expect(row().getAttribute('data-level')).toBe('over');
  });
});
