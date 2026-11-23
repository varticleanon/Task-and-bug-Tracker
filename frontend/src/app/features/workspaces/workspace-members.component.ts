import { Component, DestroyRef, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';
import { finalize } from 'rxjs';
import { WorkspaceService } from '../../core/services/workspace.service';
import { workspaceIdStream } from '../../core/helpers/workspace-route.helper';
import { AuthService } from '../../core/services/auth.service';
import { WorkspaceMember } from '../../core/models/workspace.model';
import { DialogComponent } from '../../shared/components/dialog/dialog.component';
import { EmptyStateComponent } from '../../shared/components/empty-state/empty-state.component';
import { ErrorStateComponent } from '../../shared/components/error-state/error-state.component';
import { IconComponent } from '../../shared/components/icon/icon.component';
import { StatusChipComponent } from '../../shared/components/status-chip/status-chip.component';

@Component({
  selector: 'app-workspace-members',
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
  templateUrl: './workspace-members.component.html',
  styleUrls: ['./workspace-members.component.css']
})
export class WorkspaceMembersComponent {
  private readonly route = inject(ActivatedRoute);
  private readonly ws = inject(WorkspaceService);
  private readonly auth = inject(AuthService);
  private readonly destroyRef = inject(DestroyRef);

  readonly workspaceId = signal<string>('');
  readonly members = signal<WorkspaceMember[]>([]);
  readonly errorMessage = signal<string | null>(null);
  readonly successMessage = signal<string | null>(null);
  readonly loading = signal<boolean>(false);

  readonly showAddModal = signal(false);
  readonly addSubmitting = signal<boolean>(false);

  readonly selectedMember = signal<WorkspaceMember | null>(null);
  readonly editSubmitting = signal<boolean>(false);
  readonly pendingRemoval = signal<WorkspaceMember | null>(null);

  addEmail = '';
  addRole: 'ADMIN' | 'MEMBER' | 'VIEWER' = 'MEMBER';
  editRole: 'ADMIN' | 'MEMBER' | 'VIEWER' = 'MEMBER';

  /**
   * Penanda urutan permintaan daftar anggota, supaya respons workspace lama
   * yang tertinggal tidak menimpa daftar workspace yang sedang aktif.
   */
  private membersRequestId = 0;

  readonly canManage = this.ws.canManage;

  constructor() {
    workspaceIdStream(this.route, () => this.ws.activeWorkspaceId())
      .pipe(takeUntilDestroyed())
      .subscribe(wid => {
        if (!wid) {
          this.loading.set(false);
          this.errorMessage.set('Workspace ID tidak ditemukan dalam URL.');
          return;
        }
        if (wid !== this.workspaceId()) {
          this.workspaceId.set(wid);
          this.loadMembers();
        }
      });
  }

  loadMembers(): void {
    const wid = this.workspaceId();
    if (!wid) return;

    const requestId = ++this.membersRequestId;
    this.loading.set(true);
    this.errorMessage.set(null);
    this.members.set([]);

    this.ws
      .getMembers(wid)
      .pipe(finalize(() => this.loading.set(false)), takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: res => {
          if (requestId !== this.membersRequestId) return;
          const data = res.data || [];
          this.members.set(data);
          // Sinkronkan state global supaya peran di sidebar/topbar ikut benar.
          this.ws.workspaceMembers.set(data);
        },
        error: (err: HttpErrorResponse) => {
          if (requestId !== this.membersRequestId) return;
          this.errorMessage.set(
            err.error?.error?.message || 'Gagal memuat daftar anggota workspace.'
          );
        }
      });
  }

  openAddModal(): void {
    this.addEmail = '';
    this.addRole = 'MEMBER';
    this.errorMessage.set(null);
    this.successMessage.set(null);
    this.showAddModal.set(true);
  }

  submitAddMember(): void {
    const wid = this.workspaceId();
    if (!wid) {
      this.errorMessage.set('Workspace ID tidak valid.');
      return;
    }

    const email = this.addEmail.trim();
    if (!email) {
      this.errorMessage.set('Email wajib diisi.');
      return;
    }

    this.addSubmitting.set(true);
    this.errorMessage.set(null);
    this.successMessage.set(null);

    this.ws
      .addMember(wid, email, this.addRole)
      .pipe(finalize(() => this.addSubmitting.set(false)))
      .subscribe({
        next: () => {
          this.showAddModal.set(false);
          this.successMessage.set('Anggota berhasil ditambahkan.');
          this.loadMembers();
        },
        error: (err: HttpErrorResponse) =>
          this.errorMessage.set(
            err.error?.error?.message ||
              'Gagal menambahkan anggota. Pastikan email sudah terdaftar.'
          )
      });
  }

  openRoleModal(member: WorkspaceMember): void {
    this.selectedMember.set(member);
    this.editRole = member.role;
    this.errorMessage.set(null);
    this.successMessage.set(null);
  }

  submitRoleChange(): void {
    const member = this.selectedMember();
    const wid = this.workspaceId();
    if (!member || !wid) return;

    this.editSubmitting.set(true);
    this.errorMessage.set(null);
    this.successMessage.set(null);

    this.ws
      .updateMemberRole(wid, member.user_id, this.editRole)
      .pipe(finalize(() => this.editSubmitting.set(false)))
      .subscribe({
        next: () => {
          this.selectedMember.set(null);
          this.successMessage.set('Peran anggota diperbarui.');
          this.loadMembers();
        },
        error: (err: HttpErrorResponse) =>
          this.errorMessage.set(
            err.error?.error?.message || 'Gagal memperbarui peran anggota.'
          )
      });
  }

  confirmRemoval(member: WorkspaceMember): void {
    this.errorMessage.set(null);
    this.successMessage.set(null);
    this.pendingRemoval.set(member);
  }

  executeRemoval(): void {
    const member = this.pendingRemoval();
    const wid = this.workspaceId();
    if (!member || !wid) return;

    this.ws.removeMember(wid, member.user_id).subscribe({
      next: () => {
        this.pendingRemoval.set(null);
        this.successMessage.set(`${member.name} dikeluarkan dari workspace.`);
        this.loadMembers();
      },
      error: (err: HttpErrorResponse) => {
        this.pendingRemoval.set(null);
        this.errorMessage.set(
          err.error?.error?.message || 'Gagal mengeluarkan anggota.'
        );
      }
    });
  }

  isSelf(member: WorkspaceMember): boolean {
    return member.user_id === this.auth.currentUser()?.id;
  }
}
