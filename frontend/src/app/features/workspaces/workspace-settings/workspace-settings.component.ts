import { Component, computed, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpErrorResponse } from '@angular/common/http';
import { ActivatedRoute, Router } from '@angular/router';
import { finalize, timeout } from 'rxjs';
import { AuthService } from '../../../core/services/auth.service';
import { WorkspaceService } from '../../../core/services/workspace.service';
import { workspaceIdStream } from '../../../core/helpers/workspace-route.helper';
import { DialogComponent } from '../../../shared/components/dialog/dialog.component';
import { ErrorStateComponent } from '../../../shared/components/error-state/error-state.component';
import { IconComponent } from '../../../shared/components/icon/icon.component';

@Component({
  selector: 'app-workspace-settings',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    DialogComponent,
    ErrorStateComponent,
    IconComponent
  ],
  templateUrl: './workspace-settings.component.html',
  styleUrls: ['./workspace-settings.component.css']
})
export class WorkspaceSettingsComponent {
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  protected readonly ws = inject(WorkspaceService);
  private readonly auth = inject(AuthService);

  readonly workspaceId = signal<string>('');
  readonly name = signal<string>('');
  readonly slug = signal<string>('');
  readonly description = signal<string>('');

  readonly loading = signal<boolean>(true);
  readonly saving = signal<boolean>(false);
  readonly errorMessage = signal<string | null>(null);
  readonly successMessage = signal<string | null>(null);
  readonly showArchiveConfirm = signal<boolean>(false);

  readonly canManage = this.ws.canManage;
  readonly isSuperAdmin = computed(() => this.auth.isSuperAdmin);

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
          this.loadWorkspace(wid);
        }
      });
  }

  save(): void {
    const wid = this.workspaceId();
    if (!wid) {
      this.errorMessage.set('Workspace ID tidak valid.');
      return;
    }

    const trimmedName = this.name().trim();
    const trimmedSlug = this.slug().trim();

    if (trimmedName.length < 2) {
      this.errorMessage.set('Nama workspace minimal 2 karakter.');
      return;
    }
    if (trimmedSlug.length < 3) {
      this.errorMessage.set('Slug minimal 3 karakter.');
      return;
    }

    this.saving.set(true);
    this.errorMessage.set(null);
    this.successMessage.set(null);

    this.ws
      .updateWorkspace(wid, {
        name: trimmedName,
        slug: trimmedSlug,
        description: this.description().trim()
      })
      .pipe(finalize(() => this.saving.set(false)))
      .subscribe({
        next: () => this.successMessage.set('Perubahan workspace berhasil disimpan.'),
        error: (err: HttpErrorResponse) =>
          this.errorMessage.set(
            err?.error?.error?.message || 'Gagal memperbarui workspace.'
          )
      });
  }

  archive(): void {
    const wid = this.workspaceId();
    if (!wid) {
      this.errorMessage.set('Workspace ID tidak valid.');
      return;
    }

    this.ws.archiveWorkspace(wid).subscribe({
      next: () => {
        this.ws.currentWorkspace.update(ws =>
          ws ? { ...ws, archived_at: new Date().toISOString() } : ws
        );
        this.showArchiveConfirm.set(false);
        this.router.navigate(['/app/workspaces']);
      },
      error: (err: HttpErrorResponse) => {
        this.showArchiveConfirm.set(false);
        this.errorMessage.set(
          err?.error?.error?.message || 'Gagal mengarsipkan workspace.'
        );
      }
    });
  }

  restore(): void {
    const wid = this.workspaceId();
    if (!wid) {
      this.errorMessage.set('Workspace ID tidak valid.');
      return;
    }

    this.ws.restoreWorkspace(wid).subscribe({
      next: () => {
        this.ws.currentWorkspace.update(ws =>
          ws ? { ...ws, archived_at: null } : ws
        );
        this.successMessage.set('Workspace berhasil dipulihkan.');
      },
      error: (err: HttpErrorResponse) =>
        this.errorMessage.set(
          err?.error?.error?.message || 'Gagal memulihkan workspace.'
        )
    });
  }

  private loadWorkspace(wid: string): void {
    this.loading.set(true);
    this.errorMessage.set(null);

    this.ws
      .getWorkspace(wid)
      .pipe(
        timeout(15000),
        finalize(() => this.loading.set(false))
      )
      .subscribe({
        next: res => {
          this.name.set(res.data.name);
          this.slug.set(res.data.slug);
          this.description.set(res.data.description || '');
        },
        error: (err: HttpErrorResponse) =>
          this.errorMessage.set(
            err?.error?.error?.message || 'Gagal memuat detail workspace.'
          )
      });
  }
}
