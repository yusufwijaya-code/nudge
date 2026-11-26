import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

/** Time taken off the raw time, by reason (a board slot by its name). */
export interface ReportDeduction {
  key: string;
  label: string;
  /** outside | lunch | weekend | holiday | leave | slot */
  category: string;
  seconds: number;
}

export interface ReportStatusTime {
  status: string;
  rawSeconds: number;
  countedSeconds: number;
}

export interface ReportTicket {
  key: string;
  summary: string;
  type: string;
  typeIconUrl?: string;
  status: string;
  url: string;
  created: string;
  sprints: string[];
  storyPoints: number;
  estimateSeconds: number;
  rawSeconds: number;
  countedSeconds: number;
  deductions: ReportDeduction[];
  deductedSeconds: number;
  statusTimes: ReportStatusTime[];
  firstStart?: string;
  lastEnd?: string;
  running: boolean;
  deviationPercent: number;
  scorePercent: number;
  within: boolean;
}

export interface ReportExcluded {
  key: string;
  summary: string;
  status: string;
  url: string;
  reason: string;
  countedSeconds: number;
}

export interface ReportSummary {
  ticketCount: number;
  scoredCount: number;
  withinCount: number;
  estimateSeconds: number;
  rawSeconds: number;
  deductedSeconds: number;
  countedSeconds: number;
  aggregateDeviationPercent: number;
  scorePercent: number;
  achievementPercent: number;
  withinPercent: number;
  within: boolean;
}

export interface TimeReport {
  generatedAt: string;
  userName: string;
  userEmail: string;
  boardId: number;
  boardName: string;
  projects: string[];
  from: string;
  to: string;
  capPercent: number;
  statuses: Record<string, string[]>;
  summary: ReportSummary;
  deductions: ReportDeduction[];
  tickets: ReportTicket[];
  excluded: ReportExcluded[];
  truncated: boolean;
  syncWarning?: string;
}

export type ReportState = 'running' | 'done' | 'failed' | 'cancelled';

/** A report being made (GET /reports/jobs/:id): polled until it is no longer running. */
export interface ReportProgress {
  id: string;
  state: ReportState;
  step: number;
  steps: string[];
  done: number;
  total: number;
  current?: string;
  percent: number;
  error?: string;
  params: { from: string; to: string; boardId: number };
  report?: TimeReport;
}

/** The user's Laporan: every run reads Jira again (POST starts it, then poll). */
@Injectable({ providedIn: 'root' })
export class ReportService {
  private http = inject(HttpClient);
  private base = `${environment.apiBasePath}/api/dashboard/reports/jobs`;

  start(from: string, to: string, boardId: number | null): Observable<ReportProgress> {
    return this.http.post<ReportProgress>(this.base, { from, to, boardId: boardId ?? 0 });
  }

  progress(id: string): Observable<ReportProgress> {
    return this.http.get<ReportProgress>(`${this.base}/${id}`);
  }

  cancel(id: string): Observable<void> {
    return this.http.delete<void>(`${this.base}/${id}`);
  }

  /** The finished report as a PDF file. */
  pdf(id: string): Observable<Blob> {
    return this.http.get(`${this.base}/${id}/pdf`, { responseType: 'blob' });
  }
}

/** "3j 05m", "45m", "0m" — hours and minutes, like the PDF. */
export function hm(seconds: number): string {
  const sign = seconds < 0 ? '-' : '';
  const minutes = Math.round(Math.abs(seconds) / 60);
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (h === 0) return `${sign}${m}m`;
  if (m === 0) return `${sign}${h}j`;
  return `${sign}${h}j ${String(m).padStart(2, '0')}m`;
}

/** "+20m" for more than the estimate, "-1j 05m" for less. */
export function signedHm(seconds: number): string {
  return seconds > 0 ? `+${hm(seconds)}` : hm(seconds);
}

/** "12,5%" — one decimal at most, Indonesian comma. */
export function pct(v: number, digits = 1): string {
  const fixed = v.toFixed(digits);
  const trimmed = fixed.includes('.') ? fixed.replace(/\.?0+$/, '') : fixed;
  return `${trimmed.replace('.', ',')}%`;
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des'];

/** "2026-11-03" or an RFC3339 time → "3 Nov 2026" (WIB). */
export function dayLabel(value: string): string {
  if (!value) return '';
  const iso = /^\d{4}-\d{2}-\d{2}$/.test(value) ? value : new Date(value).toLocaleDateString('en-CA', { timeZone: 'Asia/Jakarta' });
  const [y, m, d] = iso.split('-').map(Number);
  return `${d} ${MONTHS[m - 1]} ${y}`;
}

/** Today and the first day of this month, "YYYY-MM-DD" on the WIB calendar. */
export function thisMonth(now = new Date()): { from: string; to: string } {
  const today = now.toLocaleDateString('en-CA', { timeZone: 'Asia/Jakarta' });
  return { from: `${today.slice(0, 7)}-01`, to: today };
}
