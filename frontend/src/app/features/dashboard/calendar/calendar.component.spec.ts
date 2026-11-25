import { signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { CalendarComponent, dayTitle, monthTitle, shiftMonth, slotName, slotWhen, todayWIB } from './calendar.component';
import { BoardContextService } from '../../../core/services/board-context.service';
import { CalendarDay, CalendarSlot, WorkCalendarMonth, WorkCalendarService } from '../../../core/services/work-calendar.service';

const LUNCH: CalendarSlot = { kind: 'lunch', name: 'Istirahat', start: '12:00', end: '13:00' };
const DSU: CalendarSlot = { kind: 'dsu', name: 'Daily Stand Up', start: '08:45', end: '09:15' };
const PLANNING: CalendarSlot = { kind: 'ritual', name: 'Sprint Planning / Grooming', start: '11:00', end: '17:30', sprintWeek: 2 };

/** December 2026 starts on a Tuesday. */
function december(withBoard: boolean): WorkCalendarMonth {
  const days: CalendarDay[] = [];
  for (let d = 1; d <= 31; d++) {
    const date = `2026-12-${String(d).padStart(2, '0')}`;
    const weekday = new Date(Date.UTC(2026, 11, d)).getUTCDay();
    let kind: CalendarDay['kind'] = weekday === 0 || weekday === 6 ? 'weekend' : 'workday';
    if (date === '2026-12-25') kind = 'holiday';
    if (date === '2026-12-10') kind = 'leave';
    const slots: CalendarSlot[] = kind === 'workday' ? [LUNCH] : [];
    if (kind === 'workday' && withBoard && weekday === 2) slots.unshift(DSU);
    days.push({ date, weekday, kind, countedSeconds: kind === 'workday' ? (weekday === 2 && withBoard ? 27000 : 28800) : 0, note: kind === 'leave' ? 'Cuti tahunan' : undefined, slots });
  }
  return {
    month: '2026-12', workStart: '08:30', workEnd: '17:30', lunch: LUNCH,
    board: withBoard ? { id: 26, name: 'ESB Core Board', projectKey: 'CORE' } : null,
    sprint: withBoard ? { name: 'Core Sprint 24 2026', start: '2026-11-23', end: '2026-12-04' } : null,
    weekly: withBoard
      ? { dsu: [DSU], rituals: [PLANNING], dsuWeekdays: [2], ritualWeekdays: [1] }
      : { dsu: [], rituals: [], dsuWeekdays: [], ritualWeekdays: [] },
    days,
  };
}

describe('CalendarComponent', () => {
  let fixture: ComponentFixture<CalendarComponent>;
  let api: jasmine.SpyObj<WorkCalendarService>;
  let selectedId: ReturnType<typeof signal<number | null>>;
  const el = () => fixture.nativeElement as HTMLElement;
  const q = (s: string) => el().querySelector(s) as HTMLElement | null;
  const text = (s: string) => q(s)?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

  /** Opens December 2026. */
  const create = (board: number | null = 26) => {
    selectedId.set(board);
    api.month.and.callFake((month: string, b: number | null) => of({ ...december(!!b), month }));
    fixture = TestBed.createComponent(CalendarComponent);
    fixture.componentInstance['month'].set('2026-12');
    fixture.detectChanges();
    TestBed.flushEffects();
    fixture.detectChanges();
  };

  beforeEach(async () => {
    api = jasmine.createSpyObj('WorkCalendarService', ['month']);
    selectedId = signal<number | null>(26);
    await TestBed.configureTestingModule({
      imports: [CalendarComponent],
      providers: [
        { provide: WorkCalendarService, useValue: api },
        { provide: BoardContextService, useValue: { selectedId } },
      ],
    }).compileComponents();
  });

  it('shows the month with the 1st on its weekday (Monday-first) and 31 day cells', () => {
    create();
    expect(text('[data-title]')).toBe('Desember 2026');
    expect(el().querySelectorAll('.cal-cell[data-day]').length).toBe(31);
    // Dec 1, 2026 is a Tuesday: one blank before it.
    expect(el().querySelectorAll('.cal-cell.blank').length).toBe(1);
    const cells = Array.from(el().querySelectorAll('.cal-grid > *')) as HTMLElement[];
    const firstDay = cells.findIndex((c) => c.getAttribute('data-day') === '2026-12-01');
    expect(firstDay - 7).withContext('after the 7 header cells, plus the blank').toBe(1);
  });

  it('tells weekends, tanggal merah and leave apart', () => {
    create();
    const kind = (date: string) => q(`[data-day="${date}"]`)!.getAttribute('data-kind');
    expect(kind('2026-12-05')).toBe('weekend');
    expect(kind('2026-12-25')).toBe('holiday');
    expect(kind('2026-12-10')).toBe('leave');
    expect(kind('2026-12-02')).toBe('workday');
    expect(q('[data-day="2026-12-25"]')!.textContent).toContain('Libur');
    expect(q('[data-day="2026-12-10"]')!.textContent).toContain('Cuti');
  });

  it('shows the DSU as a chip on the days it applies, and not the lunch break', () => {
    create();
    const tuesday = q('[data-day="2026-12-01"]')!;
    expect(tuesday.querySelector('[data-chip]')!.textContent).toContain('DSU 08:45');
    expect(tuesday.textContent).not.toContain('Istirahat');
    expect(q('[data-day="2026-12-02"]')!.querySelector('[data-chip]')).toBeNull();
  });

  it('opens on today when it is in the month, else on the first workday, and follows a click', () => {
    create();
    // Today (the real date) is not in Dec 2026 unless the suite runs then: either way a detail panel is shown.
    expect(q('[data-card="detail"]')).not.toBeNull();
    (q('[data-day="2026-12-01"]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(text('[data-detail-title]')).toBe('Selasa, 1 Desember 2026');
    expect(text('[data-detail-kind]')).toBe('Hari kerja');
    expect(text('[data-counted]')).toContain('7 jam 30 menit');
    const slots = Array.from(el().querySelectorAll('[data-slots] li')).map((li) => li.textContent!.replace(/\s+/g, ' ').trim());
    expect(slots.length).toBe(2);
    expect(slots[0]).toContain('08:45 – 09:15');
    expect(slots[0]).toContain('Daily Stand Up');
    expect(slots[0]).toContain('30 menit');
    expect(slots[1]).toContain('12:00 – 13:00');
    expect(slots[1]).toContain('Istirahat');
  });

  it('explains a leave day, a holiday and a weekend in the detail panel', () => {
    create();
    const open = (date: string) => { (q(`[data-day="${date}"]`) as HTMLButtonElement).click(); fixture.detectChanges(); };
    open('2026-12-10');
    expect(text('[data-detail-kind]')).toBe('Cuti');
    expect(text('[data-leave]')).toContain('Cuti tahunan');
    open('2026-12-25');
    expect(text('[data-detail-kind]')).toContain('Tanggal merah');
    open('2026-12-05');
    expect(text('[data-detail-kind]')).toBe('Akhir pekan');
    expect(q('[data-slots]')).toBeNull();
  });

  it('summarises the month: workdays, holidays, leave and the counted time', () => {
    create();
    const s = text('[data-summary]');
    expect(s).toContain('21 hari kerja'); // 23 weekdays - 1 holiday - 1 leave
    expect(s).toContain('1 tanggal merah');
    expect(s).toContain('1 hari cuti');
    expect(s).toContain('waktu yang terhitung');
  });

  it('lists the fixed schedule of the selected board', () => {
    create();
    expect(text('[data-work]')).toContain('08:30 – 17:30');
    expect(text('[data-lunch]')).toContain('12:00 – 13:00');
    expect(text('[data-dsu]')).toContain('Selasa 08:45 – 09:15');
    expect(text('[data-rituals]')).toContain('Sprint Planning / Grooming');
    expect(text('[data-rituals]')).toContain('Senin 11:00 – 17:30');
    expect(text('[data-rituals]')).toContain('Hanya pada minggu ke-2 sprint');
    expect(text('[data-sprint]')).toContain('Core Sprint 24 2026');
    expect(text('[data-card="weekly"]')).toContain('ESB Core Board');
  });

  it('asks for a board when none is selected, and still shows holidays, leave and the lunch break', () => {
    create(null);
    expect(api.month).toHaveBeenCalledWith('2026-12', null);
    expect(text('[data-card="weekly"]')).toContain('Pilih board');
    expect(q('[data-day="2026-12-25"]')!.getAttribute('data-kind')).toBe('holiday');
    expect(text('[data-lunch]')).toContain('12:00 – 13:00');
    expect(q('[data-sprint]')).toBeNull();
  });

  it('moves between months and back to today', () => {
    create();
    api.month.calls.reset();
    (q('[data-next]') as HTMLButtonElement).click();
    fixture.detectChanges();
    TestBed.flushEffects();
    expect(api.month).toHaveBeenCalledWith('2027-01', 26);
    (q('[data-prev]') as HTMLButtonElement).click();
    (q('[data-prev]') as HTMLButtonElement).click();
    fixture.detectChanges();
    TestBed.flushEffects();
    expect(api.month).toHaveBeenCalledWith('2026-11', 26);
    (q('[data-today]') as HTMLButtonElement).click();
    fixture.detectChanges();
    TestBed.flushEffects();
    expect(api.month).toHaveBeenCalledWith(todayWIB().slice(0, 7), 26);
  });

  it('reloads when the navbar board changes', () => {
    create(26);
    api.month.calls.reset();
    selectedId.set(28);
    fixture.detectChanges();
    TestBed.flushEffects();
    expect(api.month).toHaveBeenCalledWith('2026-12', 28);
  });

  it('shows an error with a retry when the month cannot be loaded', () => {
    selectedId.set(26);
    api.month.and.returnValue(throwError(() => ({ status: 500 })));
    fixture = TestBed.createComponent(CalendarComponent);
    fixture.detectChanges();
    TestBed.flushEffects();
    fixture.detectChanges();
    expect(q('[data-error]')).not.toBeNull();
    api.month.and.returnValue(of(december(true)));
    (q('[data-retry]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(q('[data-error]')).toBeNull();
    expect(q('.cal-cell[data-day]')).not.toBeNull();
  });
});

describe('calendar helpers', () => {
  it('shifts months across a year end', () => {
    expect(shiftMonth('2026-12', 1)).toBe('2027-01');
    expect(shiftMonth('2026-01', -1)).toBe('2025-12');
    expect(shiftMonth('2026-11', 0)).toBe('2026-11');
  });
  it('writes titles in Indonesian', () => {
    expect(monthTitle('2026-12')).toBe('Desember 2026');
    expect(dayTitle('2026-12-02')).toBe('Rabu, 2 Desember 2026');
  });
  it('knows today on the WIB calendar', () => {
    expect(todayWIB(Date.UTC(2026, 10, 25, 17, 30))).toBe('2026-11-26'); // 00:30 WIB the next day
  });
  it('names and times slots', () => {
    expect(slotName({ kind: 'dsu', name: 'x', start: '08:45', end: '09:15' })).toBe('Daily Stand Up');
    expect(slotName({ kind: 'lunch', name: 'x', start: '12:00', end: '13:00' })).toBe('Istirahat');
    expect(slotName(PLANNING)).toBe('Sprint Planning / Grooming');
    expect(slotWhen(PLANNING)).toBe('Hanya pada minggu ke-2 sprint');
    expect(slotWhen(DSU)).toBe('Setiap minggu');
  });
});
