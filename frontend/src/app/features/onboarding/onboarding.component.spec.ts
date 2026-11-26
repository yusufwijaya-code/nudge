import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';
import { OnboardingBoard, OnboardingForm } from '../../core/services/onboarding.service';
import { OnboardingComponent } from './onboarding.component';

const CORE: OnboardingBoard = { boardId: 26, name: 'ESB Core Board', projectKey: 'CORE', sprintName: 'Core Sprint 24', ticketCount: 4, registered: true };
const NEW1: OnboardingBoard = { boardId: 40, name: 'New Board', projectKey: 'NEW', sprintName: 'New Sprint 1', ticketCount: 2, registered: false };
const NEW2: OnboardingBoard = { boardId: 41, name: 'Other Board', projectKey: 'OTH', sprintName: 'Other Sprint 3', ticketCount: 1, registered: false };

const formOf = (board: OnboardingBoard, over: Partial<OnboardingForm['defaults']> = {}): OnboardingForm => ({
  board, projectName: 'Proj ' + board.projectKey, templateProject: 'CORE',
  statuses: ['To Do', 'Code Process', 'Code Review', 'Code Completed'],
  numberFields: [{ id: 'customfield_10016', name: 'Story Points' }, { id: 'customfield_10020', name: 'Story Point Estimate' }],
  defaults: { trackedStatuses: ['Code Process'], doneStatus: 'Code Completed', reviewStatus: '', storyPointFields: ['customfield_10016'], storyPointWriteField: 'customfield_10016', storyPointWriteFieldSubtask: 'customfield_10016', ...over },
});

describe('OnboardingComponent', () => {
  let http: HttpTestingController;
  let router: Router;
  let fixture: ComponentFixture<OnboardingComponent>;
  const q = (s: string) => fixture.nativeElement.querySelector(s) as HTMLElement | null;
  const qa = (s: string) => Array.from(fixture.nativeElement.querySelectorAll(s)) as HTMLElement[];

  function render(boards: OnboardingBoard[], query: Record<string, string> = {}) {
    TestBed.configureTestingModule({
      imports: [OnboardingComponent],
      providers: [
        provideHttpClient(), provideHttpClientTesting(), provideRouter([]),
        { provide: ActivatedRoute, useValue: { snapshot: { queryParamMap: convertToParamMap(query) } } },
      ],
    });
    http = TestBed.inject(HttpTestingController);
    router = TestBed.inject(Router);
    spyOn(router, 'navigateByUrl').and.resolveTo(true);
    spyOn(router, 'navigate').and.resolveTo(true);
    fixture = TestBed.createComponent(OnboardingComponent);
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/onboarding/boards') && r.params.get('fresh') === '1').flush({ boards });
    fixture.detectChanges();
  }
  const flushForm = (board: OnboardingBoard, form = formOf(board)) => {
    http.expectOne((r) => r.url.endsWith(`/onboarding/boards/${board.boardId}/form`)).flush(form);
    fixture.detectChanges();
  };

  it('prefills the form from the suggestion and saves it with one click', async () => {
    render([CORE, NEW1]);
    flushForm(NEW1);

    expect(q('[data-template]')!.textContent).toContain('CORE');
    expect(q('[data-registered-note]')!.textContent).toContain('1 board');
    expect(q('[data-status="Code Process"]')!.classList).toContain('on');
    expect(q('[data-status="Code Review"]')!.classList).not.toContain('on');
    expect(q('[data-field="customfield_10016"]')!.classList).toContain('on');
    await fixture.whenStable(); fixture.detectChanges();
    expect((q('[data-save]') as HTMLButtonElement).disabled).toBeFalse();

    (q('[data-save]') as HTMLButtonElement).click();
    fixture.detectChanges();
    expect(q('[data-syncing]')!.textContent).toContain('menyinkronkan');
    expect((q('[data-save]') as HTMLButtonElement).disabled).toBeTrue();
    const req = http.expectOne((r) => r.method === 'POST' && r.url.endsWith('/onboarding/boards/40/setup'));
    expect(req.request.body).toEqual({
      trackedStatuses: ['Code Process'], doneStatus: 'Code Completed', reviewStatus: null,
      storyPointFields: ['customfield_10016'], storyPointWriteField: 'customfield_10016', storyPointWriteFieldSubtask: 'customfield_10016',
    });
    req.flush({ projectKey: 'NEW', synced: true, added: 2, updated: 0 });
    expect(router.navigate).toHaveBeenCalledWith(['/dashboard'], { queryParams: { board: 40 } });
  });

  it('cannot save until statuses and Story Point fields are complete', async () => {
    render([NEW1]);
    flushForm(NEW1, formOf(NEW1, { trackedStatuses: [], doneStatus: '', storyPointFields: [], storyPointWriteField: '', storyPointWriteFieldSubtask: '' }));
    expect((q('[data-save]') as HTMLButtonElement).disabled).toBeTrue();
    expect(q('[data-why]')).not.toBeNull();

    (q('[data-status="Code Process"]') as HTMLButtonElement).click();
    (q('[data-field="customfield_10016"]') as HTMLButtonElement).click();
    fixture.detectChanges();
    // Picking the first SP field also fills both write fields; the done status is still missing.
    const c = fixture.componentInstance as any;
    expect(c.writeField()).toBe('customfield_10016');
    expect(c.writeFieldSubtask()).toBe('customfield_10016');
    expect((q('[data-save]') as HTMLButtonElement).disabled).toBeTrue();
  });

  it('a tracked status cannot be the done status, and dropping a status clears the review pick', () => {
    render([NEW1]);
    flushForm(NEW1, formOf(NEW1, { trackedStatuses: ['Code Process', 'Code Review'], reviewStatus: 'Code Review' }));
    const c = fixture.componentInstance as any;
    c.toggleTracked('Code Completed');
    expect(c.done()).toBe('');
    c.toggleTracked('Code Review');
    expect(c.review()).toBe('');
  });

  it('shows the server field errors and the message when saving is refused', async () => {
    render([NEW1]);
    flushForm(NEW1);
    await fixture.whenStable(); fixture.detectChanges();
    (q('[data-save]') as HTMLButtonElement).click();
    http.expectOne((r) => r.method === 'POST').flush(
      { error: 'validation_failed', message: 'Periksa isian yang ditandai', fields: { doneStatus: 'Status selesai wajib dipilih' } },
      { status: 400, statusText: 'Bad Request' },
    );
    fixture.detectChanges();
    expect(q('[data-save-error]')!.textContent).toContain('Periksa isian');
    expect(fixture.nativeElement.textContent).toContain('Status selesai wajib dipilih');
    expect((q('[data-save]') as HTMLButtonElement).disabled).toBeFalse();
    expect(router.navigate).not.toHaveBeenCalled();
  });

  it('moves on to the next new board after saving one, then to the dashboard', async () => {
    render([NEW1, NEW2], { board: '41' });
    flushForm(NEW2);
    expect(qa('[data-board-tabs] [data-board]').length).toBe(2);
    expect(q('[data-board="41"]')!.classList).toContain('on');
    await fixture.whenStable(); fixture.detectChanges();

    (q('[data-save]') as HTMLButtonElement).click();
    http.expectOne((r) => r.method === 'POST' && r.url.endsWith('/boards/41/setup')).flush({ projectKey: 'OTH', synced: true, added: 0, updated: 0 });
    fixture.detectChanges();
    expect(router.navigate).not.toHaveBeenCalled();
    flushForm(NEW1);
    expect(q('[data-project]')!.textContent).toContain('New Board');
  });

  it('carries on when a teammate registered the board first (409)', async () => {
    render([NEW1]);
    flushForm(NEW1);
    await fixture.whenStable(); fixture.detectChanges();
    (q('[data-save]') as HTMLButtonElement).click();
    http.expectOne((r) => r.method === 'POST').flush({ error: 'already_registered' }, { status: 409, statusText: 'Conflict' });
    expect(router.navigate).toHaveBeenCalledWith(['/dashboard'], { queryParams: { board: 40 } });
  });

  it('goes straight to the dashboard when every board is registered', () => {
    render([CORE]);
    expect(router.navigateByUrl).toHaveBeenCalledWith('/dashboard');
  });

  it('shows an error when the boards cannot be read', () => {
    TestBed.configureTestingModule({
      imports: [OnboardingComponent],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([])],
    });
    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(OnboardingComponent);
    fixture.detectChanges();
    http.expectOne((r) => r.url.endsWith('/onboarding/boards')).flush({}, { status: 502, statusText: 'Bad Gateway' });
    fixture.detectChanges();
    expect(q('[data-load-error]')).not.toBeNull();
  });
});
