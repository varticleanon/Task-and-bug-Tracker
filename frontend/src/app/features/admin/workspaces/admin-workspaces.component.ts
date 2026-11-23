import { Component, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpErrorResponse } from '@angular/common/http';
import { Router } from '@angular/router';
import { WorkspaceService } from '../../../core/services/workspace.service';
import { Workspace } from '../../../core/models/workspace.model';
import { DialogComponent } from '../../../shared/components/dialog/dialog.component';
import { EmptyStateComponent } from '../../../shared/components/empty-state/empty-state.component';
import { ErrorStateComponent } from '../../../shared/components/error-state/error-state.component';
import { IconComponent } from '../../../shared/components/icon/icon.component';

@Component({
  selector: 'app-admin-workspaces',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    DialogComponent,
    EmptyStateComponent,
    ErrorStateComponent,
    IconComponent
  ],
  templateUrl: './admin-workspaces.component.html',
  styleUrls: ['./admin-workspaces.component.css']
})
export class AdminWorkspacesComponent {
  private readonly ws = inject(WorkspaceService);
  private readonly router = inject(Router);

  readonly workspaces = signal<Workspace[]>([]);
  readonly loading = signal<boolean>(true);
  readonly errorMessage = signal<string | null>(null);

  readonly showCreateModal = signal(false);
  readonly pendingArchive = signal<Workspace | null>(null);
  readonly submitting = signal(false);

  newWsName = '';
  newWsSlug = '';
  newWsDescription = '';

  constructor() {
    this.loadWorkspaces();
  }

  loadWorkspaces(): void {
    this.loading.set(true);
    this.errorMessage.set(null);

    this.ws.getWorkspaces().subscribe({
      next: res => {
        this.workspaces.set(res.data || []);
        this.loading.set(false);
      },
      error: (err: HttpErrorResponse) => {
        this.loading.set(false);
        this.errorMessage.set(
          err?.error?.error?.message || 'Gagal memuat daftar workspace.'
        );
      }
    });
  }

  openCreateModal(): void {
    this.newWsName = '';
    this.newWsSlug = '';
    this.newWsDescription = '';
    this.errorMessage.set(null);
    this.showCreateModal.set(true);
  }

  autoSlug(): void {
    this.newWsSlug = this.newWsName
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '');
  }

  submitCreateWorkspace(): void {
    if (this.newWsName.trim().length < 2) {
      this.errorMessage.set('Nama workspace minimal 2 karakter.');
      return;
    }
    if (this.newWsSlug.trim().length < 3) {
      this.errorMessage.set('Slug minimal 3 karakter.');
      return;
    }

    this.submitting.set(true);
    this.errorMessage.set(null);

    this.ws
      .createWorkspace(
        this.newWsName.trim(),
        this.newWsSlug.trim(),
        this.newWsDescription.trim()
      )
      .subscribe({
        next: res => {
          this.submitting.set(false);
          this.showCreateModal.set(false);
          this.router.navigate(['/app/w', res.data.id, 'dashboard']);
        },
        error: (err: HttpErrorResponse) => {
          this.submitting.set(false);
          this.errorMessage.set(
            err?.error?.error?.message || 'Gagal membuat workspace.'
          );
        }
      });
  }

  confirmArchive(ws: Workspace): void {
    this.errorMessage.set(null);
    this.pendingArchive.set(ws);
  }

  executeArchive(): void {
    const target = this.pendingArchive();
    if (!target) return;

    this.ws.archiveWorkspace(target.id).subscribe({
      next: () => {
        this.pendingArchive.set(null);
        this.loadWorkspaces();
      },
      error: (err: HttpErrorResponse) => {
        this.pendingArchive.set(null);
        this.errorMessage.set(
          err?.error?.error?.message || 'Gagal mengarsipkan workspace.'
        );
      }
    });
  }

  restoreWorkspace(ws: Workspace): void {
    this.ws.restoreWorkspace(ws.id).subscribe({
      next: () => this.loadWorkspaces(),
      error: (err: HttpErrorResponse) =>
        this.errorMessage.set(
          err?.error?.error?.message || 'Gagal memulihkan workspace.'
        )
    });
  }

  openWorkspace(ws: Workspace): void {
    this.ws.switchWorkspace(ws);
  }
}
