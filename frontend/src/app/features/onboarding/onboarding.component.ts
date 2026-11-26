import { Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import {
  OnboardingBoard,
  OnboardingField,
  OnboardingFieldName,
  OnboardingForm,
  OnboardingResult,
  OnboardingService,
} from '../../core/services/onboarding.service';
import { BoardAvatarComponent } from '../../shared/ui/board-avatar.component';
import { IconComponent } from '../../shared/ui/icon.component';
import { SelectComponent, SelectOption } from '../../shared/ui/select.component';
import { environment } from '../../../environments/environment';

function has(list: string[], v: string): boolean {
  return list.some((x) => x.toLowerCase() === v.toLowerCase());
}

/**
 * Langkah 2 (atur board): a user whose Jira board has no Aturan Board yet picks
 * its statuses and Story Point fields (prefilled from a similar board) and saves
 * once; the board is then synced from Jira before the dashboard opens.
 */
@Component({
  selector: 'app-onboarding',
  standalone: true,
  imports: [FormsModule, IconComponent, SelectComponent, BoardAvatarComponent],
  templateUrl: './onboarding.component.html',
  styleUrl: './onboarding.component.scss',
})
export class OnboardingComponent {
  private api = inject(OnboardingService);
  private router = inject(Router);
  private wanted = Number(inject(ActivatedRoute).snapshot.queryParamMap.get('board')) || null;

  protected loading = signal(true);
  protected loadError = signal<string | null>(null);
  protected boards = signal<OnboardingBoard[]>([]);
  protected pending = computed(() => this.boards().filter((b) => !b.registered));
  protected registeredCount = computed(() => this.boards().filter((b) => b.registered).length);

  protected current = signal<OnboardingBoard | null>(null);
  protected form = signal<OnboardingForm | null>(null);
  protected formLoading = signal(false);

  protected tracked = signal<string[]>([]);
  protected done = signal('');
  protected review = signal('');
  protected spFields = signal<string[]>([]);
  protected writeField = signal('');
  protected writeFieldSubtask = signal('');

  protected saving = signal(false);
  protected syncing = signal(false);
  protected saveError = signal<string | null>(null);
  protected fieldErrors = signal<Partial<Record<OnboardingFieldName, string>>>({});
  protected result = signal<OnboardingResult | null>(null);

  protected logoutHref = `${environment.apiBasePath}/auth/logout`;

  protected statuses = computed(() => this.form()?.statuses ?? []);
  protected numberFields = computed<OnboardingField[]>(() => this.form()?.numberFields ?? []);
  protected doneOptions = computed<SelectOption<string>[]>(() => [
    { value: '', label: 'Pilih status selesai…' },
    ...this.statuses().filter((s) => !has(this.tracked(), s)).map((s) => ({ value: s, label: s })),
  ]);
  protected reviewOptions = computed<SelectOption<string>[]>(() => [
    { value: '', label: 'Tidak ada' },
    ...this.tracked().map((s) => ({ value: s, label: s })),
  ]);
  protected writeOptions = computed<SelectOption<string>[]>(() => [
    { value: '', label: 'Pilih field…' },
    ...this.spFields().map((id) => ({ value: id, label: this.fieldName(id) })),
  ]);

  protected canSave = computed(
    () =>
      !this.saving() &&
      !!this.form() &&
      this.tracked().length > 0 &&
      !!this.done() &&
      this.spFields().length > 0 &&
      !!this.writeField() &&
      !!this.writeFieldSubtask(),
  );

  constructor() {
    this.api.boards(true).subscribe({
      next: (res) => {
        const boards = res.boards ?? [];
        this.boards.set(boards);
        this.loading.set(false);
        const pending = boards.filter((b) => !b.registered);
        if (!pending.length) {
          // Everything is registered (or the user has no board yet): nothing to set up.
          this.router.navigateByUrl('/dashboard');
          return;
        }
        this.choose(pending.find((b) => b.boardId === this.wanted) ?? pending[0]);
      },
      error: () => {
        this.loadError.set('Gagal membaca board kamu dari Jira. Coba muat ulang halaman ini.');
        this.loading.set(false);
      },
    });
  }

  protected fieldName(id: string): string {
    return this.numberFields().find((f) => f.id === id)?.name ?? id;
  }

  protected choose(board: OnboardingBoard): void {
    if (this.saving()) return;
    this.current.set(board);
    this.form.set(null);
    this.saveError.set(null);
    this.fieldErrors.set({});
    this.formLoading.set(true);
    this.api.form(board.boardId).subscribe({
      next: (f) => {
        if (this.current()?.boardId !== board.boardId) return;
        this.form.set(f);
        const d = f.defaults;
        this.tracked.set(d.trackedStatuses ?? []);
        this.done.set(d.doneStatus ?? '');
        this.review.set(d.reviewStatus ?? '');
        this.spFields.set(d.storyPointFields ?? []);
        this.writeField.set(d.storyPointWriteField ?? '');
        this.writeFieldSubtask.set(d.storyPointWriteFieldSubtask ?? '');
        this.formLoading.set(false);
      },
      error: (err) => {
        this.formLoading.set(false);
        if (err?.status === 409) {
          // Registered in the meantime (a teammate saved it): carry on.
          this.markRegistered(board.boardId);
          return;
        }
        this.loadError.set(err?.error?.message ?? 'Gagal memuat data board dari Jira. Coba lagi.');
      },
    });
  }

  protected isTracked(s: string): boolean { return has(this.tracked(), s); }

  protected toggleTracked(s: string): void {
    if (this.isTracked(s)) {
      this.tracked.update((l) => l.filter((x) => x.toLowerCase() !== s.toLowerCase()));
      if (this.review().toLowerCase() === s.toLowerCase()) this.review.set('');
    } else {
      this.tracked.update((l) => [...l, s]);
      if (this.done().toLowerCase() === s.toLowerCase()) this.done.set('');
    }
  }

  protected hasField(id: string): boolean { return this.spFields().includes(id); }

  protected toggleField(id: string): void {
    if (this.hasField(id)) {
      this.spFields.update((l) => l.filter((x) => x !== id));
    } else {
      this.spFields.update((l) => [...l, id]);
    }
    const left = this.spFields();
    if (!left.includes(this.writeField())) this.writeField.set(left[0] ?? '');
    if (!left.includes(this.writeFieldSubtask())) this.writeFieldSubtask.set(left[0] ?? '');
  }

  protected save(): void {
    const board = this.current();
    if (!board || !this.canSave()) return;
    this.saving.set(true);
    this.saveError.set(null);
    this.fieldErrors.set({});
    // The server answers after it has synced the user's tickets from Jira.
    this.syncing.set(true);
    this.api
      .setup(board.boardId, {
        trackedStatuses: this.tracked(),
        doneStatus: this.done(),
        reviewStatus: this.review() || null,
        storyPointFields: this.spFields(),
        storyPointWriteField: this.writeField(),
        storyPointWriteFieldSubtask: this.writeFieldSubtask(),
      })
      .subscribe({
        next: (res) => {
          this.saving.set(false);
          this.syncing.set(false);
          this.result.set(res);
          this.markRegistered(board.boardId);
        },
        error: (err) => {
          this.saving.set(false);
          this.syncing.set(false);
          const body = err?.error ?? {};
          if (err?.status === 409) {
            this.markRegistered(board.boardId);
            return;
          }
          if (body.fields) this.fieldErrors.set(body.fields);
          this.saveError.set(body.message ?? 'Gagal menyimpan board. Coba lagi.');
        },
      });
  }

  /** Marks the board done; moves to the next new board, or opens the dashboard on it. */
  private markRegistered(boardId: number): void {
    this.boards.update((l) => l.map((b) => (b.boardId === boardId ? { ...b, registered: true } : b)));
    const next = this.pending()[0];
    if (next) {
      this.choose(next);
      return;
    }
    this.router.navigate(['/dashboard'], { queryParams: { board: boardId } });
  }
}
