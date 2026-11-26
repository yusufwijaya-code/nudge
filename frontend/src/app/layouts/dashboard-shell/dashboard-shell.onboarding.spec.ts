import { signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { BehaviorSubject, of } from 'rxjs';
import { DashboardShellComponent } from './dashboard-shell.component';
import { AuthService } from '../../core/services/auth.service';
import { BoardService } from '../../core/services/board.service';
import { BoardContextService, BoardSummary } from '../../core/services/board-context.service';
import { ChatAppService } from '../../core/services/chat-app.service';
import { OnboardingBoard, OnboardingService } from '../../core/services/onboarding.service';

const OMS: BoardSummary = {
  boardId: 28, name: 'ESB OMS Board', projectKey: 'OMS',
  activeSprint: { id: 1, name: 'OMS Sprint 23', startDate: '2026-11-09T01:00:00.000Z', endDate: '2026-11-22T10:00:00.000Z' },
  ticketCount: 3, hasCustomSettings: false, muted: { active: false, until: null },
};
const NEW_BOARD: OnboardingBoard = { boardId: 40, name: 'New Board', projectKey: 'NEW', sprintName: 'New Sprint 1', ticketCount: 2, registered: false };

describe('DashboardShellComponent · onboarding', () => {
  let fixture: ComponentFixture<DashboardShellComponent>;
  let boards: ReturnType<typeof signal<BoardSummary[]>>;
  let onboarding: OnboardingService;
  let router: Router;
  const el = () => fixture.nativeElement as HTMLElement;

  beforeEach(async () => {
    boards = signal<BoardSummary[]>([OMS]);
    const boardContext = {
      boards, loaded: signal(true), selectedId: signal<number | null>(null),
      selected: () => null, avatarUrl: (id: number) => '/avatar/' + id, load: () => undefined, select: () => undefined,
    };
    await TestBed.configureTestingModule({
      imports: [DashboardShellComponent],
      providers: [
        provideHttpClient(), provideHttpClientTesting(), provideRouter([]),
        { provide: AuthService, useValue: { currentUser$: new BehaviorSubject<any>({ email: 'dev@esb.co.id', isAdmin: false }) } },
        { provide: BoardService, useValue: { getSettings: () => of({ mute: { global: { active: false, until: null }, board: null } }) } },
        { provide: BoardContextService, useValue: boardContext },
        { provide: ChatAppService, useValue: { status: signal(null), refresh: () => undefined, activateUrl: () => '' } },
      ],
    }).compileComponents();
    onboarding = TestBed.inject(OnboardingService);
    router = TestBed.inject(Router);
    spyOn(router, 'navigate').and.resolveTo(true);
    fixture = TestBed.createComponent(DashboardShellComponent);
    fixture.detectChanges();
  });

  it('lists a board with no Aturan Board under "Belum diatur" and opens its setup on click', () => {
    onboarding.unregistered.set([NEW_BOARD]);
    onboarding.loaded.set(true);
    fixture.detectChanges();
    (el().querySelector('.board-picker-trigger') as HTMLButtonElement).click();
    fixture.detectChanges();

    expect(el().querySelector('[data-unregistered-title]')?.textContent).toContain('Belum diatur');
    const option = el().querySelector('[data-setup-board="40"]') as HTMLButtonElement;
    expect(option.textContent).toContain('New Board');
    option.click();
    expect(router.navigate).toHaveBeenCalledWith(['/onboarding'], { queryParams: { board: 40 } });
  });

  it('keeps the dashboard when some boards are registered (the new one only waits in the picker)', () => {
    onboarding.unregistered.set([NEW_BOARD]);
    onboarding.loaded.set(true);
    fixture.detectChanges();
    TestBed.flushEffects();
    expect(router.navigate).not.toHaveBeenCalledWith(['/onboarding'], jasmine.anything());
  });

  it('sends a user whose boards are all new straight to the board setup', () => {
    boards.set([]);
    onboarding.unregistered.set([NEW_BOARD]);
    onboarding.loaded.set(true);
    fixture.detectChanges();
    TestBed.flushEffects();
    expect(router.navigate).toHaveBeenCalledWith(['/onboarding'], { queryParams: { board: 40 }, replaceUrl: true });
  });

  it('does not redirect a user with no board at all, and says so in the picker', () => {
    boards.set([]);
    onboarding.unregistered.set([]);
    onboarding.loaded.set(true);
    fixture.detectChanges();
    TestBed.flushEffects();
    expect(router.navigate).not.toHaveBeenCalledWith(['/onboarding'], jasmine.anything());
    (el().querySelector('.board-picker-trigger') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(el().querySelector('.board-menu-empty')?.textContent).toContain('Belum ada tiket');
  });
});
