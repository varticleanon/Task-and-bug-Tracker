import { Component, computed, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { CommonModule } from '@angular/common';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { combineLatest, finalize } from 'rxjs';
import { WorkItemService } from '../../../core/api/work-item.service';
import { WorkItem, ItemActivity } from '../../../core/models/item.model';
import { Attachment } from '../../../core/models/attachment.model';
import { WorkspaceService } from '../../../core/services/workspace.service';
import { workspaceIdStream } from '../../../core/helpers/workspace-route.helper';
import { AttachmentGalleryComponent } from '../attachment-gallery/attachment-gallery.component';
import { DialogComponent } from '../../../shared/components/dialog/dialog.component';
import { EmptyStateComponent } from '../../../shared/components/empty-state/empty-state.component';
import { ErrorStateComponent } from '../../../shared/components/error-state/error-state.component';
import { IconComponent } from '../../../shared/components/icon/icon.component';
import { StatusChipComponent } from '../../../shared/components/status-chip/status-chip.component';

@Component({
  selector: 'app-item-detail',
  standalone: true,
  imports: [
    CommonModule,
    RouterLink,
    AttachmentGalleryComponent,
    DialogComponent,
    EmptyStateComponent,
    ErrorStateComponent,
    IconComponent,
    StatusChipComponent
  ],
  templateUrl: './item-detail.component.html',
  styleUrls: ['./item-detail.component.css']
})
export class ItemDetailComponent {
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly itemService = inject(WorkItemService);
  private readonly ws = inject(WorkspaceService);

  readonly workspaceId = signal<string>('');
  readonly itemId = signal<string>('');
  readonly item = signal<WorkItem | null>(null);
  readonly activities = signal<ItemActivity[]>([]);
  readonly activeTab = signal<'detail' | 'attachments' | 'activity'>('detail');
  readonly loading = signal<boolean>(false);
  readonly deleting = signal<boolean>(false);
  readonly errorMessage = signal<string | null>(null);
  readonly showDeleteConfirm = signal<boolean>(false);
  readonly notFound = signal<boolean>(false);

  readonly attachments = computed<Attachment[]>(() => this.item()?.attachments ?? []);
  readonly canWrite = this.ws.canWrite;
  readonly canDelete = this.ws.canDelete;

  // Kunci cache agar detail item tidak di-load ulang untuk params yang sama.
  private loadedKey = '';

  constructor() {
    combineLatest([
      workspaceIdStream(this.route, () => this.ws.activeWorkspaceId()),
      this.route.paramMap
    ])
      .pipe(takeUntilDestroyed())
      .subscribe(([wid, params]) => {
        if (!wid) {
          this.notFound.set(true);
          this.loading.set(false);
          this.errorMessage.set('Workspace ID tidak ditemukan dalam URL.');
          return;
        }
        if (wid !== this.workspaceId()) {
          this.workspaceId.set(wid);
        }

        const id = params.get('itemId');
        if (!id) return;

        this.itemId.set(id);
        this.notFound.set(false);
        const key = `${wid}:${id}`;
        if (key !== this.loadedKey) {
          this.loadedKey = key;
          this.loadItemDetail();
          this.loadActivities();
        }
      });
  }

  loadItemDetail(): void {
    const wid = this.workspaceId();
    const id = this.itemId();
    if (!wid || !id) return;

    this.loading.set(true);
    this.errorMessage.set(null);

    this.itemService
      .getItem(wid, id)
      .pipe(finalize(() => this.loading.set(false)))
      .subscribe({
        next: res => this.item.set(res.data),
        error: err => {
          if (err?.status === 404) {
            this.notFound.set(true);
            return;
          }
          this.errorMessage.set(
            err?.error?.error?.message || 'Gagal memuat detail item.'
          );
        }
      });
  }

  loadActivities(): void {
    const wid = this.workspaceId();
    const id = this.itemId();
    if (!wid || !id) return;

    this.itemService.getItemActivities(wid, id).subscribe({
      next: res => this.activities.set(res.data || []),
      error: () => {
        // Riwayat aktivitas tidak menghalangi tampilan utama.
      }
    });
  }

  onAttachmentChanged(): void {
    this.loadItemDetail();
  }

  confirmDelete(): void {
    this.showDeleteConfirm.set(true);
  }

  cancelDelete(): void {
    this.showDeleteConfirm.set(false);
  }

  executeDelete(): void {
    const wid = this.workspaceId();
    const id = this.itemId();

    this.deleting.set(true);
    this.itemService
      .deleteItem(wid, id)
      .pipe(finalize(() => this.deleting.set(false)))
      .subscribe({
        next: () => this.router.navigate(['/app/w', wid, 'items']),
        error: err => {
          this.showDeleteConfirm.set(false);
          this.errorMessage.set(
            err?.error?.error?.message || 'Gagal menghapus item.'
          );
        }
      });
  }

  backToItems(): void {
    const wid = this.workspaceId();
    this.router.navigate(wid ? ['/app/w', wid, 'items'] : ['/app/workspaces']);
  }

  getAssigneeName(userId: string | null): string {
    return this.ws.getMemberName(userId);
  }

  isOverdue(item: WorkItem): boolean {
    if (!item.due_date || item.status === 'DONE') return false;
    return item.due_date < new Date().toISOString().slice(0, 10);
  }

  formatDetails(details: ItemActivity['details']): string {
    if (!details) return '';
    if (typeof details === 'string') return details;
    try {
      return JSON.stringify(details, null, 2);
    } catch {
      return '';
    }
  }
}
