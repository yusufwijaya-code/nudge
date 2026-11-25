import { Component, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { HttpErrorResponse } from '@angular/common/http';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import {
  CreateFieldKind,
  CreateFieldType,
  ExcludedSchedule,
  RitualFrequency,
  JiraProjectLookup,
  PROJECT_KEY_PATTERN,
  TrackingRule,
  TrackingRuleError,
  TrackingRuleField,
  TrackingRuleInput,
  TrackingRuleService,
  normalizeProjectKey,
} from '../../../core/services/tracking-rule.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { SelectComponent, SelectOption } from '../../../shared/ui/select.component';
import { TimePickerComponent } from '../../../shared/ui/time-picker.component';
import { formatDuration } from '../../../core/duration-format';
import { recalculateAction } from './recalculate-action';

const MAX_FIELD_SUGGESTIONS = 12;

const KIND_LABELS: Record<CreateFieldKind, string> = {
  text: 'Teks', textarea: 'Teks panjang', number: 'Angka', select: 'Pilihan', cascade: 'Pilihan bertingkat', date: 'Tanggal', labels: 'Label',
};

const WEEKDAYS = [
  { value: 1, label: 'Senin' },
  { value: 2, label: 'Selasa' },
  { value: 3, label: 'Rabu' },
  { value: 4, label: 'Kamis' },
  { value: 5, label: 'Jumat' },
];
const WEEKDAY_OPTIONS: SelectOption<number>[] = WEEKDAYS.map((d) => ({ value: d.value, label: d.label }));
const FREQUENCY_OPTIONS: SelectOption<RitualFrequency>[] = [
  { value: 'weekly', label: 'Setiap minggu' },
  { value: 'sprint_week', label: 'Hanya pada minggu ke-N sprint' },
];
const SPRINT_WEEK_OPTIONS: SelectOption<number>[] = [1, 2, 3, 4].map((n) => ({ value: n, label: `Minggu ke-${n}` }));

export const MAX_SLOT_MINUTES = 540;
const MAX_RITUALS = 10;

/** One weekday of the Daily Stand Up: off until ticked. */
export interface DsuRow {
  weekday: number;
  label: string;
  on: boolean;
  start: string;
  minutes: number;
}

export interface RitualRow {
  name: string;
  enabled: boolean;
  weekday: number;
  start: string;
  minutes: number;
  frequency: RitualFrequency;
  sprintWeek: number;
}

function defaultDsuRows(): DsuRow[] {
  return WEEKDAYS.map((d) => ({ weekday: d.value, label: d.label, on: false, start: '08:45', minutes: 30 }));
}

/** "HH:MM" plus minutes, capped at 24:00 ("11:00" + 390 = "17:30"). */
export function slotEnd(start: string, minutes: number): string {
  const [h, m] = (start || '00:00').split(':').map(Number);
  const end = Math.min(24 * 60, (h || 0) * 60 + (m || 0) + (minutes || 0));
  return `${String(Math.floor(end / 60)).padStart(2, '0')}:${String(end % 60).padStart(2, '0')}`;
}

/** Keeps a typed length inside what the server accepts. */
export function clampMinutes(raw: unknown): number {
  const n = Math.round(Number(raw));
  if (!Number.isFinite(n) || n < 1) return 1;
  return Math.min(n, MAX_SLOT_MINUTES);
}

function hasFold(list: string[], value: string): boolean {
  return list.some((v) => v.toLowerCase() === value.toLowerCase());
}

/**
 * Tambah / edit satu aturan board (Tech Spec "Aturan Board (Admin CRUD)",
 * US-3/US-4): statuses and Story Point fields are picked from the project's
 * Jira data; stored values Jira no longer has are flagged ⚠ and must be
 * dropped before saving. "Field form Buat Card" picks, per issue type, the
 * optional Jira fields the create form shows (required ones always show).
 */
@Component({
  selector: 'app-tracking-rule-form',
  standalone: true,
  imports: [FormsModule, RouterLink, IconComponent, SelectComponent, TimePickerComponent],
  templateUrl: './tracking-rule-form.component.html',
  styleUrl: './tracking-rule-form.component.scss',
})
export class TrackingRuleFormComponent {
  private api = inject(TrackingRuleService);
  private toast = inject(ToastService);
  private router = inject(Router);

  /** The rule being edited (route /admin/tracking-rules/:key); null = new rule. */
  protected readonly editKey: string | null = inject(ActivatedRoute).snapshot.paramMap.get('key');
  protected readonly isEdit = this.editKey !== null;

  protected keyInput = signal(this.editKey ?? '');
  protected lookup = signal<JiraProjectLookup | null>(null);
  protected lookingUp = signal(false);
  protected lookupError = signal<string | null>(null);
  protected loadError = signal<string | null>(null);
  protected avatarFailed = signal(false);

  protected tracked = signal<string[]>([]);
  protected done = signal('');
  protected review = signal('');
  protected spFields = signal<string[]>([]);
  protected writeField = signal('');
  protected writeFieldSubtask = signal('');
  protected fieldQuery = signal('');

  /** Issue type id → optional fields shown on Buat Card. */
  protected createFields = signal<Record<string, string[]>>({});
  protected cfTypes = signal<CreateFieldType[] | null>(null);
  protected cfLoading = signal(false);
  protected cfError = signal<string | null>(null);
  protected cfTypeId = signal('');
  protected readonly kindLabels = KIND_LABELS;
  /** The selected type, without the Story Point fields (Buat Card has its own SP input). */
  protected cfType = computed(() => {
    const t = (this.cfTypes() ?? []).find((x) => x.typeId === this.cfTypeId());
    if (!t) return null;
    const sp = new Set(this.spFields());
    return { ...t, fields: t.fields.filter((f) => !sp.has(f.id)) };
  });
  /** Total optional fields chosen, over the types Jira still has. */
  protected cfTotal = computed(() => (this.cfTypes() ?? []).reduce((n, t) => n + this.cfCount(t.typeId), 0));

  /** Hours that are not counted toward a ticket's time on this board. */
  protected readonly weekdayOptions = WEEKDAY_OPTIONS;
  protected readonly frequencyOptions = FREQUENCY_OPTIONS;
  protected readonly sprintWeekOptions = SPRINT_WEEK_OPTIONS;
  protected readonly maxMinutes = MAX_SLOT_MINUTES;
  protected readonly slotEnd = slotEnd;
  protected dsuRows = signal<DsuRow[]>(defaultDsuRows());
  protected rituals = signal<RitualRow[]>([]);
  protected canAddRitual = computed(() => this.rituals().length < MAX_RITUALS);
  /** How long a slot is, in words: 390 → "6 jam 30 menit". */
  protected slotLength(minutes: number): string {
    return formatDuration(minutes * 60);
  }

  protected saving = signal(false);
  protected fieldErrors = signal<Partial<Record<TrackingRuleField, string>>>({});
  protected existingKey = signal<string | null>(null);
  /** Field names of the stored rule, shown until (or without) the Jira lookup. */
  private storedNames = signal(new Map<string, string>());

  protected keyValid = computed(() => PROJECT_KEY_PATTERN.test(normalizeProjectKey(this.keyInput())));
  protected statuses = computed(() => this.lookup()?.statuses ?? []);
  private fieldNames = computed(() => {
    const names = new Map(this.storedNames());
    for (const f of this.lookup()?.numberFields ?? []) names.set(f.id, f.name);
    return names;
  });

  /** Stored statuses no longer in the project's Jira workflow (AC4.4). */
  protected staleStatuses = computed(() => {
    if (!this.lookup()) return [];
    const all = this.statuses();
    const used = [...this.tracked(), this.done(), this.review()].filter((s) => s !== '');
    return [...new Set(used)].filter((s) => !hasFold(all, s));
  });
  /** Stored Story Point fields Jira no longer has as a number field. */
  protected staleFields = computed(() => {
    const l = this.lookup();
    if (!l) return [];
    return this.spFields().filter((id) => !l.numberFields.some((f) => f.id === id));
  });

  protected doneOptions = computed<SelectOption<string>[]>(() => [
    { value: '', label: 'Pilih status selesai…' },
    ...this.statuses()
      .filter((s) => !hasFold(this.tracked(), s))
      .map((s) => ({ value: s, label: s })),
  ]);
  protected reviewOptions = computed<SelectOption<string>[]>(() => [
    { value: '', label: 'Tidak ada — review tidak dihitung' },
    ...this.tracked().map((s) => ({ value: s, label: s })),
  ]);
  protected writeOptions = computed<SelectOption<string>[]>(() => [
    { value: '', label: 'Pilih field…' },
    ...this.spFields().map((id) => ({ value: id, label: this.fieldLabel(id) })),
  ]);
  /** Number fields not picked yet, story-point-like first (server order), filtered by the search box. */
  protected suggestions = computed(() => {
    const q = this.fieldQuery().trim().toLowerCase();
    const picked = this.spFields();
    return (this.lookup()?.numberFields ?? [])
      .filter((f) => !picked.includes(f.id))
      .filter((f) => !q || f.name.toLowerCase().includes(q) || f.id.includes(q))
      .slice(0, MAX_FIELD_SUGGESTIONS);
  });

  /** AC3.3 — live sentence of what the rule does. */
  protected preview = computed(() => {
    const label = (id: string) => (id ? this.fieldLabel(id) : '…');
    const tracked = this.tracked();
    const fields = this.spFields();
    return (
      `Budget berjalan selama ${tracked.length ? tracked.join(' atau ') : '…'}; selesai saat ${this.done() || '…'}. ` +
      `Review ${this.review() ? 'ikut' : 'tidak'} dihitung. ` +
      `SP dibaca dari ${fields.length ? fields.map(label).join(' → ') : '…'}; ` +
      `ditulis ke ${label(this.writeField())} (card biasa) / ${label(this.writeFieldSubtask())} (subtask).`
    );
  });

  protected canSave = computed(
    () =>
      !!this.lookup() &&
      !this.saving() &&
      this.tracked().length > 0 &&
      this.done() !== '' &&
      this.spFields().length > 0 &&
      this.writeField() !== '' &&
      this.writeFieldSubtask() !== '' &&
      this.staleStatuses().length === 0 &&
      this.staleFields().length === 0 &&
      this.rituals().every((r) => r.name.trim() !== ''),
  );

  constructor() {
    if (this.editKey) this.loadRule(this.editKey);
  }

  protected fieldLabel(id: string): string {
    const name = this.fieldNames().get(id);
    return name ? `${name} (${id})` : id;
  }

  private loadRule(key: string): void {
    this.api.list().subscribe({
      next: (res) => {
        const rule = res.rules.find((r) => r.projectKey === key);
        if (!rule) {
          this.loadError.set(`Aturan untuk ${key} tidak ditemukan.`);
          return;
        }
        this.apply(rule);
        // AC4.1: reload the Jira data so the choices are current.
        this.runLookup();
      },
      error: () => this.loadError.set('Gagal memuat aturan, coba lagi.'),
    });
  }

  private apply(rule: TrackingRule): void {
    this.storedNames.set(new Map(rule.storyPointFields.map((f) => [f.id, f.name] as [string, string])));
    this.tracked.set([...rule.trackedStatuses]);
    this.done.set(rule.doneStatus);
    this.review.set(rule.reviewStatus ?? '');
    this.spFields.set(rule.storyPointFields.map((f) => f.id));
    this.writeField.set(rule.storyPointWriteField);
    this.writeFieldSubtask.set(rule.storyPointWriteFieldSubtask);
    this.createFields.set(Object.fromEntries(Object.entries(rule.createFields ?? {}).map(([k, v]) => [k, [...v]])));
    this.applyExcluded(rule.excluded);
  }

  private applyExcluded(excluded: ExcludedSchedule | undefined): void {
    this.dsuRows.set(
      defaultDsuRows().map((row) => {
        const slot = (excluded?.dsu ?? []).find((d) => d.weekday === row.weekday);
        return slot ? { ...row, on: true, start: slot.start, minutes: slot.minutes } : row;
      }),
    );
    this.rituals.set((excluded?.rituals ?? []).map((r) => ({ ...r })));
  }

  protected onKeyInput(value: string): void {
    this.keyInput.set(value ?? '');
    const current = this.lookup();
    // Another key: what was picked for the old project no longer applies.
    if (current && normalizeProjectKey(value ?? '') !== current.projectKey) {
      this.lookup.set(null);
      this.tracked.set([]);
      this.done.set('');
      this.review.set('');
      this.spFields.set([]);
      this.writeField.set('');
      this.writeFieldSubtask.set('');
      this.createFields.set({});
      this.applyExcluded(undefined);
      this.cfTypes.set(null);
      this.cfError.set(null);
    }
  }

  protected runLookup(): void {
    const key = normalizeProjectKey(this.keyInput());
    if (!PROJECT_KEY_PATTERN.test(key) || this.lookingUp()) return;
    this.keyInput.set(key);
    this.lookingUp.set(true);
    this.lookupError.set(null);
    this.existingKey.set(null);
    this.avatarFailed.set(false);
    // Edit (load + "Muat ulang dari Jira") reads Jira fresh, past the server cache.
    this.api.lookup(key, this.isEdit).subscribe({
      next: (l) => {
        this.lookingUp.set(false);
        this.lookup.set(l);
        this.loadCreateFields();
      },
      error: (err: HttpErrorResponse) => {
        this.lookingUp.set(false);
        this.lookup.set(null);
        this.lookupError.set(
          err?.status === 404
            ? `Project ${key} tidak ditemukan di Jira`
            : err?.status === 400
              ? 'Format project key tidak valid'
              : 'Jira tidak merespons, coba lagi',
        );
      },
    });
  }

  /** Issue types + fillable fields for "Field form Buat Card"; a failure only affects that section. */
  protected loadCreateFields(): void {
    const key = this.lookup()?.projectKey;
    if (!key) return;
    this.cfLoading.set(true);
    this.cfError.set(null);
    this.api.createFields(key, this.isEdit).subscribe({
      next: (res) => {
        if (this.lookup()?.projectKey !== key) return;
        this.cfLoading.set(false);
        const types = res.types ?? [];
        this.cfTypes.set(types);
        if (!types.some((t) => t.typeId === this.cfTypeId())) this.cfTypeId.set(types.find((t) => !t.subtask)?.typeId ?? types[0]?.typeId ?? '');
      },
      error: () => {
        this.cfLoading.set(false);
        this.cfTypes.set(null);
        this.cfError.set('Gagal memuat field Jira, coba lagi.');
      },
    });
  }

  protected cfCount(typeId: string): number {
    const t = (this.cfTypes() ?? []).find((x) => x.typeId === typeId);
    const chosen = this.createFields()[typeId] ?? [];
    return t ? t.fields.filter((f) => !f.required && chosen.includes(f.id)).length : chosen.length;
  }

  protected cfChosen(typeId: string, fieldId: string): boolean {
    return (this.createFields()[typeId] ?? []).includes(fieldId);
  }

  protected toggleCreateField(typeId: string, fieldId: string): void {
    const cur = this.createFields()[typeId] ?? [];
    const next = cur.includes(fieldId) ? cur.filter((f) => f !== fieldId) : [...cur, fieldId];
    this.createFields.set({ ...this.createFields(), [typeId]: next });
    this.clearErrors('createFields');
  }

  /** What is saved: with Jira's list loaded, only optional fields Jira still has on types it still has. */
  private createFieldsInput(): Record<string, string[]> {
    const types = this.cfTypes();
    const out: Record<string, string[]> = {};
    for (const [typeId, ids] of Object.entries(this.createFields())) {
      const t = types?.find((x) => x.typeId === typeId);
      if (types && !t) continue;
      const keep = t ? ids.filter((id) => t.fields.some((f) => f.id === id && !f.required)) : ids;
      if (keep.length) out[typeId] = keep;
    }
    return out;
  }

  protected isTracked(status: string): boolean {
    return hasFold(this.tracked(), status);
  }

  protected toggleTracked(status: string): void {
    const next = this.isTracked(status)
      ? this.tracked().filter((s) => s.toLowerCase() !== status.toLowerCase())
      : [...this.tracked(), status];
    this.tracked.set(next);
    if (this.done() && hasFold(next, this.done())) this.done.set('');
    if (this.review() && !hasFold(next, this.review())) this.review.set('');
    this.clearErrors('trackedStatuses', 'doneStatus', 'reviewStatus');
  }

  protected setDone(value: string): void {
    this.done.set(value ?? '');
    this.clearErrors('doneStatus');
  }

  protected setReview(value: string): void {
    this.review.set(value ?? '');
    this.clearErrors('reviewStatus');
  }

  protected setWrite(which: 'card' | 'subtask', value: string): void {
    if (which === 'card') {
      this.writeField.set(value ?? '');
      this.clearErrors('storyPointWriteField');
    } else {
      this.writeFieldSubtask.set(value ?? '');
      this.clearErrors('storyPointWriteFieldSubtask');
    }
  }

  protected addField(id: string): void {
    if (this.spFields().includes(id)) return;
    const next = [...this.spFields(), id];
    this.spFields.set(next);
    // Default write field = the first read field.
    if (!this.writeField()) this.writeField.set(next[0]);
    if (!this.writeFieldSubtask()) this.writeFieldSubtask.set(next[0]);
    this.fieldQuery.set('');
    this.clearErrors('storyPointFields');
  }

  protected moveField(index: number, step: -1 | 1): void {
    const list = [...this.spFields()];
    const target = index + step;
    if (target < 0 || target >= list.length) return;
    [list[index], list[target]] = [list[target], list[index]];
    this.spFields.set(list);
  }

  protected removeField(id: string): void {
    this.spFields.set(this.spFields().filter((f) => f !== id));
    // A write dropdown pointing at the removed field is reset.
    if (this.writeField() === id) this.writeField.set('');
    if (this.writeFieldSubtask() === id) this.writeFieldSubtask.set('');
  }

  protected dropStaleStatus(status: string): void {
    this.tracked.set(this.tracked().filter((s) => s !== status));
    if (this.done() === status) this.done.set('');
    if (this.review() === status || (this.review() && !hasFold(this.tracked(), this.review()))) this.review.set('');
  }

  protected clampedMinutes(raw: unknown): number {
    return clampMinutes(raw);
  }

  protected setDsu(index: number, patch: Partial<DsuRow>): void {
    this.dsuRows.set(this.dsuRows().map((row, i) => (i === index ? { ...row, ...patch } : row)));
    this.clearErrors('excluded');
  }

  protected addRitual(): void {
    if (!this.canAddRitual()) return;
    this.rituals.set([...this.rituals(), { name: '', enabled: true, weekday: 1, start: '11:00', minutes: 60, frequency: 'weekly', sprintWeek: 2 }]);
    this.clearErrors('excluded');
  }

  protected setRitual(index: number, patch: Partial<RitualRow>): void {
    this.rituals.set(this.rituals().map((row, i) => (i === index ? { ...row, ...patch } : row)));
    this.clearErrors('excluded');
  }

  protected removeRitual(index: number): void {
    this.rituals.set(this.rituals().filter((_, i) => i !== index));
    this.clearErrors('excluded');
  }

  /** What is saved: the ticked DSU days and every ritual (a "week N" only when it is of that kind). */
  private excludedInput(): ExcludedSchedule {
    return {
      dsu: this.dsuRows()
        .filter((d) => d.on)
        .map((d) => ({ weekday: d.weekday, start: d.start, minutes: clampMinutes(d.minutes) })),
      rituals: this.rituals().map((r) => ({
        name: r.name.trim(),
        enabled: r.enabled,
        weekday: r.weekday,
        start: r.start,
        minutes: clampMinutes(r.minutes),
        frequency: r.frequency,
        sprintWeek: r.frequency === 'sprint_week' ? r.sprintWeek : 0,
      })),
    };
  }

  private clearErrors(...fields: TrackingRuleField[]): void {
    const errors = { ...this.fieldErrors() };
    for (const f of fields) delete errors[f];
    this.fieldErrors.set(errors);
  }

  protected save(): void {
    const l = this.lookup();
    if (!l || !this.canSave()) return;
    const key = l.projectKey;
    const input: TrackingRuleInput = {
      trackedStatuses: this.tracked(),
      doneStatus: this.done(),
      reviewStatus: this.review() || null,
      storyPointFields: this.spFields(),
      storyPointWriteField: this.writeField(),
      storyPointWriteFieldSubtask: this.writeFieldSubtask(),
      createFields: this.createFieldsInput(),
      excluded: this.excludedInput(),
    };
    this.saving.set(true);
    this.existingKey.set(null);
    const request = this.isEdit ? this.api.update(key, input) : this.api.create({ ...input, projectKey: key });
    request.subscribe({
      next: (saved) => {
        this.saving.set(false);
        this.toast.show(
          'success',
          'Aturan board tersimpan',
          saved?.recalculating
            ? `${key} berlaku mulai cycle poller berikutnya. Waktu seluruh sprint ${key} sedang dihitung ulang dari Jira di latar belakang.`
            : `${key} berlaku mulai cycle poller berikutnya. Waktu yang sudah tercatat bisa dihitung ulang dari Jira.`,
          10000,
          recalculateAction(this.api, this.toast),
        );
        this.router.navigate(['/admin/tracking-rules']);
      },
      error: (err: HttpErrorResponse) => {
        this.saving.set(false);
        const body = err?.error as TrackingRuleError | undefined;
        if (err?.status === 400 && body?.fields) {
          this.fieldErrors.set(body.fields);
          return;
        }
        if (err?.status === 409) {
          this.existingKey.set(key);
          return;
        }
        this.toast.error('Gagal menyimpan aturan', body?.message ?? 'Coba lagi sebentar.');
      },
    });
  }
}
