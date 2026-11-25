import { Component, computed, effect, inject, signal, untracked } from '@angular/core';
import { BoardContextService } from '../../../core/services/board-context.service';
import { CalendarDay, CalendarSlot, WorkCalendarMonth, WorkCalendarService } from '../../../core/services/work-calendar.service';
import { formatDuration } from '../../../core/duration-format';
import { IconComponent } from '../../../shared/ui/icon.component';

const MONTHS = ['Januari', 'Februari', 'Maret', 'April', 'Mei', 'Juni', 'Juli', 'Agustus', 'September', 'Oktober', 'November', 'Desember'];
const WEEKDAYS = ['Minggu', 'Senin', 'Selasa', 'Rabu', 'Kamis', 'Jumat', 'Sabtu'];
/** Monday-first header, as on Indonesian calendars. */
const HEAD = ['Sen', 'Sel', 'Rab', 'Kam', 'Jum', 'Sab', 'Min'];

/** Today in WIB as 2006-01-02. */
export function todayWIB(now: number = Date.now()): string {
  return new Date(now + 7 * 3600_000).toISOString().slice(0, 10);
}

/** "2026-12" shifted by n months. */
export function shiftMonth(month: string, n: number): string {
  const [y, m] = month.split('-').map(Number);
  const d = new Date(Date.UTC(y, m - 1 + n, 1));
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, '0')}`;
}

export function monthTitle(month: string): string {
  const [y, m] = month.split('-').map(Number);
  return `${MONTHS[m - 1]} ${y}`;
}

/** "2026-12-02" → "Rabu, 2 Desember 2026". */
export function dayTitle(date: string): string {
  const [y, m, d] = date.split('-').map(Number);
  return `${WEEKDAYS[new Date(Date.UTC(y, m - 1, d)).getUTCDay()]}, ${d} ${MONTHS[m - 1]} ${y}`;
}

/** What a slot is called in the UI. */
export function slotName(slot: CalendarSlot): string {
  return slot.kind === 'dsu' ? 'Daily Stand Up' : slot.kind === 'lunch' ? 'Istirahat' : slot.name;
}

/** When a slot applies: "Setiap minggu" or "Hanya pada minggu ke-2 sprint". */
export function slotWhen(slot: CalendarSlot): string {
  return slot.sprintWeek ? `Hanya pada minggu ke-${slot.sprintWeek} sprint` : 'Setiap minggu';
}

interface Cell {
  day: CalendarDay | null;
  /** 1-31, 0 for the blanks before the 1st. */
  number: number;
}

/**
 * Kalender: what counts, and what does not, on each day, so a user can see why a
 * ticket's time is what it is: weekends, national holidays, the user's own leave,
 * the lunch break and the selected board's Daily Stand Up / special activities.
 * The server works every day out with the same rules as the time calculation.
 */
@Component({
  selector: 'app-calendar',
  standalone: true,
  imports: [IconComponent],
  templateUrl: './calendar.component.html',
  styleUrl: './calendar.component.scss',
})
export class CalendarComponent {
  private api = inject(WorkCalendarService);
  private boards = inject(BoardContextService);

  protected readonly head = HEAD;
  protected readonly today = todayWIB();
  protected readonly slotName = slotName;
  protected readonly slotWhen = slotWhen;
  protected readonly dayTitle = dayTitle;
  protected readonly monthTitle = monthTitle;
  protected readonly duration = formatDuration;
  protected readonly weekdayNames = WEEKDAYS;

  protected month = signal(this.today.slice(0, 7));
  protected data = signal<WorkCalendarMonth | null>(null);
  protected loading = signal(true);
  protected error = signal(false);
  protected selected = signal<string | null>(null);

  protected hasBoard = computed(() => !!this.boards.selectedId());

  protected cells = computed<Cell[]>(() => {
    const data = this.data();
    if (!data || !data.days.length) return [];
    const lead = (data.days[0].weekday + 6) % 7;
    const cells: Cell[] = Array.from({ length: lead }, () => ({ day: null, number: 0 }));
    for (const day of data.days) cells.push({ day, number: Number(day.date.slice(8)) });
    return cells;
  });

  /** The day shown in the detail panel: the picked one, else today (when in this month), else the first workday. */
  protected detail = computed<CalendarDay | null>(() => {
    const days = this.data()?.days ?? [];
    const pick = this.selected();
    return days.find((d) => d.date === pick) ?? days.find((d) => d.date === this.today) ?? days.find((d) => d.kind === 'workday') ?? null;
  });

  protected summary = computed(() => {
    const days = this.data()?.days ?? [];
    const counted = days.filter((d) => d.kind === 'workday');
    return {
      workdays: counted.length,
      holidays: days.filter((d) => d.kind === 'holiday').length,
      leaves: days.filter((d) => d.kind === 'leave').length,
      seconds: counted.reduce((sum, d) => sum + d.countedSeconds, 0),
    };
  });

  /** Weekly pattern rows: "Selasa" + the slot. */
  protected weeklyRows = computed(() => {
    const w = this.data()?.weekly;
    if (!w) return { dsu: [], rituals: [] };
    return {
      dsu: w.dsu.map((slot, i) => ({ weekday: WEEKDAYS[w.dsuWeekdays[i]], slot })),
      rituals: w.rituals.map((slot, i) => ({ weekday: WEEKDAYS[w.ritualWeekdays[i]], slot })),
    };
  });

  constructor() {
    // Reload whenever the month or the navbar's board changes.
    effect(() => {
      const month = this.month();
      const board = this.boards.selectedId();
      untracked(() => this.load(month, board));
    });
  }

  private load(month: string, board: number | null): void {
    this.loading.set(true);
    this.error.set(false);
    this.api.month(month, board).subscribe({
      next: (data) => {
        // A slower answer for a month or board the user already left must not overwrite the current one.
        if (month !== this.month() || board !== this.boards.selectedId()) return;
        this.data.set(data);
        this.loading.set(false);
      },
      error: () => {
        if (month !== this.month()) return;
        this.error.set(true);
        this.loading.set(false);
      },
    });
  }

  protected go(n: number): void {
    this.selected.set(null);
    this.month.set(shiftMonth(this.month(), n));
  }

  protected goToday(): void {
    this.selected.set(this.today);
    this.month.set(this.today.slice(0, 7));
  }

  protected pick(day: CalendarDay): void {
    this.selected.set(day.date);
  }

  protected retry(): void {
    this.load(this.month(), this.boards.selectedId());
  }

  /** The short chips shown inside a day cell: the DSU and special activities, not the lunch break. */
  protected chips(day: CalendarDay): CalendarSlot[] {
    return day.slots.filter((s) => s.kind !== 'lunch');
  }

  protected kindLabel(day: CalendarDay): string {
    switch (day.kind) {
      case 'holiday':
        return 'Tanggal merah (libur nasional)';
      case 'weekend':
        return 'Akhir pekan';
      case 'leave':
        return 'Cuti';
      default:
        return 'Hari kerja';
    }
  }

  protected minutes(slot: CalendarSlot): number {
    const [sh, sm] = slot.start.split(':').map(Number);
    const [eh, em] = slot.end.split(':').map(Number);
    return eh * 60 + em - (sh * 60 + sm);
  }
}
