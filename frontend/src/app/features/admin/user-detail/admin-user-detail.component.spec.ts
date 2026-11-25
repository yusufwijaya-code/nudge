import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { Component } from '@angular/core';
import { provideHttpClient } from '@angular/common/http';
import { BehaviorSubject, of, throwError } from 'rxjs';
import { AdminUserDetailComponent } from './admin-user-detail.component';
import { ChatAppService } from '../../../core/services/chat-app.service';
import { AuthService, CurrentUser } from '../../../core/services/auth.service';
import { AdminService, UserRow } from '../../../core/services/admin.service';
import { AdminUserSettingsApi } from '../../../core/services/admin-user-settings';
import { SETTINGS_MODE, SettingsApi } from '../../../core/services/settings-api';
import { SettingsComponent } from '../../dashboard/settings/settings.component';

@Component({ selector: 'app-settings', standalone: true, template: '<p class="settings-stub">settings</p>' })
class SettingsStubComponent {}

const USER: UserRow = {
  userId: 12,
  email: 'fernando.marco@esb.co.id',
  userName: 'Marco',
  flagActive: true,
  createdDate: '',
  jiraAccountId: '712020:abc',
  hasCredential: true,
  credentialMasked: '••••50CF',
  chatAppActivated: false,
  chatAppActivatedDate: null,
  chatSender: null,
  effectiveChatSender: 'operator', isAdmin: false, isOwner: false,
};

describe('AdminUserDetailComponent', () => {
  let fixture: ComponentFixture<AdminUserDetailComponent>;
  let adminService: jasmine.SpyObj<AdminService>;
  let me: BehaviorSubject<CurrentUser | null>;
  const el = () => fixture.nativeElement as HTMLElement;

  beforeEach(async () => {
    adminService = jasmine.createSpyObj('AdminService', ['getUser', 'listUserBoards', 'toggleActive', 'editJiraAccountId']);
    adminService.listUserBoards.and.returnValue(of({ boards: [] }));
    me = new BehaviorSubject<CurrentUser | null>({ email: 'yusuf.wijaya@esb.co.id', isAdmin: true, isOwner: true });

    await TestBed.configureTestingModule({
      imports: [AdminUserDetailComponent],
      providers: [
        provideRouter([]),
        provideHttpClient(),
        { provide: ChatAppService, useValue: { load: () => of({ enabled: false }), activateUrl: () => '' } },
        { provide: AdminService, useValue: adminService },
        { provide: AuthService, useValue: { currentUser$: me } },
        { provide: ActivatedRoute, useValue: { snapshot: { paramMap: convertToParamMap({ id: '12' }) } } },
      ],
    })
      // The real Pengaturan page is covered by its own spec.
      .overrideComponent(AdminUserDetailComponent, { remove: { imports: [SettingsComponent] }, add: { imports: [SettingsStubComponent] } })
      .compileComponents();

    fixture = TestBed.createComponent(AdminUserDetailComponent);
  });

  it('shows the profile and the settings for the user', () => {
    adminService.getUser.and.returnValue(of(USER));
    fixture.detectChanges();

    expect(adminService.getUser).toHaveBeenCalledWith(12);
    expect(el().querySelector('.profile-card')!.textContent).toContain('fernando.marco@esb.co.id');
    expect(el().querySelector('.profile-card')!.textContent).toContain('••••50CF');
    expect(el().querySelector('.admin-editing-banner')!.textContent).toContain('fernando.marco@esb.co.id');
    expect(el().querySelector('.settings-stub')).not.toBeNull();
    expect(el().querySelector('[data-row="chat-app"]')!.textContent).toContain('Belum aktivasi');
  });

  it('shows the Nudge App activation date (AC4.3)', () => {
    adminService.getUser.and.returnValue(of({ ...USER, chatAppActivated: true, chatAppActivatedDate: '2026-11-15T02:00:00Z' }));
    fixture.detectChanges();
    expect(el().querySelector('[data-row="chat-app"]')!.textContent).toContain('Aktif sejak 15 Nov 2026');
  });

  it('points SettingsApi at the admin endpoints in admin mode', () => {
    adminService.getUser.and.returnValue(of(USER));
    fixture.detectChanges();

    const injector = fixture.debugElement.injector;
    expect(injector.get(SettingsApi)).toBe(injector.get(AdminUserSettingsApi));
    expect(injector.get(SETTINGS_MODE)).toBe('admin');
  });

  it('shows "User tidak ditemukan" on 404', () => {
    adminService.getUser.and.returnValue(throwError(() => ({ status: 404 })));
    fixture.detectChanges();

    expect(el().textContent).toContain('User tidak ditemukan');
    expect(el().querySelector('a[href="/admin/users"]')).not.toBeNull();
    expect(el().querySelector('.settings-stub')).toBeNull();
  });

  it('toggles the user active status from the profile card', () => {
    adminService.getUser.and.returnValue(of(USER));
    adminService.toggleActive.and.returnValue(of({}));
    fixture.detectChanges();

    (el().querySelector('.profile-grid button') as HTMLButtonElement).click();
    expect(adminService.toggleActive).toHaveBeenCalledWith(12, false);
  });

  describe('the owner account', () => {
    const OWNER: UserRow = { ...USER, userId: 1, email: 'yusuf.wijaya@esb.co.id', isOwner: true, isAdmin: true };

    it('is read-only for an admin who is not the owner', async () => {
      me.next({ email: 'keva.galih@esb.co.id', isAdmin: true, isOwner: false });
      adminService.getUser.and.returnValue(of(OWNER));
      fixture.detectChanges();
      await fixture.whenStable(); // ngModel applies [disabled] after a microtask
      fixture.detectChanges();

      expect(el().querySelector('[data-owner-lock]')!.textContent).toContain('hanya pemilik yang bisa mengubahnya');
      expect((el().querySelector('.profile-grid button') as HTMLButtonElement).disabled).toBeTrue();
      expect((el().querySelector('.inline-edit input') as HTMLInputElement).disabled).toBeTrue();
      expect(el().querySelector('[data-owner-settings]')!.classList).toContain('locked');
      expect(el().querySelector('[data-owner-settings]')!.hasAttribute('inert')).toBeTrue();
    });

    it('is editable by the owner himself', () => {
      adminService.getUser.and.returnValue(of(OWNER));
      fixture.detectChanges();

      expect(el().querySelector('[data-owner-lock]')).toBeNull();
      expect((el().querySelector('.profile-grid button') as HTMLButtonElement).disabled).toBeFalse();
      expect(el().querySelector('[data-owner-settings]')!.classList).not.toContain('locked');
    });

    it('does not lock the page of another user', () => {
      me.next({ email: 'keva.galih@esb.co.id', isAdmin: true, isOwner: false });
      adminService.getUser.and.returnValue(of(USER));
      fixture.detectChanges();

      expect(el().querySelector('[data-owner-lock]')).toBeNull();
      expect((el().querySelector('.profile-grid button') as HTMLButtonElement).disabled).toBeFalse();
    });
  });
});
