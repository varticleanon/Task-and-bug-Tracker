import { Component, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import {
  AbstractControl,
  FormBuilder,
  ReactiveFormsModule,
  ValidationErrors,
  Validators
} from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';
import { AuthService } from '../../core/services/auth.service';

function matchPasswords(control: AbstractControl): ValidationErrors | null {
  const current = control.get('new_password')?.value;
  const confirm = control.get('confirm_password')?.value;
  return current === confirm ? null : { passwordMismatch: true };
}

@Component({
  selector: 'app-change-password',
  standalone: true,
  imports: [CommonModule, ReactiveFormsModule],
  template: `
    <main class="auth-screen">
      <div class="auth-card">
        <div class="auth-brand">
          <img class="brand-logo" src="logo.svg" alt="" width="28" height="28" />
          <span class="brand-name">Task &amp; Bug Tracker</span>
        </div>

        <h1 class="auth-title">Ganti password</h1>
        <p class="auth-subtitle">
          {{
            mustChange()
              ? 'Anda wajib memperbarui password sebelum melanjutkan.'
              : 'Perbarui password akun Anda secara berkala.'
          }}
        </p>

        @if (errorMessage(); as message) {
          <div class="alert alert-error mb-4" role="alert" aria-live="assertive">
            {{ message }}
          </div>
        }

        @if (mustChange()) {
          <div class="alert alert-info mb-4">
            Password baru harus berbeda dari password saat ini dan minimal 12 karakter.
          </div>
        }

        <form [formGroup]="passwordForm" (ngSubmit)="onSubmit()" novalidate class="password-form">
          <div class="form-group">
            <label class="form-label" for="current_password">Password saat ini</label>
            <input
              id="current_password"
              [type]="showCurrent() ? 'text' : 'password'"
              class="form-control"
              formControlName="current_password"
              autocomplete="current-password"
              [class.field-invalid]="isInvalid('current_password')"
              aria-describedby="current-password-error"
            />
            @if (currentPasswordError()) {
              <span id="current-password-error" class="field-error">
                Password saat ini salah.
              </span>
            } @else if (isInvalid('current_password')) {
              <span id="current-password-error" class="field-error">
                Password saat ini wajib diisi.
              </span>
            }
          </div>

          <div class="form-group">
            <label class="form-label" for="new_password">Password baru</label>
            <input
              id="new_password"
              [type]="showNew() ? 'text' : 'password'"
              class="form-control"
              formControlName="new_password"
              autocomplete="new-password"
              [class.field-invalid]="isInvalid('new_password')"
              aria-describedby="new-password-hint new-password-error"
            />
            <span id="new-password-hint" class="form-hint">
              Minimal 12 karakter, maksimal 128 karakter.
            </span>
            @if (isInvalid('new_password')) {
              <span id="new-password-error" class="field-error">
                Password baru wajib diisi dan minimal 12 karakter.
              </span>
            }
          </div>

          <div class="form-group">
            <label class="form-label" for="confirm_password">Konfirmasi password baru</label>
            <input
              id="confirm_password"
              [type]="showNew() ? 'text' : 'password'"
              class="form-control"
              formControlName="confirm_password"
              autocomplete="new-password"
              [class.field-invalid]="hasMismatch()"
              aria-describedby="confirm-password-error"
            />
            @if (hasMismatch()) {
              <span id="confirm-password-error" class="field-error">
                Konfirmasi password tidak cocok.
              </span>
            }
          </div>

          <button
            class="btn btn-primary btn-lg btn-block"
            type="submit"
            [disabled]="isLoading() || passwordForm.invalid"
          >
            @if (isLoading()) {
              <span class="spinner spinner-sm" aria-hidden="true"></span>
            }
            {{ isLoading() ? 'Menyimpan...' : 'Simpan password baru' }}
          </button>
        </form>
      </div>
    </main>
  `,
  styles: `
    .password-form {
      display: flex;
      flex-direction: column;
      gap: var(--space-4);
    }
  `
})
export class ChangePasswordComponent {
  private readonly fb = inject(FormBuilder);
  private readonly auth = inject(AuthService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  readonly passwordForm = this.fb.group(
    {
      current_password: ['', [Validators.required]],
      new_password: [
        '',
        [Validators.required, Validators.minLength(12), Validators.maxLength(128)]
      ],
      confirm_password: ['', [Validators.required]]
    },
    { validators: matchPasswords }
  );

  readonly showCurrent = signal(false);
  readonly showNew = signal(false);
  readonly isLoading = signal(false);
  readonly errorMessage = signal<string | null>(null);
  readonly currentPasswordError = signal(false);
  readonly mustChange = signal(false);

  constructor() {
    // Server tetap sumber kebenaran; nilai ini hanya untuk ditampilkan.
    this.mustChange.set(this.auth.currentUser()?.must_change_password ?? false);
  }

  isInvalid(controlName: string): boolean {
    const control = this.passwordForm.get(controlName);
    return !!control && control.touched && control.invalid;
  }

  hasMismatch(): boolean {
    const confirm = this.passwordForm.get('confirm_password');
    return !!confirm?.touched && this.passwordForm.hasError('passwordMismatch');
  }

  onSubmit(): void {
    if (this.passwordForm.invalid) {
      this.passwordForm.markAllAsTouched();
      return;
    }

    this.isLoading.set(true);
    this.errorMessage.set(null);
    this.currentPasswordError.set(false);

    const { current_password, new_password } = this.passwordForm.getRawValue();

    this.auth.changePassword(current_password!, new_password!).subscribe({
      next: () => {
        this.isLoading.set(false);
        const returnUrl = this.route.snapshot.queryParamMap.get('returnUrl');
        this.auth.navigateAfterLogin(returnUrl);
      },
      error: (err: HttpErrorResponse) => {
        this.isLoading.set(false);
        const code = err?.error?.error?.code;
        if (code === 'INVALID_CREDENTIALS') {
          this.currentPasswordError.set(true);
          this.errorMessage.set('Password saat ini tidak cocok.');
        } else {
          this.errorMessage.set(
            err?.error?.error?.message || 'Gagal memperbarui password. Silakan coba lagi.'
          );
        }
      }
    });
  }
}
