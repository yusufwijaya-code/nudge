import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

export type DayKind = 'workday' | 'weekend' | 'holiday' | 'leave';
export type SlotKind = 'lunch' | 'dsu' | 'ritual';

/** A stretch of the working day that does not count toward ticket time. */
export interface CalendarSlot {
  kind: SlotKind;
  name: string;
  /** "HH:MM" (WIB). */
  start: string;
  end: string;
  /** Only in that week of the sprint; 0/absent = every week. */
  sprintWeek?: number;
}

export interface CalendarDay {
  /** "YYYY-MM-DD". */
  date: string;
  /** 0 = Sunday … 6 = Saturday. */
  weekday: number;
  kind: DayKind;
  /** Seconds that count toward a ticket's time that day (0 on a day off). */
  countedSeconds: number;
  /** The leave note. */
  note?: string;
  slots: CalendarSlot[];
}

export interface WorkCalendarMonth {
  month: string;
  board: { id: number; name: string; projectKey: string } | null;
  sprint: { name: string; start: string; end: string } | null;
  workStart: string;
  workEnd: string;
  lunch: CalendarSlot;
  /** What the board leaves out every week; the weekday arrays run parallel to the slot arrays. */
  weekly: { dsu: CalendarSlot[]; rituals: CalendarSlot[]; dsuWeekdays: number[]; ritualWeekdays: number[] };
  days: CalendarDay[];
}

@Injectable({ providedIn: 'root' })
export class WorkCalendarService {
  private http = inject(HttpClient);

  /** One month; board null = no board (only holidays, leave and the lunch break). */
  month(month: string, boardId: number | null): Observable<WorkCalendarMonth> {
    const params: Record<string, string> = { month };
    if (boardId) params['board'] = String(boardId);
    return this.http.get<WorkCalendarMonth>(`${environment.apiBasePath}/api/dashboard/calendar`, { params });
  }
}
