import { signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { BehaviorSubject, of, throwError } from 'rxjs';
import { AdminUsersComponent } from './admin-users.component';
import { AdminService, ListUsersResponse, UserRow } from '../../../core/services/admin.service';
import { AuthService, CurrentUser } from '../../../core/services/auth.service';
import { ToastService } from '../../../core/ui/toast.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ChatAppService } from '../../../core/services/chat-app.service';
import { ActivatedRoute, Router, convertToParamMap, provideRouter } from '@angular/router';

describe('AdminUsersComponent', () => {
  let fixture: ComponentFixture<AdminUsersComponent>;
  let adminService: jasmine.SpyObj<AdminService>;
  let confirmService: jasmine.SpyObj<ConfirmService>;
  let me: BehaviorSubject<CurrentUser | null>;

  beforeEach(async () => {
    adminService = jasmine.createSpyObj('AdminService', ['listUsers', 'toggleActive', 'editJiraAccountId', 'deleteCredential', 'deleteUser', 'setAdmin']);
    confirmService = jasmine.createSpyObj('ConfirmService', ['ask']);
    me = new BehaviorSubject<CurrentUser | null>({ email: 'yusuf.wijaya@esb.co.id', isAdmin: true, isOwner: true });

    await TestBed.configureTestingModule({
      imports: [AdminUsersComponent],
      providers: [
        provideRouter([]),
        { provide: ActivatedRoute, useValue: { snapshot: { queryParamMap: convertToParamMap({}) } } },
        { provide: AdminService, useValue: adminService },
        { provide: ConfirmService, useValue: confirmService },
        { provide: AuthService, useValue: { currentUser$: me } },
        { provide: ToastService, useValue: jasmine.createSpyObj('ToastService', ['success', 'error', 'info', 'show']) },
        { provide: ChatAppService, useValue: { status: signal({ operatorName: 'Yusuf Wijaya' }), load: () => of({}) } },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(AdminUsersComponent);
  });

  it('renders one row per user with status badge and Jira Account ID', () => {
    const response: ListUsersResponse = {
      users: [
        { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '2026-01-01', jiraAccountId: 'acc-1', hasCredential: true, credentialMasked: '••••ab3f', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false },
        { userId: 2, email: 'b@esb.co.id', userName: 'B', flagActive: false, createdDate: '2026-01-02', jiraAccountId: '', hasCredential: false, credentialMasked: '', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false },
      ],
    };
    adminService.listUsers.and.returnValue(of(response));
    fixture.detectChanges();

    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    expect(rows.length).toBe(2);
    expect(rows[0].textContent).toContain('a@esb.co.id');
    expect(rows[0].querySelector('.status-badge').textContent).toContain('Aktif');
    expect(rows[1].querySelector('.status-badge').textContent).toContain('Nonaktif');
    expect(rows[1].textContent).toContain('—');
  });

  it('filters to active users that have not activated Nudge App', () => {
    const base = { userName: '', createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '', chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false };
    adminService.listUsers.and.returnValue(of({
      users: [
        { ...base, userId: 1, email: 'a@esb.co.id', flagActive: true, chatAppActivated: true },
        { ...base, userId: 2, email: 'b@esb.co.id', flagActive: true, chatAppActivated: false },
        { ...base, userId: 3, email: 'c@esb.co.id', flagActive: false, chatAppActivated: false },
      ],
    }));
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelectorAll('tbody tr').length).toBe(3);

    const tab = fixture.nativeElement.querySelector('[data-filter-chat-app]') as HTMLButtonElement;
    expect(tab.textContent).toContain('1'); // one active user without the Nudge App
    tab.click();
    fixture.detectChanges();

    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    expect(rows.length).toBe(1);
    expect(rows[0].textContent).toContain('b@esb.co.id');
  });

  it('shows an empty table with no rows when there are no users', () => {
    adminService.listUsers.and.returnValue(of({ users: [] }));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelectorAll('tbody tr').length).toBe(0);
  });

  it('shows the access-denied message on a 403 response', () => {
    adminService.listUsers.and.returnValue(throwError(() => ({ status: 403 })));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Kamu tidak punya akses ke halaman ini');
  });

  it('shows a generic error message on a 500 response', () => {
    adminService.listUsers.and.returnValue(throwError(() => ({ status: 500 })));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Gagal memuat daftar user, coba lagi');
  });

  it('toggles active status and reloads the list on success', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    adminService.toggleActive.and.returnValue(of({}));

    fixture.nativeElement.querySelector('.actions button').click();

    expect(adminService.toggleActive).toHaveBeenCalledWith(1, false);
    expect(adminService.listUsers).toHaveBeenCalledTimes(2);
  });

  it('shows a row-scoped error message when toggling active status fails', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    adminService.toggleActive.and.returnValue(throwError(() => ({ error: { error: 'Tidak bisa menonaktifkan diri sendiri' } })));

    fixture.nativeElement.querySelector('.actions button').click();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.actions .msg.error').textContent).toContain('Tidak bisa menonaktifkan diri sendiri');
  });

  it('saves the Jira Account ID optimistically, without reloading the list', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: 'old-id', hasCredential: false, credentialMasked: '', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    adminService.editJiraAccountId.and.returnValue(of({}));

    fixture.componentInstance['jiraAccountIdDrafts'][1] = 'new-id';
    fixture.componentInstance['onSaveJiraAccountId'](user);

    expect(adminService.editJiraAccountId).toHaveBeenCalledWith(1, 'new-id');
    expect(adminService.listUsers).toHaveBeenCalledTimes(1);
    expect(user.jiraAccountId).toBe('new-id');
  });

  it('only shows the remove-credential icon button when the user has a credential', () => {
    const withCred = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: true, credentialMasked: '••••ab3f', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false };
    const withoutCred = { userId: 2, email: 'b@esb.co.id', userName: 'B', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false };
    adminService.listUsers.and.returnValue(of({ users: [withCred, withoutCred] }));
    fixture.detectChanges();

    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    const button = rows[0].querySelector('[data-delete-credential]') as HTMLButtonElement;
    expect(button).not.toBeNull();
    expect(button.textContent!.trim()).toBe(''); // an icon, no text
    expect(button.getAttribute('aria-label')).toBe('Hapus credential');
    expect(rows[1].querySelector('[data-delete-credential]')).toBeNull();
  });

  it('asks for confirmation before deleting a credential, and does nothing if cancelled', async () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: true, credentialMasked: '••••ab3f', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    confirmService.ask.and.resolveTo(false);

    await fixture.componentInstance['onDeleteCredential'](user);

    expect(adminService.deleteCredential).not.toHaveBeenCalled();
  });

  it('deletes the credential and reloads the list on confirmation', async () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: true, credentialMasked: '••••ab3f', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    confirmService.ask.and.resolveTo(true);
    adminService.deleteCredential.and.returnValue(of({}));

    await fixture.componentInstance['onDeleteCredential'](user);

    expect(adminService.deleteCredential).toHaveBeenCalledWith(1);
    expect(adminService.listUsers).toHaveBeenCalledTimes(2);
  });

  it('shows the self-lockout error message when deleting a user fails', async () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    confirmService.ask.and.resolveTo(true);
    adminService.deleteUser.and.returnValue(throwError(() => ({ error: { error: 'Tidak bisa menghapus diri sendiri' } })));

    await fixture.componentInstance['onDeleteUser'](user);
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.actions .msg.error').textContent).toContain('Tidak bisa menghapus diri sendiri');
  });

  it('disables every action button for a row while an action is in flight', () => {
    const user = { userId: 1, email: 'a@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false };
    adminService.listUsers.and.returnValue(of({ users: [user] }));
    fixture.detectChanges();
    adminService.toggleActive.and.returnValue(of({}));

    fixture.componentInstance['rowActionInFlight'][1] = true;
    fixture.detectChanges();

    const buttons: HTMLButtonElement[] = Array.from(fixture.nativeElement.querySelectorAll('tbody tr button'));
    expect(buttons.every((btn) => btn.disabled)).toBe(true);
  });

  it('filters the table by the search query', () => {
    const a = { userId: 1, email: 'alice@esb.co.id', userName: 'A', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false };
    const b = { userId: 2, email: 'bob@esb.co.id', userName: 'B', flagActive: false, createdDate: '', jiraAccountId: '', hasCredential: true, credentialMasked: '••••ab3f', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false };
    adminService.listUsers.and.returnValue(of({ users: [a, b] }));
    fixture.detectChanges();

    fixture.componentInstance['onQuery']('bob');
    fixture.detectChanges();

    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    expect(rows.length).toBe(1);
    expect(rows[0].textContent).toContain('bob@esb.co.id');
  });

  describe('navigation to the user detail', () => {
    const user = { userId: 2, email: 'b@esb.co.id', userName: 'B', flagActive: true, createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '', chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false };
    let navigate: jasmine.Spy;

    beforeEach(() => {
      navigate = spyOn(TestBed.inject(Router), 'navigate').and.resolveTo(true);
      adminService.listUsers.and.returnValue(of({ users: [user] }));
      fixture.detectChanges();
    });

    it('opens the detail page when the row is clicked', () => {
      (fixture.nativeElement.querySelector('.user-cell') as HTMLElement).click();
      expect(navigate).toHaveBeenCalledWith(['/admin/users', 2]);
    });

    it('does not navigate when a row action is clicked', () => {
      adminService.toggleActive.and.returnValue(of({}));
      (fixture.nativeElement.querySelector('.actions button') as HTMLButtonElement).click();
      expect(navigate).not.toHaveBeenCalled();
    });

    it('has an Atur link to the detail page', () => {
      const link = fixture.nativeElement.querySelector('.actions a') as HTMLAnchorElement;
      expect(link.getAttribute('aria-label')).toBe('Atur user');
      expect(link.textContent!.trim()).toBe('');
      expect(link.getAttribute('href')).toBe('/admin/users/2');
    });
  });
  it('shows the Nudge App activation date and the sender by name (AC4.1, AC4.4)', () => {
    const base = { userName: '', createdDate: '', jiraAccountId: '', hasCredential: false, credentialMasked: '', chatSender: null, flagActive: true };
    adminService.listUsers.and.returnValue(of({
      users: [
        { ...base, userId: 1, email: 'a@esb.co.id', chatAppActivated: true, chatAppActivatedDate: '2026-11-15T02:00:00Z', effectiveChatSender: 'nudge' as const, isAdmin: false, isOwner: false },
        { ...base, userId: 2, email: 'b@esb.co.id', chatAppActivated: false, chatAppActivatedDate: null, effectiveChatSender: 'operator' as const, isAdmin: false, isOwner: false },
      ],
    }));
    fixture.detectChanges();
    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    expect(rows[0].querySelector('[data-col="chat-app"]').textContent).toContain('✓ 15 Nov 2026');
    expect(rows[0].querySelector('[data-col="sender"]').textContent.trim()).toBe('Nudge');
    expect(rows[1].querySelector('[data-col="chat-app"]').textContent.trim()).toBe('—');
    expect(rows[1].querySelector('[data-col="sender"]').textContent.trim()).toBe('Yusuf Wijaya');
  });

  describe('admin roster', () => {
    const row = (partial: Partial<UserRow>): UserRow => ({
      userId: 0, email: '', userName: '', flagActive: true, createdDate: '', jiraAccountId: 'acc', hasCredential: true, credentialMasked: '••••1234',
      chatAppActivated: false, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator', isAdmin: false, isOwner: false, ...partial,
    });
    const users = [
      row({ userId: 1, email: 'yusuf.wijaya@esb.co.id', isOwner: true, isAdmin: true }),
      row({ userId: 2, email: 'keva.galih@esb.co.id', isAdmin: true }),
      row({ userId: 3, email: 'eren.dhoheiri@esb.co.id' }),
    ];
    const el = () => fixture.nativeElement as HTMLElement;
    const rowOf = (id: number) => el().querySelector(`tr[data-user-id="${id}"]`) as HTMLElement;
    const load = () => {
      adminService.listUsers.and.returnValue(of({ users }));
      fixture.detectChanges();
    };

    it('marks the owner and the promoted admins with a badge', () => {
      load();
      expect(rowOf(1).querySelector('[data-badge="owner"]')?.textContent).toContain('Pemilik');
      expect(rowOf(1).querySelector('[data-badge="admin"]')).toBeNull();
      expect(rowOf(2).querySelector('[data-badge="admin"]')?.textContent).toContain('Admin');
      expect(rowOf(3).querySelector('[data-badge]')).toBeNull();
    });

    it('lets the owner promote and demote, but not on his own row', () => {
      load();
      expect(rowOf(1).querySelector('[data-toggle-admin]')).toBeNull();
      expect(rowOf(2).querySelector('[data-toggle-admin]')?.getAttribute('aria-label')).toBe('Cabut Admin');
      expect(rowOf(3).querySelector('[data-toggle-admin]')?.getAttribute('aria-label')).toBe('Jadikan Admin');
    });

    it('promotes after confirmation and reloads the list', async () => {
      load();
      confirmService.ask.and.resolveTo(true);
      adminService.setAdmin.and.returnValue(of({ ok: true, isAdmin: true }));
      (rowOf(3).querySelector('[data-toggle-admin]') as HTMLButtonElement).click();
      await fixture.whenStable();
      expect(adminService.setAdmin).toHaveBeenCalledWith(3, true);
      expect(adminService.listUsers).toHaveBeenCalledTimes(2);
    });

    it('does nothing when the confirmation is cancelled', async () => {
      load();
      confirmService.ask.and.resolveTo(false);
      (rowOf(3).querySelector('[data-toggle-admin]') as HTMLButtonElement).click();
      await fixture.whenStable();
      expect(adminService.setAdmin).not.toHaveBeenCalled();
    });

    it('shows the server message when promoting is refused', async () => {
      load();
      confirmService.ask.and.resolveTo(true);
      adminService.setAdmin.and.returnValue(throwError(() => ({ status: 403, error: { error: 'Hanya pemilik yang bisa menjadikan atau mencabut admin.' } })));
      (rowOf(3).querySelector('[data-toggle-admin]') as HTMLButtonElement).click();
      await fixture.whenStable();
      fixture.detectChanges();
      expect(rowOf(3).textContent).toContain('Hanya pemilik yang bisa');
    });

    describe('seen by an admin who is not the owner', () => {
      beforeEach(() => me.next({ email: 'keva.galih@esb.co.id', isAdmin: true, isOwner: false }));

      it('has no promote or demote buttons at all', () => {
        load();
        expect(el().querySelectorAll('[data-toggle-admin]').length).toBe(0);
      });

      it('locks every change on the owner row, but not on the other rows', async () => {
        load();
        await fixture.whenStable(); // ngModel applies [disabled] after a microtask
        fixture.detectChanges();
        const owner = rowOf(1);
        expect((owner.querySelector('.inline-edit input') as HTMLInputElement).disabled).toBeTrue();
        expect((owner.querySelector('.inline-edit .icon-btn') as HTMLButtonElement).disabled).toBeTrue();
        expect((owner.querySelector('[data-delete-credential]') as HTMLButtonElement).disabled).toBeTrue();
        expect((owner.querySelector('.action-row .icon-btn.danger') as HTMLButtonElement).disabled).toBeTrue();
        const buttons = Array.from(owner.querySelectorAll('.action-row button')) as HTMLButtonElement[];
        expect(buttons.length).toBeGreaterThan(0);
        expect(buttons.every((b) => b.disabled)).toBeTrue();
        expect(owner.querySelector('.action-row a.icon-btn')).withContext('the owner page can still be opened to read').not.toBeNull();

        const other = rowOf(3);
        expect((other.querySelector('.action-row .icon-btn.danger') as HTMLButtonElement).disabled).toBeFalse();
        expect((other.querySelector('.inline-edit input') as HTMLInputElement).disabled).toBeFalse();
      });
    });
  });

  describe('filters, icon actions and pagination', () => {
    const mk = (id: number, over: Partial<UserRow> = {}): UserRow => ({
      userId: id, email: `user${String(id).padStart(2, '0')}@esb.co.id`, userName: `User ${id}`, flagActive: true, createdDate: '', jiraAccountId: 'acc',
      hasCredential: false, credentialMasked: '', chatAppActivated: true, chatAppActivatedDate: null, chatSender: null, effectiveChatSender: 'operator',
      isAdmin: false, isOwner: false, ...over,
    });
    const el = () => fixture.nativeElement as HTMLElement;
    const q = (s: string) => el().querySelector(s) as HTMLElement | null;
    const emails = () => Array.from(el().querySelectorAll('tbody tr')).map((r) => r.querySelector('strong')!.textContent!.trim());
    const load = (users: UserRow[]) => {
      adminService.listUsers.and.returnValue(of({ users }));
      localStorage.removeItem('nudge.adminUsers.pageSize');
      fixture.detectChanges();
    };
    const twenty = () => Array.from({ length: 20 }, (_, i) => mk(i + 1));

    it('every action is an icon button with a tooltip and an accessible name, never text', () => {
      load([mk(1)]);
      const buttons = Array.from(el().querySelectorAll('.action-row .icon-btn')) as HTMLElement[];
      expect(buttons.length).toBeGreaterThanOrEqual(3);
      for (const b of buttons) {
        expect(b.textContent!.trim()).withContext(b.outerHTML).toBe('');
        expect(b.getAttribute('aria-label')).toBeTruthy();
        expect(b.getAttribute('title')).toBeTruthy();
        expect(b.querySelector('app-icon')).not.toBeNull();
      }
      expect(el().querySelector('.action-row .btn')).toBeNull();
    });

    it('the active/inactive icon says what it will do', () => {
      load([mk(1), mk(2, { flagActive: false })]);
      expect(q('tr[data-user-id="1"] [data-act="toggle-active"]')!.getAttribute('aria-label')).toBe('Nonaktifkan user');
      expect(q('tr[data-user-id="2"] [data-act="toggle-active"]')!.getAttribute('aria-label')).toBe('Aktifkan user');
    });

    it('counts users in the tabs and filters by them, with the search narrowing the counts', () => {
      load([mk(1), mk(2, { chatAppActivated: false }), mk(3, { flagActive: false }), mk(4, { chatAppActivated: false })]);
      const count = (name: string) => Number(q(`[data-filter="${name}"] .seg-count`)!.textContent);
      expect([count('all'), count('notActivated'), count('inactive')]).toEqual([4, 2, 1]);
      expect(q('[data-filter="all"]')!.classList).toContain('on');

      (q('[data-filter="inactive"]') as HTMLButtonElement).click();
      fixture.detectChanges();
      expect(emails()).toEqual(['user03@esb.co.id']);

      (q('[data-filter="notActivated"]') as HTMLButtonElement).click();
      fixture.detectChanges();
      expect(emails()).toEqual(['user02@esb.co.id', 'user04@esb.co.id']);

      fixture.componentInstance['onQuery']('user04');
      fixture.detectChanges();
      expect(emails()).toEqual(['user04@esb.co.id']);
      expect([count('all'), count('notActivated'), count('inactive')]).toEqual([1, 1, 0]);
    });

    it('searches the name as well as the email', () => {
      load([mk(1, { userName: 'Alice Wong' }), mk(2, { userName: 'Bob' })]);
      fixture.componentInstance['onQuery']('alice');
      fixture.detectChanges();
      expect(emails()).toEqual(['user01@esb.co.id']);
    });

    it('cuts the list into pages of 8 by default, with the range and the page numbers', () => {
      load(twenty());
      expect(emails().length).toBe(8);
      expect(emails()[0]).toBe('user01@esb.co.id');
      expect(q('[data-range]')!.textContent).toContain('1–8 dari 20');
      expect(Array.from(el().querySelectorAll('[data-page]')).map((b) => b.textContent!.trim())).toEqual(['1', '2', '3']);
      expect(q('[data-page="1"]')!.getAttribute('aria-current')).toBe('page');
      expect((q('[data-prev]') as HTMLButtonElement).disabled).toBeTrue();
    });

    it('moves between pages with the numbers and the arrows', () => {
      load(twenty());
      (q('[data-next]') as HTMLButtonElement).click();
      fixture.detectChanges();
      expect(emails()[0]).toBe('user09@esb.co.id');
      expect(q('[data-range]')!.textContent).toContain('9–16 dari 20');
      (q('[data-page="3"]') as HTMLButtonElement).click();
      fixture.detectChanges();
      expect(emails()).toEqual(['user17@esb.co.id', 'user18@esb.co.id', 'user19@esb.co.id', 'user20@esb.co.id']);
      expect(q('[data-range]')!.textContent).toContain('17–20 dari 20');
      expect((q('[data-next]') as HTMLButtonElement).disabled).toBeTrue();
      (q('[data-prev]') as HTMLButtonElement).click();
      fixture.detectChanges();
      expect(emails()[0]).toBe('user09@esb.co.id');
    });

    it('goes back to page 1 when the filter or the search changes, and never stays past the end', () => {
      load(twenty());
      (q('[data-page="3"]') as HTMLButtonElement).click();
      fixture.detectChanges();
      fixture.componentInstance['onQuery']('user1');
      fixture.detectChanges();
      expect(q('[data-page="1"]')!.getAttribute('aria-current')).toBe('page');
      expect(emails()[0]).toBe('user10@esb.co.id');

      // 10 users match "user1" (user10–19): two pages. A stale page 99 falls back to the last one.
      fixture.componentInstance['page'] = 99;
      fixture.detectChanges();
      expect(emails().length).toBe(2);
      expect(fixture.componentInstance['currentPage']).toBe(2);
    });

    it('lets the admin pick how many rows a page has, and remembers it', () => {
      load(twenty());
      fixture.componentInstance['setPageSize'](5);
      fixture.detectChanges();
      expect(emails().length).toBe(5);
      expect(q('[data-range]')!.textContent).toContain('1–5 dari 20');
      // 4 pages: 1 2 … 4 (only the neighbours of the current page are listed).
      expect(Array.from(el().querySelectorAll('[data-page]')).map((n) => n.textContent!.trim())).toEqual(['1', '2', '4']);
      expect(el().querySelectorAll('.pager-gap').length).toBe(1);
      expect(localStorage.getItem('nudge.adminUsers.pageSize')).toBe('5');
      localStorage.removeItem('nudge.adminUsers.pageSize');
    });

    it('shortens a long page list with gaps', () => {
      load(Array.from({ length: 100 }, (_, i) => mk(i + 1)));
      fixture.componentInstance['setPageSize'](5);
      fixture.componentInstance['goTo'](10);
      fixture.detectChanges();
      expect(fixture.componentInstance['pageNumbers']).toEqual([1, null, 9, 10, 11, null, 20]);
      localStorage.removeItem('nudge.adminUsers.pageSize');
    });

    it('shows no page arrows for a single page, only the range and the size', () => {
      load([mk(1), mk(2)]);
      expect(q('.pager-nav')).toBeNull();
      expect(q('[data-range]')!.textContent).toContain('1–2 dari 2');
      expect(q('[data-page-size]')).not.toBeNull();
    });

    it('has no pager when nothing matches', () => {
      load([mk(1)]);
      fixture.componentInstance['onQuery']('zzz');
      fixture.detectChanges();
      expect(q('[data-pager]')).toBeNull();
      expect(q('.empty-row')!.textContent).toContain('Tidak ada user yang cocok dengan "zzz"');
    });

    it('opens the notActivated tab straight from the dashboard link (?chatApp=inactive)', async () => {
      const route = TestBed.inject(ActivatedRoute);
      (route.snapshot as { queryParamMap: unknown }).queryParamMap = convertToParamMap({ chatApp: 'inactive' });
      adminService.listUsers.and.returnValue(of({ users: [mk(1), mk(2, { chatAppActivated: false })] }));
      const f = TestBed.createComponent(AdminUsersComponent);
      f.detectChanges();
      expect((f.nativeElement.querySelector('[data-filter="notActivated"]') as HTMLElement).classList).toContain('on');
      expect(f.nativeElement.querySelectorAll('tbody tr').length).toBe(1);
    });

    it('never scrolls: the table wrapper does not clip or scroll its content', () => {
      load(twenty());
      const wrap = q('.users-wrap')!;
      expect(getComputedStyle(wrap).overflowX).not.toBe('auto');
      expect(getComputedStyle(wrap).overflowX).not.toBe('scroll');
      expect(getComputedStyle(q('table.users-table')!).tableLayout).toBe('fixed');
    });
  });
});
