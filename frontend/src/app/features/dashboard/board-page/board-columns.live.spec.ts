import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { BoardColumnsComponent } from './board-columns.component';
import { TicketCard } from '../../../core/services/dashboard.service';
import { LiveClockService } from '../../../core/services/live-clock.service';

const T0 = Date.parse('2026-11-26T10:00:00+07:00');
const card = (over: Partial<TicketCard>): TicketCard => ({
  issueKey: 'CORE-1', summary: 'S', status: 'Code Process', issueURL: 'u', boardId: 26, hasBudgetDetail: true, storyPoints: 1,
  elapsedSeconds: 1800, budgetSeconds: 3600, elapsedLabel: '', remainingLabel: '', deadlineLabel: '',
  serverNow: '2026-11-26T10:00:00+07:00', counting: true, nextChangeAt: '2026-11-26T12:00:00+07:00', ...over,
});

describe('BoardColumnsComponent · live clock', () => {
  function render(cards: TicketCard[]) {
    TestBed.configureTestingModule({ imports: [BoardColumnsComponent], providers: [provideHttpClient(), provideHttpClientTesting()] });
    const clock = TestBed.inject(LiveClockService);
    clock.now.set(T0);
    const f = TestBed.createComponent(BoardColumnsComponent);
    f.componentRef.setInput('columns', [{ status: 'Code Process', cards }]);
    f.detectChanges();
    return { f, clock };
  }
  const q = (f: { nativeElement: HTMLElement }, key: string) => f.nativeElement.querySelector(`[data-kcard="${key}"]`) as HTMLElement;

  it('moves the time and the bar of a running card every second', () => {
    const { f, clock } = render([card({})]);
    const c = q(f, 'CORE-1');
    expect(c.classList).toContain('running');
    expect(c.textContent).toContain('30 menit / 1 jam');
    expect((c.querySelector('.kb-bar > span') as HTMLElement).style.width).toBe('50%');
    clock.now.set(T0 + 600_000);
    f.detectChanges();
    expect(c.textContent).toContain('40 menit / 1 jam');
    expect(parseFloat((c.querySelector('.kb-bar > span') as HTMLElement).style.width)).toBeCloseTo(66.67, 1);
  });

  it('freezes a card while time does not count (moving it stays possible)', () => {
    const { f, clock } = render([card({ issueKey: 'CORE-2', counting: false, nextChangeAt: '2026-11-26T13:00:00+07:00' })]);
    const c = q(f, 'CORE-2');
    expect(c.classList).toContain('frozen');
    expect(c.classList).not.toContain('running');
    expect(c.querySelector('[data-freeze]')?.textContent).toContain('dihentikan');
    expect(c.classList).not.toContain('busy'); // frozen is only a look: moving it is not blocked
    clock.now.set(T0 + 600_000);
    f.detectChanges();
    expect(c.textContent).toContain('30 menit / 1 jam');
  });

  it('leaves a card without a running clock alone', () => {
    const { f } = render([card({ issueKey: 'CORE-3', hasBudgetDetail: false, timeRecorded: true, elapsedSeconds: 1200 })]);
    const c = q(f, 'CORE-3');
    expect(c.classList).not.toContain('running');
    expect(c.classList).not.toContain('frozen');
    expect(c.textContent).toContain('20 menit / 1 jam');
  });
});
