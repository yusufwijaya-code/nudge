import { Component, DestroyRef, NgZone, computed, inject, input, output, signal } from '@angular/core';
import { ReportProgress } from '../../../core/services/report.service';
import { IconComponent } from '../../../shared/ui/icon.component';

const TIPS = [
  'Riwayat status dibaca langsung dari Jira, jadi angkanya selalu yang terbaru.',
  'Istirahat, akhir pekan, libur nasional, cuti, dan jadwal board dipisahkan satu per satu.',
  'Setiap detik hanya dihitung sekali, walau jadwalnya bertumpuk.',
  'Tiket tanpa Story Point tetap dicatat, tapi tidak ikut skor.',
];

/**
 * The pop up while a report is being made: overall progress, the step it is on, and
 * for the longest step which ticket is being read, so it never looks stuck.
 */
@Component({
  selector: 'app-report-progress',
  standalone: true,
  imports: [IconComponent],
  templateUrl: './report-progress.component.html',
  styleUrl: './report-progress.component.scss',
})
export class ReportProgressComponent {
  progress = input.required<ReportProgress>();
  /** 'download' says the PDF follows. */
  mode = input<'show' | 'download'>('show');
  cancel = output<void>();

  protected elapsed = signal(0);
  protected tip = signal(0);
  protected readonly tips = TIPS;

  protected stepIndex = computed(() => this.progress().step);
  protected headline = computed(() => {
    const p = this.progress();
    if (p.state === 'done') return this.mode() === 'download' ? 'Laporan siap, mengunduh PDF…' : 'Laporan siap';
    return p.steps[p.step] ?? 'Menyiapkan laporan';
  });
  protected detail = computed(() => {
    const p = this.progress();
    if (p.step === 2 && p.total > 0) return `${p.done} dari ${p.total} tiket${p.current ? ' · ' + p.current : ''}`;
    if (p.step === 3 && p.total > 0) return `${p.done} dari ${p.total} tiket dihitung`;
    if (p.step === 0) return 'Memastikan data Nudge sama dengan Jira';
    if (p.step === 1) return 'Mencari tiket yang dibuat pada rentang tanggal ini';
    return 'Menjumlahkan semuanya';
  });

  constructor() {
    const zone = inject(NgZone);
    const timer = zone.runOutsideAngular(() =>
      setInterval(() => zone.run(() => {
        this.elapsed.update((n) => n + 1);
        if (this.elapsed() % 5 === 0) this.tip.update((i) => (i + 1) % TIPS.length);
      }), 1000),
    );
    inject(DestroyRef).onDestroy(() => clearInterval(timer));
  }

  protected stepState(i: number): 'done' | 'active' | 'pending' {
    const p = this.progress();
    if (p.state === 'done' || i < p.step) return 'done';
    return i === p.step ? 'active' : 'pending';
  }

  protected elapsedLabel(): string {
    const s = this.elapsed();
    return s < 60 ? `${s} detik` : `${Math.floor(s / 60)} menit ${s % 60} detik`;
  }
}
