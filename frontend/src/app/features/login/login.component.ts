import { Component, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';
import { finalize } from 'rxjs';
import { AuthService } from '../../core/services/auth.service';
import { IconComponent } from '../../shared/components/icon/icon.component';

@Component({
  selector: 'app-login',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule, IconComponent],
  template: `
    <main class="auth-screen">
      <div class="auth-card">
        <div class="auth-brand">
          <img class="brand-logo" src="logo.svg" alt="" width="28" height="28" />
          <span class="brand-name">Task &amp; Bug Tracker</span>
        </div>

        <h1 class="auth-title">Masuk ke workspace Anda</h1>
        <p class="auth-subtitle">Gunakan email yang terdaftar oleh administrator.</p>

        @if (errorMessage(); as message) {
          <div class="alert alert-error mb-4" role="alert" aria-live="assertive">
            {{ message }}
          </div>
        }

        <form [formGroup]="loginForm" (ngSubmit)="onSubmit()" novalidate class="login-form">
          <div class="form-group">
            <label class="form-label" for="email">Email</label>
            <input
              id="email"
              type="email"
              class="form-control"
              formControlName="email"
              placeholder="nama@domain.com"
              autocomplete="email"
              [class.field-invalid]="emailInvalid()"
              aria-describedby="email-error"
            />
            @if (emailInvalid(); as message) {
              <span id="email-error" class="field-error">{{ message }}</span>
            }
          </div>

          <div class="form-group">
            <label class="form-label" for="password">Password</label>
            <div class="password-field">
              <input
                id="password"
                [type]="showPassword() ? 'text' : 'password'"
                class="form-control"
                formControlName="password"
                placeholder="Masukkan password"
                autocomplete="current-password"
                [class.field-invalid]="passwordInvalid()"
                aria-describedby="password-error"
              />
              <button
                type="button"
                class="password-toggle"
                (click)="showPassword.set(!showPassword())"
                [attr.aria-label]="showPassword() ? 'Sembunyikan password' : 'Tampilkan password'"
                [attr.aria-pressed]="showPassword()"
              >
                <app-icon [name]="showPassword() ? 'eye-off' : 'eye'" [size]="18" />
              </button>
            </div>
            @if (passwordInvalid(); as message) {
              <span id="password-error" class="field-error">{{ message }}</span>
            }
          </div>

          <button
            class="btn btn-primary btn-lg btn-block"
            type="submit"
            [disabled]="isLoading() || loginForm.invalid"
          >
            @if (isLoading()) {
              <span class="spinner spinner-sm" aria-hidden="true"></span>
            }
            {{ isLoading() ? 'Memproses...' : 'Masuk' }}
          </button>
        </form>

        <p class="auth-footer">
          Akun hanya dibuat oleh administrator. Hubungi Super Admin bila belum memiliki akses.
        </p>
      </div>
    </main>
  `,
  styles: `
    .login-form {
      display: flex;
      flex-direction: column;
      gap: var(--space-4);
    }

    .password-field {
      position: relative;
    }

    .password-field .form-control {
      padding-right: 3rem;
    }

    .password-toggle {
      position: absolute;
      right: 0.35rem;
      top: 50%;
      transform: translateY(-50%);
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 36px;
      height: 36px;
      padding: 0;
      border: none;
      border-radius: var(--radius-sm);
      background: transparent;
      color: var(--ink-3);
    }

    .password-toggle:hover {
      background: var(--accent-soft);
      color: var(--accent-text);
    }
  `
})
export class LoginComponent {
  private readonly fb = inject(FormBuilder);
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  readonly loginForm = this.fb.group({
    email: ['', [Validators.required, Validators.email]],
    password: ['', [Validators.required]]
  });

  readonly showPassword = signal(false);
  readonly isLoading = signal(false);
  readonly errorMessage = signal<string | null>(null);

  emailInvalid(): string | null {
    const control = this.loginForm.get('email');
    if (!control?.touched) return null;
    if (control.hasError('required')) return 'Email wajib diisi.';
    if (control.hasError('email')) return 'Format email tidak valid.';
    return null;
  }

  passwordInvalid(): string | null {
    const control = this.loginForm.get('password');
    if (!control?.touched) return null;
    if (control.hasError('required')) return 'Password wajib diisi.';
    return null;
  }

  onSubmit(): void {
    if (this.loginForm.invalid) {
      this.loginForm.markAllAsTouched();
      return;
    }

    this.isLoading.set(true);
    this.errorMessage.set(null);

    const { email, password } = this.loginForm.getRawValue();

    this.auth
      .login(email!.trim(), password!)
      .pipe(finalize(() => this.isLoading.set(false)))
      .subscribe({
        next: res => {
          const user = res.data?.user;
          if (!user) {
            this.errorMessage.set('Respons login tidak valid dari server.');
            return;
          }

          const returnUrl = this.route.snapshot.queryParamMap.get('returnUrl');
          try {
            this.auth.navigateAfterLogin(returnUrl);
          } catch {
            this.errorMessage.set('Gagal arahkan ke halaman utama.');
          }
        },
        error: (err: HttpErrorResponse) => {
          const code = err?.error?.error?.code;
          if (code === 'ACCOUNT_DISABLED') {
            this.errorMessage.set('Akun Anda dinonaktifkan. Hubungi administrator.');
          } else if (code === 'INVALID_CREDENTIALS') {
            this.errorMessage.set('Email atau password salah. Silakan coba lagi.');
          } else {
            this.errorMessage.set(
              err?.error?.error?.message || 'Login gagal. Silakan coba lagi.'
            );
          }
        }
      });
  }
}
