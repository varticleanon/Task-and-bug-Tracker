import {
  ChangeDetectionStrategy,
  Component,
  computed,
  inject,
  signal
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { HttpErrorResponse } from '@angular/common/http';
import { catchError, of } from 'rxjs';
import { AuthService } from '../../core/services/auth.service';
import { WorkspaceService } from '../../core/services/workspace.service';
import { Workspace } from '../../core/models/workspace.model';
import { DialogComponent } from '../../shared/components/dialog/dialog.component';
import { EmptyStateComponent } from '../../shared/components/empty-state/empty-state.component';
import { ErrorStateComponent } from '../../shared/components/error-state/error-state.component';
import { IconComponent } from '../../shared/components/icon/icon.component';
import { StatusChipComponent } from '../../shared/components/status-chip/status-chip.component';

@Component({
  selector: 'app-workspace-list',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    CommonModule,
    FormsModule,
    DialogComponent,
    EmptyStateComponent,
    ErrorStateComponent,
    IconComponent,
    StatusChipComponent
  ],
  templateUrl: './workspace-list.component.html',
  styleUrls: ['./workspace-list.component.css']
})
export class WorkspaceListComponent {
  private readonly ws = inject(WorkspaceService);
  private readonly auth = inject(AuthService);

  /** Jumlah workspace yang dirender sekaligus. Dataset bisa berisi ribuan
   *  entri, merender semuanya membuat DOM membloki main thread dan halaman
   *  tampak blank. */
  readonly pageSize = 24;

  readonly workspaces = signal<Workspace[]>([]);
  readonly loading = signal<boolean>(true);
  readonly errorMessage = signal<string | null>(null);

  readonly search = signal<string>('');
  readonly page = signal<number>(1);

  readonly showCreateModal = signal(false);
  readonly pendingArchive = signal<Workspace | null>(null);
  readonly submitting = signal(false);

  newWsName = '';
  newWsSlug = '';
  newWsDescription = '';

  readonly canManage = computed(() => this.auth.isSuperAdmin);

  /** Workspace yang cocok dengan kata kunci pencarian. */
  readonly filtered = computed<Workspace[]>(() => {
    const term = this.search().trim().toLowerCase();
    const all = this.workspaces();
    if (!term) return all;
    return all.filter(
      w =>
        (w.name ?? '').toLowerCase().includes(term) ||
        (w.slug ?? '').toLowerCase().includes(term) ||
        (w.description ?? '').toLowerCase().includes(term)
    );
  });

  readonly totalPages = computed<number>(() =>
    Math.max(1, Math.ceil(this.filtered().length / this.pageSize))
  );

  /** Hanya sebagian workspace yang dirender ke DOM. */
  readonly visible = computed<Workspace[]>(() => {
    const start = (this.page() - 1) * this.pageSize;
    return this.filtered().slice(start, start + this.pageSize);
  });

  readonly rangeStart = computed<number>(() =>
    this.filtered().length === 0 ? 0 : (this.page() - 1) * this.pageSize + 1
  );

  readonly rangeEnd = computed<number>(() =>
    Math.min(this.page() * this.pageSize, this.filtered().length)
  );

  constructor() {
    this.loadWorkspaces();
  }

  loadWorkspaces(): void {
    this.loading.set(true);
    this.errorMessage.set(null);

    this.ws
      .getWorkspaces()
      .pipe(
        catchError((err: HttpErrorResponse) => {
          this.errorMessage.set(
            err instanceof HttpErrorResponse
              ? err.error?.error?.message || 'Gagal memuat daftar workspace.'
              : 'Tidak dapat terhubung ke server.'
          );
          return of({ data: [] as Workspace[] });
        })
      )
      .subscribe({
        next: res => {
          const data = (res && res.data) || [];
          this.workspaces.set(Array.isArray(data) ? data : []);
          this.page.set(1);
          this.loading.set(false);
        },
        error: () => {
          this.workspaces.set([]);
          this.loading.set(false);
        }
      });
  }

  onSearchChange(value: string): void {
    this.search.set(value);
    this.page.set(1);
  }

  goToPage(page: number): void {
    const total = this.totalPages();
    if (page < 1 || page > total) return;
    this.page.set(page);
  }

  nextPage(): void {
    this.goToPage(this.page() + 1);
  }

  prevPage(): void {
    this.goToPage(this.page() - 1);
  }

  selectWorkspace(ws: Workspace): void {
    if (!ws || ws.archived_at && !this.canManage()) return;
    this.ws.switchWorkspace(ws);
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
        next: () => {
          this.submitting.set(false);
          this.showCreateModal.set(false);
          this.loadWorkspaces();
        },
        error: (err: HttpErrorResponse) => {
          this.submitting.set(false);
          this.errorMessage.set(
            err instanceof HttpErrorResponse
              ? err.error?.error?.message || 'Gagal membuat workspace.'
              : 'Gagal membuat workspace.'
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
          err instanceof HttpErrorResponse
            ? err.error?.error?.message || 'Gagal mengarsipkan workspace.'
            : 'Gagal mengarsipkan workspace.'
        );
      }
    });
  }

  restoreWorkspace(ws: Workspace): void {
    this.errorMessage.set(null);
    this.ws.restoreWorkspace(ws.id).subscribe({
      next: () => this.loadWorkspaces(),
      error: (err: HttpErrorResponse) =>
        this.errorMessage.set(
          err instanceof HttpErrorResponse
            ? err.error?.error?.message || 'Gagal memulihkan workspace.'
            : 'Gagal memulihkan workspace.'
        )
    });
  }
}
