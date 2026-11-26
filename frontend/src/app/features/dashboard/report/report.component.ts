import { Component, DestroyRef, computed, inject, signal } from '@angular/core';
import { BoardContextService } from '../../../core/services/board-context.service';
import {
  ReportDeduction,
  ReportProgress,
  ReportService,
  ReportTicket,
  TimeReport,
  dayLabel,
  hm,
  pct,
  signedHm,
  thisMonth,
} from '../../../core/services/report.service';
import { ToastService } from '../../../core/ui/toast.service';
import { DateRange, DateRangePickerComponent } from '../../../shared/ui/date-range-picker.component';
import { IconComponent } from '../../../shared/ui/icon.component';
import { ReportProgressComponent } from './report-progress.component';

type Mode = 'show' | 'download';

const POLL_MS = 600;
const WIB = 'Asia/Jakarta';

function isoDay(d: Date): string {
  return d.toLocaleDateString('en-CA', { timeZone: WIB });
}

/** Quick ranges: this month, last month, the last 30 days, this quarter. */
export function presetRange(name: 'month' | 'lastMonth' | 'last30' | 'quarter', now = new Date()): { from: string; to: string } {
  const today = isoDay(now);
  const [y, m] = today.split('-').map(Number);
  switch (name) {
    case 'lastMonth': {
      const first = new Date(Date.UTC(y, m - 2, 1));
      const last = new Date(Date.UTC(y, m - 1, 0));
      return { from: first.toISOString().slice(0, 10), to: last.toISOString().slice(0, 10) };
    }
    case 'last30': {
      const from = new Date(Date.parse(`${today}T00:00:00Z`) - 29 * 86_400_000);
      return { from: from.toISOString().slice(0, 10), to: today };
    }
    case 'quarter': {
      const qm = Math.floor((m - 1) / 3) * 3 + 1;
      return { from: `${y}-${String(qm).padStart(2, '0')}-01`, to: today };
    }
    default:
      return thisMonth(now);
  }
}

/**
 * Laporan: the user's tickets created in a date range, on the board picked in the
 * navbar (or every board). Showing or downloading always reads Jira again; the raw
 * time is broken down into what is taken off (and why) before it is compared with the
 * estimate: deviation, score and the cascading key result.
 */
@Component({
  selector: 'app-report',
  standalone: true,
  imports: [IconComponent, DateRangePickerComponent, ReportProgressComponent],
  templateUrl: './report.component.html',
  styleUrl: './report.component.scss',
})
export class ReportComponent {
  private api = inject(ReportService);
  private toast = inject(ToastService);
  protected boards = inject(BoardContextService);

  protected readonly hm = hm;
  protected readonly signedHm = signedHm;
  protected readonly pct = pct;
  protected readonly dayLabel = dayLabel;
  protected readonly maxDay = isoDay(new Date());
  protected readonly presets: { key: 'month' | 'lastMonth' | 'last30' | 'quarter'; label: string }[] = [
    { key: 'month', label: 'Bulan ini' },
    { key: 'lastMonth', label: 'Bulan lalu' },
    { key: 'last30', label: '30 hari terakhir' },
    { key: 'quarter', label: 'Kuartal ini' },
  ];

  protected from = signal(thisMonth().from);
  protected to = signal(thisMonth().to);
  protected progress = signal<ReportProgress | null>(null);
  protected mode = signal<Mode>('show');
  protected report = signal<TimeReport | null>(null);
  protected error = signal<string | null>(null);
  protected expanded = signal<Set<string>>(new Set());
  /** The board the shown report is for; another one picked since → show again. */
  private reportBoard = signal<number | null>(null);
  private timer: ReturnType<typeof setTimeout> | null = null;
  private alive = true;

  protected busy = computed(() => this.progress()?.state === 'running');
  protected boardLabel = computed(() => this.boards.selected()?.name ?? 'Semua board');
  protected boardChanged = computed(() => !!this.report() && this.reportBoard() !== this.boards.selectedId());
  protected canRun = computed(() => !!this.from() && !!this.to() && this.from() <= this.to() && !this.busy());

  /** The widest bar of the waterfall is the raw time. */
  protected share(seconds: number): number {
    const raw = this.report()?.summary.rawSeconds ?? 0;
    return raw > 0 ? Math.max(0, Math.min(100, (seconds / raw) * 100)) : 0;
  }

  constructor() {
    inject(DestroyRef).onDestroy(() => {
      this.alive = false;
      if (this.timer) clearTimeout(this.timer);
    });
  }

  protected pick(range: DateRange): void {
    this.from.set(range.from);
    this.to.set(range.to || range.from);
  }

  protected usePreset(key: 'month' | 'lastMonth' | 'last30' | 'quarter'): void {
    const r = presetRange(key);
    this.from.set(r.from);
    this.to.set(r.to);
  }

  protected isPreset(key: 'month' | 'lastMonth' | 'last30' | 'quarter'): boolean {
    const r = presetRange(key);
    return r.from === this.from() && r.to === this.to();
  }

  /** Show or download: both read Jira again. */
  protected run(mode: Mode): void {
    if (!this.canRun()) return;
    this.mode.set(mode);
    this.error.set(null);
    const board = this.boards.selectedId();
    this.api.start(this.from(), this.to(), board).subscribe({
      next: (p) => {
        this.progress.set(p);
        this.poll(p.id, board);
      },
      error: (err) => this.error.set(err?.error?.message ?? 'Gagal memulai laporan, coba lagi.'),
    });
  }

  private poll(id: string, board: number | null): void {
    if (this.timer) clearTimeout(this.timer);
    this.timer = setTimeout(() => {
      if (!this.alive) return;
      this.api.progress(id).subscribe({
        next: (p) => {
          if (!this.alive) return;
          this.progress.set(p);
          switch (p.state) {
            case 'running':
              this.poll(id, board);
              return;
            case 'done':
              this.finish(p, board);
              return;
            case 'cancelled':
              this.progress.set(null);
              return;
            default:
              this.progress.set(null);
              this.error.set(p.error ?? 'Laporan gagal dibuat, coba lagi.');
          }
        },
        error: (err) => {
          this.progress.set(null);
          this.error.set(err?.error?.message ?? 'Koneksi terputus saat membuat laporan, coba lagi.');
        },
      });
    }, POLL_MS);
  }

  private finish(p: ReportProgress, board: number | null): void {
    this.report.set(p.report ?? null);
    this.reportBoard.set(board);
    this.expanded.set(new Set());
    if (this.mode() !== 'download') {
      // Let the "ready" state show for a moment before the pop up goes.
      setTimeout(() => this.alive && this.progress.set(null), 450);
      return;
    }
    this.api.pdf(p.id).subscribe({
      next: (blob) => {
        this.save(blob, this.fileName(p.report));
        this.progress.set(null);
        this.toast.success('PDF diunduh', 'Laporan juga ditampilkan di halaman ini.');
      },
      error: () => {
        this.progress.set(null);
        this.toast.error('Gagal mengunduh PDF', 'Data laporan tetap ditampilkan; coba unduh lagi.');
      },
    });
  }

  private fileName(r?: TimeReport): string {
    const name = (r?.userName || 'user').replace(/[^A-Za-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'user';
    return `Laporan-Nudge-${name}-${r?.from ?? this.from()}_${r?.to ?? this.to()}.pdf`;
  }

  private save(blob: Blob, name: string): void {
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = name;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 2000);
  }

  protected cancel(): void {
    const p = this.progress();
    if (!p) return;
    if (this.timer) clearTimeout(this.timer);
    this.progress.set(null);
    this.api.cancel(p.id).subscribe({ error: () => undefined });
    this.toast.info('Laporan dibatalkan');
  }

  protected toggle(key: string): void {
    this.expanded.update((s) => {
      const next = new Set(s);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }

  protected isOpen(key: string): boolean {
    return this.expanded().has(key);
  }

  protected dedColor(d: ReportDeduction): string {
    return `ded-${d.category}`;
  }

  protected statusList(r: TimeReport): string {
    return r.projects
      .map((p) => (r.statuses[p]?.length ? `${p}: ${r.statuses[p].join(', ')}` : ''))
      .filter(Boolean)
      .join('; ');
  }

  protected achievementTone(r: TimeReport): string {
    if (r.summary.scoredCount === 0) return 'muted';
    if (r.summary.achievementPercent >= 100) return 'good';
    return r.summary.achievementPercent >= 50 ? 'warn' : 'bad';
  }

  protected trackTicket(_: number, t: ReportTicket): string {
    return t.key;
  }
}
