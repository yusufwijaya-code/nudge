import { Routes } from '@angular/router';
import { LoginComponent } from './features/login/login.component';
import { DashboardShellComponent } from './layouts/dashboard-shell/dashboard-shell.component';
import { HomeComponent } from './features/dashboard/home/home.component';
import { AuthGuard } from './core/guards/auth.guard';
import { AdminGuard } from './core/guards/admin.guard';
import { GuestGuard } from './core/guards/guest.guard';

export const routes: Routes = [
  { path: '', pathMatch: 'full', component: LoginComponent, canActivate: [GuestGuard], title: 'Masuk · Nudge' },
  {
    path: 'dashboard',
    component: DashboardShellComponent,
    canActivate: [AuthGuard],
    children: [
      {
        path: '',
        pathMatch: 'full',
        component: HomeComponent,
        title: 'Dashboard · Nudge',
        data: { heading: 'Dashboard', subheading: 'Ringkasan tiket dan budget waktu kamu' },
      },
      // Tech Spec "Papan Tiket & Cuti".
      {
        path: 'board',
        loadComponent: () => import('./features/dashboard/board-page/board-page.component').then((m) => m.BoardPageComponent),
        canDeactivate: [(component: { canLeave?: () => Promise<boolean> }) => component.canLeave?.() ?? true],
        title: 'Papan Tiket · Nudge',
        data: { heading: 'Papan Tiket', subheading: 'Pindahkan status tiket sprint aktif kamu sesuai workflow Jira' },
      },
      {
        path: 'leave',
        loadComponent: () => import('./features/dashboard/leave/leave.component').then((m) => m.LeaveComponent),
        title: 'Cuti · Nudge',
        data: { heading: 'Cuti', subheading: 'Hari cuti tidak dihitung ke waktu tiket' },
      },
      {
        path: 'calendar',
        loadComponent: () => import('./features/dashboard/calendar/calendar.component').then((m) => m.CalendarComponent),
        title: 'Kalender · Nudge',
        data: { heading: 'Kalender', subheading: 'Hari dan jam yang tidak dihitung ke waktu tiket' },
      },
      {
        path: 'settings',
        loadComponent: () => import('./features/dashboard/settings/settings.component').then((m) => m.SettingsComponent),
        title: 'Pengaturan · Nudge',
        data: { heading: 'Pengaturan', subheading: 'Atur cara Nudge mengingatkan kamu' },
      },
      // The admin panel moved to its own shell (Tech Spec "Admin Panel Revamp").
      { path: 'admin', pathMatch: 'full', redirectTo: '/admin' },
    ],
  },
  {
    // Lazy-loaded: only admins ever download the admin panel.
    path: 'admin',
    loadComponent: () => import('./layouts/admin-shell/admin-shell.component').then((m) => m.AdminShellComponent),
    canActivate: [AuthGuard, AdminGuard],
    children: [
      {
        path: '',
        pathMatch: 'full',
        loadComponent: () => import('./features/admin/overview/admin-overview.component').then((m) => m.AdminOverviewComponent),
        title: 'Admin · Nudge',
        data: { heading: 'Dashboard', subheading: 'Ringkasan user Nudge', crumbs: ['Admin'] },
      },
      {
        path: 'users',
        loadComponent: () => import('./features/admin/users/admin-users.component').then((m) => m.AdminUsersComponent),
        title: 'User · Admin · Nudge',
        data: { heading: 'User', subheading: 'Kelola user dan pengaturannya', crumbs: ['Admin', 'User'] },
      },
      {
        path: 'users/:id',
        loadComponent: () => import('./features/admin/user-detail/admin-user-detail.component').then((m) => m.AdminUserDetailComponent),
        title: 'Detail User · Admin · Nudge',
        data: { heading: 'Detail User', subheading: 'Atur notifikasi, mute, dan Jira credential user', crumbs: ['Admin', 'User', 'Detail'] },
      },
      {
        path: 'broadcast',
        loadComponent: () => import('./features/admin/broadcast/broadcast.component').then((m) => m.BroadcastComponent),
        title: 'Broadcast · Admin · Nudge',
        data: { heading: 'Broadcast Message', subheading: 'Kirim pengumuman ke semua user aktif', crumbs: ['Admin', 'Broadcast Message'] },
      },
      {
        path: 'chat-logs',
        loadComponent: () => import('./features/admin/chat-logs/chat-logs.component').then((m) => m.ChatLogsComponent),
        title: 'Pesan · Admin · Nudge',
        data: { heading: 'Pesan', subheading: 'Chat tiap user dengan Nudge di Google Chat', crumbs: ['Admin', 'Pesan'] },
      },
      // Pengaturan Aplikasi: the deviation, and the central Jira account the bot acts as.
      {
        path: 'app-settings',
        loadComponent: () => import('./features/admin/app-settings/app-settings.component').then((m) => m.AppSettingsComponent),
        title: 'Pengaturan Aplikasi · Admin · Nudge',
        data: { heading: 'Pengaturan Aplikasi', subheading: 'Nilai yang berlaku untuk seluruh aplikasi, dan akun Jira yang dipakai bot', crumbs: ['Admin', 'Pengaturan Aplikasi'] },
      },
      // The old address of the Jira account page (it moved into Pengaturan Aplikasi).
      { path: 'jira-account', pathMatch: 'full', redirectTo: 'app-settings' },
      // Tech Spec "Aturan Board (Admin CRUD)".
      {
        path: 'tracking-rules',
        loadComponent: () => import('./features/admin/tracking-rules/tracking-rules.component').then((m) => m.TrackingRulesComponent),
        title: 'Aturan Board · Admin · Nudge',
        data: { heading: 'Aturan Board', subheading: 'Status & field Story Point yang dipantau per project Jira', crumbs: ['Admin', 'Aturan Board'] },
      },
      {
        path: 'tracking-rules/new',
        loadComponent: () => import('./features/admin/tracking-rules/tracking-rule-form.component').then((m) => m.TrackingRuleFormComponent),
        title: 'Tambah Aturan · Admin · Nudge',
        data: { heading: 'Tambah Aturan Board', subheading: 'Pilih status dan field Story Point dari data Jira', crumbs: ['Admin', 'Aturan Board', 'Tambah'] },
      },
      {
        path: 'tracking-rules/:key',
        loadComponent: () => import('./features/admin/tracking-rules/tracking-rule-form.component').then((m) => m.TrackingRuleFormComponent),
        title: 'Edit Aturan · Admin · Nudge',
        data: { heading: 'Edit Aturan Board', subheading: 'Status dan field dimuat ulang dari Jira', crumbs: ['Admin', 'Aturan Board', 'Edit'] },
      },
    ],
  },
  // Pendaftaran: a Google identity with no account lands here (no session yet).
  {
    path: 'register',
    loadComponent: () => import('./features/register/register.component').then((m) => m.RegisterComponent),
    title: 'Daftar · Nudge',
  },
  // Langkah 2: set up the user's boards that have no Aturan Board yet.
  {
    path: 'onboarding',
    loadComponent: () => import('./features/onboarding/onboarding.component').then((m) => m.OnboardingComponent),
    canActivate: [AuthGuard],
    title: 'Atur Board · Nudge',
  },
  { path: '**', redirectTo: '' },
];
