import { Component, OnInit, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { NgFor, NgIf } from '@angular/common';
import { AdminService, UserRow, formatActivationDate } from '../../../core/services/admin.service';
import { AuthService } from '../../../core/services/auth.service';
import { ChatAppService } from '../../../core/services/chat-app.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { UserAvatarComponent, adminUserAvatarUrl } from '../../../shared/ui/user-avatar.component';
import { SelectComponent, SelectOption } from '../../../shared/ui/select.component';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';

export type UserFilter = 'all' | 'notActivated' | 'inactive';
export const PAGE_SIZES = [5, 8, 10, 20];
const PAGE_SIZE_KEY = 'nudge.adminUsers.pageSize';

@Component({
  selector: 'app-admin-users',
  standalone: true,
  imports: [NgFor, NgIf, FormsModule, RouterLink, IconComponent, UserAvatarComponent, SelectComponent],
  templateUrl: './admin-users.component.html',
  styleUrl: './admin-users.component.scss',
})
export class AdminUsersComponent implements OnInit {
  protected readonly avatarUrl = adminUserAvatarUrl;
  protected readonly activationDate = formatActivationDate;
  /** Placeholder rows while the first load runs (same columns as the real table). */
  protected readonly skeletonRows = [0, 1, 2, 3, 4, 5];
  protected users: UserRow[] = [];
  protected loading = true;
  protected error: string | null = null;
  protected query = '';
  protected filter: UserFilter = 'all';
  protected page = 1;
  protected pageSize = this.storedPageSize();
  protected readonly pageSizeOptions: SelectOption<number>[] = PAGE_SIZES.map((n) => ({ value: n, label: `${n} baris` }));
  protected rowActionInFlight: Record<number, boolean> = {};
  protected rowMessage: Record<number, string> = {};
  protected jiraAccountIdDrafts: Record<number, string> = {};

  private adminService = inject(AdminService);
  private confirm = inject(ConfirmService);
  private toast = inject(ToastService);
  private router = inject(Router);
  private route = inject(ActivatedRoute);
  private chatApp = inject(ChatAppService);
  private auth = inject(AuthService);

  /** Only the owner promotes admins and may change the owner's own account. */
  protected get viewerIsOwner(): boolean {
    return !!this.auth.currentUser$.value?.isOwner;
  }

  /** The owner's account is read-only for every other admin. */
  protected locked(user: UserRow): boolean {
    return user.isOwner && !this.viewerIsOwner;
  }

  /** The operator account's display name (AC4.4), from the status endpoint. */
  protected operatorName(): string {
    return this.chatApp.status()?.operatorName || 'Operator';
  }

  ngOnInit(): void {
    if (this.route.snapshot.queryParamMap.get('chatApp') === 'inactive') this.filter = 'notActivated';
    if (!this.chatApp.status()) {
      this.chatApp.load().subscribe({ error: () => undefined });
    }
    this.loadUsers();
  }

  private matches(u: UserRow, filter: UserFilter): boolean {
    switch (filter) {
      case 'notActivated': return u.flagActive && !u.chatAppActivated;
      case 'inactive': return !u.flagActive;
      default: return true;
    }
  }

  /** The search narrows the list first; the tabs count within it. */
  private get searched(): UserRow[] {
    const q = this.query.trim().toLowerCase();
    return q ? this.users.filter((u) => u.email.toLowerCase().includes(q) || (u.userName ?? '').toLowerCase().includes(q)) : this.users;
  }

  protected count(filter: UserFilter): number {
    return this.searched.filter((u) => this.matches(u, filter)).length;
  }

  protected get filteredUsers(): UserRow[] {
    return this.searched.filter((u) => this.matches(u, this.filter));
  }

  protected setFilter(filter: UserFilter): void {
    this.filter = filter;
    this.page = 1;
  }

  protected onQuery(value: string): void {
    this.query = value;
    this.page = 1;
  }

  // ---- pagination: no scrolling, the list is cut into pages ----

  protected get totalPages(): number {
    return Math.max(1, Math.ceil(this.filteredUsers.length / this.pageSize));
  }

  /** The page in range, also after a delete or a filter made the list shorter. */
  protected get currentPage(): number {
    return Math.min(Math.max(1, this.page), this.totalPages);
  }

  protected get pagedUsers(): UserRow[] {
    const start = (this.currentPage - 1) * this.pageSize;
    return this.filteredUsers.slice(start, start + this.pageSize);
  }

  protected get rangeLabel(): string {
    const total = this.filteredUsers.length;
    if (total === 0) return '';
    const start = (this.currentPage - 1) * this.pageSize + 1;
    return `${start}–${Math.min(total, start + this.pageSize - 1)} dari ${total}`;
  }

  /** 1 … 4 5 6 … 12: the first, the last and the neighbours of the current page. */
  protected get pageNumbers(): (number | null)[] {
    const total = this.totalPages;
    const current = this.currentPage;
    const shown = new Set([1, total, current - 1, current, current + 1].filter((n) => n >= 1 && n <= total));
    const out: (number | null)[] = [];
    let last = 0;
    for (const n of [...shown].sort((a, b) => a - b)) {
      if (n - last > 1) out.push(null);
      out.push(n);
      last = n;
    }
    return out;
  }

  protected goTo(page: number): void {
    this.page = Math.min(Math.max(1, page), this.totalPages);
  }

  protected setPageSize(size: number): void {
    this.pageSize = size;
    this.page = 1;
    try { localStorage.setItem(PAGE_SIZE_KEY, String(size)); } catch { /* storage unavailable: the choice just is not kept */ }
  }

  private storedPageSize(): number {
    try {
      const n = Number(localStorage.getItem(PAGE_SIZE_KEY));
      if (PAGE_SIZES.includes(n)) return n;
    } catch { /* storage unavailable */ }
    return 8;
  }

  /** Row click → the user's detail page (settings, mute, credential). */
  protected open(user: UserRow): void {
    this.router.navigate(['/admin/users', user.userId]);
  }

  protected loadUsers(): void {
    // Only the very first load shows the skeleton; reloads after an action
    // keep the table on screen so the page doesn't flicker.
    this.loading = this.users.length === 0;
    this.error = null;

    this.adminService.listUsers().subscribe({
      next: (res) => {
        this.users = res.users;
        this.jiraAccountIdDrafts = Object.fromEntries(res.users.map((u) => [u.userId, u.jiraAccountId]));
        this.loading = false;
      },
      error: (err) => {
        this.error = err?.status === 403 ? 'Kamu tidak punya akses ke halaman ini' : 'Gagal memuat daftar user, coba lagi';
        this.loading = false;
      },
    });
  }

  protected onToggleActive(user: UserRow): void {
    this.startRowAction(user);
    this.adminService.toggleActive(user.userId, !user.flagActive).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        this.toast.success(user.flagActive ? 'User dinonaktifkan' : 'User diaktifkan', user.email);
        this.loadUsers();
      },
      error: (err) => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = err?.error?.error ?? 'Gagal mengubah status user';
      },
    });
  }

  protected onSaveJiraAccountId(user: UserRow): void {
    const value = this.jiraAccountIdDrafts[user.userId];
    this.startRowAction(user);
    this.adminService.editJiraAccountId(user.userId, value).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        user.jiraAccountId = value;
        this.toast.success('Jira Account ID tersimpan', user.email);
      },
      error: (err) => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = err?.error?.error ?? 'Gagal menyimpan Jira Account ID';
      },
    });
  }

  protected async onDeleteCredential(user: UserRow): Promise<void> {
    const confirmed = await this.confirm.ask({
      title: 'Hapus credential user?',
      message: `Credential Jira pribadi milik ${user.email} akan dihapus. Aksi Jira user ini kembali memakai akun bot.`,
      confirmText: 'Hapus credential',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    this.startRowAction(user);
    this.adminService.deleteCredential(user.userId).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        this.toast.success('Credential dihapus', user.email);
        this.loadUsers();
      },
      error: () => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = 'Gagal menghapus credential';
      },
    });
  }

  protected async onDeleteUser(user: UserRow): Promise<void> {
    const confirmed = await this.confirm.ask({
      title: 'Hapus user permanen?',
      message: `${user.email} akan dihapus beserta datanya. Tindakan ini tidak bisa dibatalkan.`,
      confirmText: 'Hapus user',
      tone: 'danger',
    });
    if (!confirmed) {
      return;
    }
    this.startRowAction(user);
    this.adminService.deleteUser(user.userId).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        this.toast.success('User dihapus', user.email);
        this.loadUsers();
      },
      error: (err) => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = err?.error?.error ?? 'Gagal menghapus user';
      },
    });
  }

  /** Owner only: promote the user to admin, or take the role away. */
  protected async onToggleAdmin(user: UserRow): Promise<void> {
    const promote = !user.isAdmin;
    const confirmed = await this.confirm.ask({
      title: promote ? 'Jadikan admin?' : 'Cabut peran admin?',
      message: promote
        ? `${user.email} akan mendapat semua hak admin, termasuk mengatur user lain, aturan board, dan akun Jira.`
        : `${user.email} tidak lagi bisa membuka panel admin.`,
      points: promote ? ['Admin tidak bisa mengubah, menonaktifkan, atau menghapus akun pemilik.', 'Hanya pemilik yang bisa menjadikan atau mencabut admin.'] : undefined,
      confirmText: promote ? 'Jadikan admin' : 'Cabut admin',
      tone: promote ? 'primary' : 'danger',
      icon: 'shield',
    });
    if (!confirmed) {
      return;
    }
    this.startRowAction(user);
    this.adminService.setAdmin(user.userId, promote).subscribe({
      next: () => {
        this.rowActionInFlight[user.userId] = false;
        this.toast.success(promote ? 'Dijadikan admin' : 'Peran admin dicabut', user.email);
        this.loadUsers();
      },
      error: (err) => {
        this.rowActionInFlight[user.userId] = false;
        this.rowMessage[user.userId] = err?.error?.error ?? 'Gagal mengubah peran admin';
      },
    });
  }

  private startRowAction(user: UserRow): void {
    this.rowActionInFlight[user.userId] = true;
    delete this.rowMessage[user.userId];
  }
}
