import { Component, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpErrorResponse } from '@angular/common/http';
import { finalize, timeout } from 'rxjs';
import { AdminUserService } from '../../../core/services/admin-user.service';
import {
  User,
  UserListResponse,
  CreateUserResponse,
  ResetPasswordResponse
} from '../../../core/models/user.model';
import { DialogComponent } from '../../../shared/components/dialog/dialog.component';
import { EmptyStateComponent } from '../../../shared/components/empty-state/empty-state.component';
import { ErrorStateComponent } from '../../../shared/components/error-state/error-state.component';
import { IconComponent } from '../../../shared/components/icon/icon.component';
import { StatusChipComponent } from '../../../shared/components/status-chip/status-chip.component';

@Component({
  selector: 'app-admin-users',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    DialogComponent,
    EmptyStateComponent,
    ErrorStateComponent,
    IconComponent,
    StatusChipComponent
  ],
  templateUrl: './users.component.html',
  styleUrls: ['./users.component.css']
})
export class UsersComponent {
  private readonly adminUserService = inject(AdminUserService);

  readonly users = signal<User[]>([]);
  readonly currentPage = signal(1);
  readonly totalPages = signal(1);
  readonly searchQuery = signal('');
  readonly errorMessage = signal<string | null>(null);
  readonly successMessage = signal<string | null>(null);
  readonly loading = signal<boolean>(true);

  readonly showCreateModal = signal(false);
  readonly createSubmitting = signal(false);

  readonly selectedUser = signal<User | null>(null);
  readonly editSubmitting = signal(false);

  readonly pendingReset = signal<User | null>(null);
  readonly tempPasswordNotice = signal<string | null>(null);
  readonly copyState = signal<'idle' | 'copied' | 'failed'>('idle');

  createEmail = '';
  createName = '';
  editName = '';
  editIsActive = true;

  constructor() {
    this.loadUsers(1);
  }

  loadUsers(page: number): void {
    this.errorMessage.set(null);
    this.loading.set(true);

    this.adminUserService
      .getUsers(page, 20, this.searchQuery())
      .pipe(
        timeout(15000),
        finalize(() => this.loading.set(false))
      )
      .subscribe({
        next: (res: UserListResponse) => {
          // Null-safe: backend bisa mengirim body kosong / null.
          const list = Array.isArray(res?.data) ? res.data : [];
          const meta = res?.meta;

          this.users.set(list);
          this.currentPage.set(meta?.page ?? page);
          this.totalPages.set(meta?.total_pages ?? 1);
        },
        error: (err: HttpErrorResponse) => {
          this.users.set([]);
          this.currentPage.set(1);
          this.totalPages.set(1);

          if (err.status === 403) {
            this.errorMessage.set(
              'Akses ditolak: memerlukan hak akses Super Admin.'
            );
          } else if (err.status === 0) {
            this.errorMessage.set(
              'Tidak dapat menghubungi server. Periksa koneksi Anda.'
            );
          } else {
            this.errorMessage.set(
              err?.error?.error?.message || 'Gagal memuat data pengguna.'
            );
          }
        }
      });
  }

  onSearchEnter(): void {
    this.loadUsers(1);
  }

  openCreateModal(): void {
    this.createEmail = '';
    this.createName = '';
    this.errorMessage.set(null);
    this.successMessage.set(null);
    this.showCreateModal.set(true);
  }

  submitCreateUser(): void {
    const email = this.createEmail.trim();
    const name = this.createName.trim();

    if (!email || !name) {
      this.errorMessage.set('Email dan nama wajib diisi.');
      return;
    }

    this.createSubmitting.set(true);
    this.errorMessage.set(null);
    this.successMessage.set(null);

    this.adminUserService
      .createUser(email, name)
      .pipe(
        timeout(15000),
        finalize(() => this.createSubmitting.set(false))
      )
      .subscribe({
        next: (res: CreateUserResponse) => {
          this.showCreateModal.set(false);
          this.createEmail = '';
          this.createName = '';

          const tempPassword = res?.data?.temporary_password;
          if (tempPassword) {
            this.copyState.set('idle');
            this.tempPasswordNotice.set(tempPassword);
          }
          this.successMessage.set(`Pengguna "${name}" berhasil dibuat.`);
          this.loadUsers(1);
        },
        error: (err: HttpErrorResponse) => {
          if (err.status === 409) {
            this.errorMessage.set('Email sudah terdaftar. Gunakan email lain.');
          } else if (err.status === 422) {
            this.errorMessage.set(
              err?.error?.error?.message ||
                'Validasi gagal: email dan nama wajib diisi.'
            );
          } else {
            this.errorMessage.set(
              err?.error?.error?.message || 'Gagal membuat pengguna.'
            );
          }
        }
      });
  }

  openEditModal(user: User): void {
    this.selectedUser.set(user);
    this.editName = user.name;
    this.editIsActive = user.is_active;
    this.errorMessage.set(null);
    this.successMessage.set(null);
  }

  submitEditUser(): void {
    const user = this.selectedUser();
    if (!user) return;

    this.editSubmitting.set(true);
    this.errorMessage.set(null);
    this.successMessage.set(null);

    this.adminUserService
      .updateUser(user.id, { name: this.editName, is_active: this.editIsActive })
      .pipe(
        timeout(15000),
        finalize(() => this.editSubmitting.set(false))
      )
      .subscribe({
        next: () => {
          this.selectedUser.set(null);
          this.successMessage.set(`Pengguna "${user.email}" berhasil diperbarui.`);
          this.loadUsers(this.currentPage());
        },
        error: (err: HttpErrorResponse) =>
          this.errorMessage.set(
            err?.error?.error?.message || 'Gagal memperbarui pengguna.'
          )
      });
  }

  confirmResetPassword(user: User): void {
    this.errorMessage.set(null);
    this.successMessage.set(null);
    this.pendingReset.set(user);
  }

  resetPassword(): void {
    const user = this.pendingReset();
    if (!user) return;

    this.adminUserService
      .resetPassword(user.id)
      .pipe(timeout(15000))
      .subscribe({
        next: (res: ResetPasswordResponse) => {
          this.pendingReset.set(null);
          const tempPassword = res?.data?.temporary_password;
          if (tempPassword) {
            this.copyState.set('idle');
            this.tempPasswordNotice.set(tempPassword);
          } else {
            this.successMessage.set(res?.data?.message || 'Password berhasil direset.');
          }
        },
        error: (err: HttpErrorResponse) => {
          this.pendingReset.set(null);
          this.errorMessage.set(
            err?.error?.error?.message || 'Gagal mereset password.'
          );
        }
      });
  }

  async copyTempPassword(): Promise<void> {
    const value = this.tempPasswordNotice();
    if (!value) return;
    try {
      await navigator.clipboard.writeText(value);
      this.copyState.set('copied');
    } catch {
      this.copyState.set('failed');
    }
  }
}
