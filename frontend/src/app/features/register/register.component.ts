import { Component, inject, signal } from '@angular/core';
import { Router } from '@angular/router';
import { IconComponent } from '../../shared/ui/icon.component';
import { SignupError, SignupMe, SignupService, chatAdminUrl } from '../../core/services/signup.service';

/**
 * Pendaftaran. "Masuk" never creates an account: a Google identity with no
 * account lands here. Pressing the button adds the user to the Google Group
 * first; only when that works is the account created. A failure shows the
 * reason and asks the user to chat the admin (nothing was created).
 */
@Component({
  selector: 'app-register',
  standalone: true,
  imports: [IconComponent],
  templateUrl: './register.component.html',
  styleUrl: './register.component.scss',
})
export class RegisterComponent {
  private api = inject(SignupService);
  private router = inject(Router);

  protected me = signal<SignupMe | null>(null);
  protected loading = signal(true);
  /** The signed-in-with-Google window ran out (or was never there). */
  protected expired = signal(false);
  protected submitting = signal(false);
  protected error = signal<SignupError | null>(null);
  protected readonly chatAdminUrl = chatAdminUrl;

  constructor() {
    this.api.me().subscribe({
      next: (me) => {
        if (me.registered) {
          // An account exists already (another tab): the normal login takes over.
          this.router.navigateByUrl('/');
          return;
        }
        this.me.set(me);
        this.loading.set(false);
      },
      error: () => {
        this.expired.set(true);
        this.loading.set(false);
      },
    });
  }

  protected register(): void {
    if (this.submitting()) return;
    this.submitting.set(true);
    this.error.set(null);
    this.api.register().subscribe({
      next: () => {
        // The session is set: step 2 (board setup) is decided on the dashboard.
        this.router.navigateByUrl('/dashboard');
      },
      error: (err) => {
        this.submitting.set(false);
        if (err?.status === 401) {
          this.expired.set(true);
          return;
        }
        const body = (err?.error ?? {}) as Partial<SignupError>;
        this.error.set({
          error: body.error ?? 'register_failed',
          message: body.message ?? 'Pendaftaran gagal. Akun kamu belum dibuat, coba lagi atau hubungi admin lewat chat.',
          adminEmail: body.adminEmail ?? this.me()?.adminEmail,
        });
      },
    });
  }
}
