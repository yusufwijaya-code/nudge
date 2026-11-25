import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';
import { Subject, of, throwError } from 'rxjs';
import { TrackingRuleFormComponent } from './tracking-rule-form.component';
import { CreateFieldType, JiraProjectLookup, TrackingRule, TrackingRuleService } from '../../../core/services/tracking-rule.service';
import { clampMinutes, slotEnd } from './tracking-rule-form.component';
import { ToastService } from '../../../core/ui/toast.service';

const POS: JiraProjectLookup = {
  projectKey: 'POS', projectName: 'ESB POS', avatarUrl: '',
  statuses: ['To Do', 'In Progress', 'Code Review', 'Done'],
  numberFields: [
    { id: 'customfield_10016', name: 'Story point estimate', storyPointLike: true },
    { id: 'customfield_200', name: 'Effort', storyPointLike: false },
  ],
};

const CORE_RULE: TrackingRule = {
  projectKey: 'CORE', projectName: 'ESB Core', trackedStatuses: ['Code Process', 'CODE REVIEW'], doneStatus: 'Code Completed', reviewStatus: 'CODE REVIEW',
  storyPointFields: [{ id: 'customfield_10125', name: 'Story Point Estimate' }, { id: 'customfield_11053', name: 'Story Points' }],
  storyPointWriteField: 'customfield_10125', storyPointWriteFieldSubtask: 'customfield_11053', isActive: true, editedBy: 'seed', editedDate: '2026-11-16T09:00:00+07:00',
};

const OMS_TYPES: CreateFieldType[] = [
  { typeId: '10143', typeName: 'SUB Story', subtask: true, fields: [{ id: 'duedate', name: 'Due date', kind: 'date', required: false }] },
  { typeId: '10206', typeName: 'Bug', subtask: false, fields: [
    { id: 'customfield_10286', name: 'Service', kind: 'cascade', required: true },
    { id: 'customfield_10016', name: 'Story point estimate', kind: 'number', required: false },
    { id: 'duedate', name: 'Due date', kind: 'date', required: false },
    { id: 'priority', name: 'Priority', kind: 'select', required: false },
  ] },
];

describe('TrackingRuleFormComponent', () => {
  let api: jasmine.SpyObj<TrackingRuleService>;
  let toast: jasmine.SpyObj<ToastService>;

  function setup(key: string | null): void {
    api = jasmine.createSpyObj('TrackingRuleService', ['list', 'lookup', 'create', 'update', 'setActive', 'recalculate', 'createFields']);
    api.createFields.and.returnValue(of({ types: [] }));
    toast = jasmine.createSpyObj('ToastService', ['show', 'success', 'error', 'info']);
    TestBed.configureTestingModule({
      imports: [TrackingRuleFormComponent],
      providers: [
        provideRouter([]),
        { provide: ActivatedRoute, useValue: { snapshot: { paramMap: convertToParamMap(key ? { key } : {}) } } },
        { provide: TrackingRuleService, useValue: api },
        { provide: ToastService, useValue: toast },
      ],
    });
  }

  function create() {
    const fixture = TestBed.createComponent(TrackingRuleFormComponent);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, c: fixture.componentInstance as any };
  }

  it('looks the project up only for a valid key and normalizes it (AC3.1, AC3.8)', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    const { fixture, el, c } = create();
    const button = () => el.querySelector('[data-lookup]') as HTMLButtonElement;
    c.onKeyInput('x');
    fixture.detectChanges();
    expect(button().disabled).toBeTrue();
    c.onKeyInput(' pos ');
    fixture.detectChanges();
    button().click();
    fixture.detectChanges();
    expect(api.lookup).toHaveBeenCalledWith('POS', false);
    expect(el.querySelector('[data-project]')?.textContent).toContain('ESB POS');
    expect(el.querySelectorAll('[data-status]').length).toBe(4);
  });

  it('shows a loading skeleton while the Jira lookup runs', () => {
    setup(null);
    const pending = new Subject<JiraProjectLookup>();
    api.lookup.and.returnValue(pending);
    const { fixture, el, c } = create();
    c.onKeyInput('POS');
    c.runLookup();
    fixture.detectChanges();
    expect(el.querySelector('[data-lookup-loading]')).not.toBeNull();
    expect((el.querySelector('[data-lookup]') as HTMLButtonElement).classList).toContain('loading');
    pending.next(POS);
    pending.complete();
    fixture.detectChanges();
    expect(el.querySelector('[data-lookup-loading]')).toBeNull();
    expect(el.querySelector('[data-project]')).not.toBeNull();
  });

  it('shows the not-found message from Jira (AC3.5)', () => {
    setup(null);
    api.lookup.and.returnValue(throwError(() => ({ status: 404 })));
    const { fixture, el, c } = create();
    c.onKeyInput('NOPE');
    c.runLookup();
    fixture.detectChanges();
    expect(el.querySelector('[data-lookup-error]')?.textContent).toContain('Project NOPE tidak ditemukan di Jira');
  });

  it('keeps done out of the tracked statuses and review inside them (AC3.2)', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    const { c } = create();
    c.onKeyInput('POS');
    c.runLookup();
    c.toggleTracked('In Progress');
    c.toggleTracked('Code Review');
    c.setDone('Done');
    c.setReview('Code Review');
    expect(c.doneOptions().map((o: { value: string }) => o.value)).toEqual(['', 'To Do', 'Done']);
    c.toggleTracked('Code Review');
    expect(c.review()).toBe('');
    c.toggleTracked('Done');
    expect(c.done()).toBe('');
  });

  it('orders Story Point fields and resets a write field whose field is removed', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    const { c } = create();
    c.onKeyInput('POS');
    c.runLookup();
    c.addField('customfield_200');
    c.addField('customfield_10016');
    expect(c.writeField()).toBe('customfield_200'); // default = the first field
    c.moveField(1, -1);
    expect(c.spFields()).toEqual(['customfield_10016', 'customfield_200']);
    c.removeField('customfield_200');
    expect(c.writeField()).toBe('');
    expect(c.writeFieldSubtask()).toBe('');
  });

  it('previews the rule as a sentence and saves it with the recalculation toast (AC3.3, AC3.4)', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    api.create.and.returnValue(of({ ...CORE_RULE, projectKey: 'POS' }));
    const { fixture, el, c } = create();
    const router = TestBed.inject(Router);
    spyOn(router, 'navigate').and.resolveTo(true);
    c.onKeyInput('POS');
    c.runLookup();
    c.toggleTracked('In Progress');
    c.setDone('Done');
    c.addField('customfield_10016');
    fixture.detectChanges();

    expect(el.querySelector('[data-preview]')?.textContent?.trim()).toBe(
      'Budget berjalan selama In Progress; selesai saat Done. Review tidak dihitung. ' +
        'SP dibaca dari Story point estimate (customfield_10016); ' +
        'ditulis ke Story point estimate (customfield_10016) (card biasa) / Story point estimate (customfield_10016) (subtask).',
    );
    (el.querySelector('[data-save]') as HTMLButtonElement).click();
    expect(api.create).toHaveBeenCalledWith({
      projectKey: 'POS', trackedStatuses: ['In Progress'], doneStatus: 'Done', reviewStatus: null,
      storyPointFields: ['customfield_10016'], storyPointWriteField: 'customfield_10016', storyPointWriteFieldSubtask: 'customfield_10016',
      createFields: {}, excluded: { dsu: [], rituals: [] },
    });
    expect(toast.show.calls.mostRecent().args[4]!.label).toBe('Hitung ulang semua user');
    expect(router.navigate).toHaveBeenCalledWith(['/admin/tracking-rules']);
  });

  it('links to Edit on 409 and shows server field errors (AC3.6, AC3.7)', () => {
    setup(null);
    api.lookup.and.returnValue(of(POS));
    const { fixture, el, c } = create();
    c.onKeyInput('POS');
    c.runLookup();
    c.toggleTracked('In Progress');
    c.setDone('Done');
    c.addField('customfield_10016');

    api.create.and.returnValue(throwError(() => ({ status: 409, error: { error: 'rule_exists' } })));
    c.save();
    fixture.detectChanges();
    expect(el.querySelector('[data-exists] a')?.getAttribute('href')).toBe('/admin/tracking-rules/POS');

    api.create.and.returnValue(throwError(() => ({ status: 400, error: { error: 'validation_failed', fields: { doneStatus: 'Status selesai tidak boleh termasuk status yang dihitung' } } })));
    c.save();
    fixture.detectChanges();
    expect(el.textContent).toContain('Status selesai tidak boleh termasuk status yang dihitung');
  });

  it('loads a stored rule on Edit and blocks saving while a stored status is gone from Jira (AC4.1, AC4.4)', () => {
    setup('CORE');
    api.list.and.returnValue(of({ rules: [CORE_RULE] }));
    api.lookup.and.returnValue(of({
      projectKey: 'CORE', projectName: 'ESB Core', avatarUrl: '',
      statuses: ['To Do', 'Code Process', 'CODE REVIEW', 'Done'],
      numberFields: [
        { id: 'customfield_10125', name: 'Story Point Estimate', storyPointLike: true },
        { id: 'customfield_11053', name: 'Story Points', storyPointLike: true },
      ],
    }));
    api.update.and.returnValue(of(CORE_RULE));
    const { fixture, el, c } = create();

    expect((el.querySelector('[data-key-input]') as HTMLInputElement).readOnly).toBeTrue();
    expect(api.lookup).toHaveBeenCalledWith('CORE', true);
    expect(el.querySelector('[data-status="Code Process"]')?.classList).toContain('on');
    expect(el.querySelector('[data-stale-statuses]')?.textContent).toContain('Code Completed');
    expect((el.querySelector('[data-save]') as HTMLButtonElement).disabled).toBeTrue();

    c.dropStaleStatus('Code Completed');
    c.setDone('Done');
    fixture.detectChanges();
    expect(el.querySelector('[data-stale-statuses]')).toBeNull();
    (el.querySelector('[data-save]') as HTMLButtonElement).click();
    expect(api.update).toHaveBeenCalledWith('CORE', jasmine.objectContaining({ doneStatus: 'Done', reviewStatus: 'CODE REVIEW' }));

    // "Muat ulang dari Jira" bypasses the server's lookup cache too.
    (el.querySelector('[data-lookup]') as HTMLButtonElement).click();
    expect(api.lookup.calls.count()).toBe(2);
    expect(api.lookup.calls.mostRecent().args).toEqual(['CORE', true]);
  });

  describe('Field form Buat Card', () => {
    function omsForm(rule?: Partial<TrackingRule>) {
      setup(rule ? 'OMS' : null);
      api.lookup.and.returnValue(of({ ...POS, projectKey: 'OMS', projectName: 'ESB OMS' }));
      api.createFields.and.returnValue(of({ types: OMS_TYPES }));
      if (rule) api.list.and.returnValue(of({ rules: [{ ...CORE_RULE, projectKey: 'OMS', ...rule }] }));
      const r = create();
      if (!rule) { r.c.onKeyInput('OMS'); r.c.runLookup(); r.fixture.detectChanges(); }
      return r;
    }

    it('lists issue types (non-subtask first selected) with required fields locked and SP fields hidden', () => {
      const { fixture, el, c } = omsForm();
      c.addField('customfield_10016');
      fixture.detectChanges();
      expect(api.createFields).toHaveBeenCalledWith('OMS', false);
      expect(el.querySelector('[data-cf-type="10143"]')?.textContent).toContain('subtask');
      expect(el.querySelector('[data-cf-type="10206"]')?.classList).toContain('on');
      const service = el.querySelector('[data-cf-field="customfield_10286"]')!;
      expect(service.textContent).toContain('Wajib di Jira');
      expect((service.querySelector('input') as HTMLInputElement).checked).toBeTrue();
      expect((service.querySelector('input') as HTMLInputElement).disabled).toBeTrue();
      expect(el.querySelector('[data-cf-field="customfield_10016"]')).withContext('the SP field has its own input').toBeNull();
      expect((el.querySelector('[data-cf-field="duedate"] input') as HTMLInputElement).checked).toBeFalse();
    });

    it('toggles optional fields per type, counts them and saves them with the rule', () => {
      const { fixture, el, c } = omsForm();
      api.create.and.returnValue(of({ ...CORE_RULE, projectKey: 'OMS' }));
      spyOn(TestBed.inject(Router), 'navigate').and.resolveTo(true);
      const check = (id: string) => (el.querySelector(`[data-cf-field="${id}"] input`) as HTMLInputElement);
      check('duedate').click(); check('priority').click(); fixture.detectChanges();
      check('priority').click(); fixture.detectChanges();
      expect(el.querySelector('[data-cf-type="10206"] .trf-cf-count')?.textContent?.trim()).toBe('1');
      (el.querySelector('[data-cf-type="10143"]') as HTMLButtonElement).click(); fixture.detectChanges();
      check('duedate').click(); fixture.detectChanges();
      expect(el.querySelector('[data-cf-total]')?.textContent).toContain('2 field tambahan');
      c.toggleTracked('In Progress'); c.setDone('Done'); c.addField('customfield_10016');
      fixture.detectChanges();
      (el.querySelector('[data-save]') as HTMLButtonElement).click();
      expect(api.create.calls.mostRecent().args[0].createFields).toEqual({ '10206': ['duedate'], '10143': ['duedate'] });
    });

    it('loads stored choices on Edit and drops ones Jira no longer offers when saving', () => {
      const { fixture, el, c } = omsForm({ createFields: { '10206': ['priority', 'customfield_gone', 'customfield_10286'], '999': ['duedate'] } });
      api.update.and.returnValue(of(CORE_RULE));
      expect(api.createFields).toHaveBeenCalledWith('OMS', true);
      expect((el.querySelector('[data-cf-field="priority"] input') as HTMLInputElement).checked).toBeTrue();
      c.dropStaleStatus('Code Completed'); c.dropStaleStatus('Code Process'); c.setDone('Done');
      c.spFields.set(['customfield_10016']); c.writeField.set('customfield_10016'); c.writeFieldSubtask.set('customfield_10016');
      fixture.detectChanges();
      (el.querySelector('[data-save]') as HTMLButtonElement).click();
      expect(api.update.calls.mostRecent().args[1].createFields).toEqual({ '10206': ['priority'] });
    });

    it('shows a skeleton while loading and a retry on error, without blocking the rest of the form', () => {
      setup(null);
      api.lookup.and.returnValue(of(POS));
      const pending = new Subject<{ types: CreateFieldType[] }>();
      api.createFields.and.returnValue(pending);
      const { fixture, el, c } = create();
      c.onKeyInput('POS'); c.runLookup(); fixture.detectChanges();
      expect(el.querySelector('[data-cf-loading]')).not.toBeNull();
      pending.error({ status: 502 });
      fixture.detectChanges();
      expect(el.querySelector('[data-cf-error]')?.textContent).toContain('Gagal memuat field Jira');
      expect(el.querySelector('[data-section="statuses"]')).not.toBeNull();
      api.createFields.and.returnValue(of({ types: OMS_TYPES }));
      (el.querySelector('[data-cf-retry]') as HTMLButtonElement).click(); fixture.detectChanges();
      expect(el.querySelector('[data-cf-error]')).toBeNull();
      expect(el.querySelectorAll('[data-cf-type]').length).toBe(2);
    });
  });

  describe('Jadwal yang tidak dihitung', () => {
    function posForm() {
      setup(null);
      api.lookup.and.returnValue(of(POS));
      api.create.and.returnValue(of({ ...CORE_RULE, projectKey: 'POS', recalculating: true }));
      const r = create();
      spyOn(TestBed.inject(Router), 'navigate').and.resolveTo(true);
      r.c.onKeyInput('POS');
      r.c.runLookup();
      r.c.toggleTracked('In Progress');
      r.c.setDone('Done');
      r.c.addField('customfield_10016');
      r.fixture.detectChanges();
      return r;
    }

    it('shows the five weekdays with no DSU ticked and no ritual', () => {
      const { el } = posForm();
      expect(el.querySelectorAll('[data-dsu-day]').length).toBe(5);
      expect(el.querySelectorAll('[data-dsu-day].on').length).toBe(0);
      expect(el.querySelector('[data-no-rituals]')).not.toBeNull();
    });

    it('saves the ticked DSU days and the rituals with the rule', () => {
      const { fixture, el, c } = posForm();
      c.setDsu(1, { on: true });
      c.setDsu(2, { on: true, start: '09:00', minutes: 15 });
      c.addRitual();
      c.setRitual(0, { name: ' Sprint Planning / Grooming ', weekday: 1, start: '11:00', minutes: 390, frequency: 'sprint_week', sprintWeek: 2 });
      c.addRitual();
      c.setRitual(1, { name: 'Retro', enabled: false, weekday: 5, start: '15:00', minutes: 60 });
      fixture.detectChanges();
      (el.querySelector('[data-save]') as HTMLButtonElement).click();

      expect(api.create.calls.mostRecent().args[0].excluded).toEqual({
        dsu: [{ weekday: 2, start: '08:45', minutes: 30 }, { weekday: 3, start: '09:00', minutes: 15 }],
        rituals: [
          { name: 'Sprint Planning / Grooming', enabled: true, weekday: 1, start: '11:00', minutes: 390, frequency: 'sprint_week', sprintWeek: 2 },
          { name: 'Retro', enabled: false, weekday: 5, start: '15:00', minutes: 60, frequency: 'weekly', sprintWeek: 0 },
        ],
      });
    });

    it('shows what a ritual cuts, in words, and when it applies', () => {
      const { fixture, el, c } = posForm();
      c.addRitual();
      c.setRitual(0, { name: 'Sprint Planning / Grooming', start: '11:00', minutes: 390, frequency: 'sprint_week', sprintWeek: 2 });
      fixture.detectChanges();
      const cut = el.querySelector('[data-cut]')!.textContent!.replace(/\s+/g, ' ');
      expect(cut).toContain('11:00 – 17:30');
      expect(cut).toContain('6 jam 30 menit');
      expect(cut).toContain('Berlaku hanya pada minggu ke-2 sprint');
      c.setRitual(0, { frequency: 'weekly' });
      fixture.detectChanges();
      expect(el.querySelector('[data-cut]')!.textContent).toContain('Berlaku setiap minggu');
    });

    it('will not save a ritual without a name', () => {
      const { fixture, el, c } = posForm();
      c.addRitual();
      fixture.detectChanges();
      expect((el.querySelector('[data-save]') as HTMLButtonElement).disabled).toBeTrue();
      c.setRitual(0, { name: 'Grooming' });
      fixture.detectChanges();
      expect((el.querySelector('[data-save]') as HTMLButtonElement).disabled).toBeFalse();
    });

    it('adds and removes rituals, up to ten', () => {
      const { fixture, el, c } = posForm();
      for (let i = 0; i < 12; i++) c.addRitual();
      fixture.detectChanges();
      expect(c.rituals().length).toBe(10);
      expect((el.querySelector('[data-add-ritual]') as HTMLButtonElement).disabled).toBeTrue();
      (el.querySelector('[data-remove-ritual]') as HTMLButtonElement).click();
      fixture.detectChanges();
      expect(c.rituals().length).toBe(9);
    });

    it('loads the stored schedule on Edit and shows the server message when it is refused', () => {
      setup('OMS');
      // Jira still has everything the stored rule uses, so saving is allowed.
      api.lookup.and.returnValue(of({
        projectKey: 'OMS', projectName: 'ESB OMS', avatarUrl: '',
        statuses: ['Code Process', 'CODE REVIEW', 'Code Completed'],
        numberFields: [
          { id: 'customfield_10125', name: 'Story Point Estimate', storyPointLike: true },
          { id: 'customfield_11053', name: 'Story Points', storyPointLike: true },
        ],
      }));
      api.list.and.returnValue(of({ rules: [{ ...CORE_RULE, projectKey: 'OMS', excluded: {
        dsu: [{ weekday: 3, start: '08:45', minutes: 30 }],
        rituals: [{ name: 'Grooming', enabled: true, weekday: 1, start: '11:00', minutes: 60, frequency: 'weekly', sprintWeek: 0 }],
      } }] }));
      const { fixture, el, c } = create();
      fixture.detectChanges();
      expect(el.querySelector('[data-dsu-day="3"]')?.classList).toContain('on');
      expect(el.querySelector('[data-dsu-day="2"]')?.classList).not.toContain('on');
      expect(c.rituals()[0].name).toBe('Grooming');

      api.update.and.returnValue(throwError(() => ({ status: 400, error: { error: 'validation_failed', fields: { excluded: 'Daily Stand Up hanya satu per hari' } } })));
      (el.querySelector('[data-save]') as HTMLButtonElement).click();
      fixture.detectChanges();
      expect(el.querySelector('[data-excluded-error]')?.textContent).toContain('hanya satu per hari');
    });

    it('says in the toast that the sprints are being recalculated', () => {
      const { el } = posForm();
      (el.querySelector('[data-save]') as HTMLButtonElement).click();
      expect(toast.show.calls.mostRecent().args[2]).toContain('seluruh sprint POS sedang dihitung ulang dari Jira');
    });
  });

  describe('slot helpers', () => {
    it('ends a slot at start + minutes, capped at midnight', () => {
      expect(slotEnd('11:00', 390)).toBe('17:30');
      expect(slotEnd('08:45', 30)).toBe('09:15');
      expect(slotEnd('23:30', 120)).toBe('24:00');
    });
    it('keeps a typed length inside 1..540', () => {
      expect(clampMinutes(0)).toBe(1);
      expect(clampMinutes('abc')).toBe(1);
      expect(clampMinutes(30.4)).toBe(30);
      expect(clampMinutes(9999)).toBe(540);
    });
  });
});
