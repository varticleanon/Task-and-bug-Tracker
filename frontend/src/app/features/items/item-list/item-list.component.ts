import { Component, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Params, Router, RouterLink } from '@angular/router';
import { Subject, combineLatest } from 'rxjs';
import { debounceTime, distinctUntilChanged } from 'rxjs/operators';
import { finalize } from 'rxjs';
import { WorkItemService } from '../../../core/api/work-item.service';
import { WorkItem, ItemFilterParams } from '../../../core/models/item.model';
import { WorkspaceService } from '../../../core/services/workspace.service';
import { workspaceIdStream } from '../../../core/helpers/workspace-route.helper';
import { IconComponent } from '../../../shared/components/icon/icon.component';
import { StatusChipComponent } from '../../../shared/components/status-chip/status-chip.component';
import { EmptyStateComponent } from '../../../shared/components/empty-state/empty-state.component';
import { ErrorStateComponent } from '../../../shared/components/error-state/error-state.component';

@Component({
  selector: 'app-item-list',
  standalone: true,
  imports: [
    CommonModule,
    FormsModule,
    RouterLink,
    IconComponent,
    StatusChipComponent,
    EmptyStateComponent,
    ErrorStateComponent
  ],
  templateUrl: './item-list.component.html',
  styleUrls: ['./item-list.component.css']
})
export class ItemListComponent {
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly itemService = inject(WorkItemService);
  private readonly ws = inject(WorkspaceService);

  readonly workspaceId = signal<string>('');
  readonly items = signal<WorkItem[]>([]);
  readonly loading = signal<boolean>(false);
  readonly error = signal<string | null>(null);

  readonly page = signal<number>(1);
  readonly pageSize = signal<number>(20);
  readonly total = signal<number>(0);
  readonly totalPages = signal<number>(0);

  readonly filterType = signal<string>('');
  readonly filterStatus = signal<string>('');
  readonly filterPriority = signal<string>('');
  readonly filterAssigneeId = signal<string>('');
  readonly filterUnassigned = signal<boolean>(false);
  readonly filterDueBefore = signal<string>('');
  readonly searchQuery = signal<string>('');

  readonly canWrite = this.ws.canWrite;
  readonly members = this.ws.workspaceMembers;

  private readonly searchSubject = new Subject<string>();

  constructor() {
    combineLatest([
      workspaceIdStream(this.route, () => this.ws.activeWorkspaceId()),
      this.route.queryParams
    ])
      .pipe(takeUntilDestroyed())
      .subscribe(([wid, queryParams]) => {
        if (!wid) {
          this.loading.set(false);
          this.error.set('Workspace ID tidak ditemukan dalam URL.');
          return;
        }

        this.applyFilters(queryParams);

        if (wid !== this.workspaceId()) {
          this.workspaceId.set(wid);
        }
        this.fetchItems();
      });

    this.searchSubject
      .pipe(debounceTime(300), distinctUntilChanged(), takeUntilDestroyed())
      .subscribe(query => this.updateQueryParams({ search: query || null, page: 1 }));
  }

  onSearchInput(value: string): void {
    this.searchQuery.set(value);
    this.searchSubject.next(value);
  }

  onFilterChange(): void {
    this.updateQueryParams({
      type: this.filterType() || null,
      status: this.filterStatus() || null,
      priority: this.filterPriority() || null,
      assignee_id: this.filterAssigneeId() || null,
      unassigned: this.filterUnassigned() ? 'true' : null,
      due_before: this.filterDueBefore() || null,
      page: 1
    });
  }

  resetFilters(): void {
    this.filterType.set('');
    this.filterStatus.set('');
    this.filterPriority.set('');
    this.filterAssigneeId.set('');
    this.filterUnassigned.set(false);
    this.filterDueBefore.set('');
    this.searchQuery.set('');
    this.page.set(1);

    this.router.navigate([], {
      relativeTo: this.route,
      queryParams: {}
    });
  }

  goToPage(newPage: number): void {
    if (newPage >= 1 && newPage <= this.totalPages()) {
      this.updateQueryParams({ page: newPage });
    }
  }

  onPageSizeChange(size: number): void {
    this.updateQueryParams({ page_size: size, page: 1 });
  }

  getAssigneeName(userId: string | null): string {
    return this.ws.getMemberName(userId);
  }

  isOverdue(item: WorkItem): boolean {
    if (!item.due_date || item.status === 'DONE') return false;
    return item.due_date < this.today();
  }

  firstShown(): number {
    return this.total() === 0 ? 0 : (this.page() - 1) * this.pageSize() + 1;
  }

  lastShown(): number {
    return Math.min(this.page() * this.pageSize(), this.total());
  }

  private applyFilters(queryParams: Params): void {
    this.filterType.set(queryParams['type'] || '');
    this.filterStatus.set(queryParams['status'] || '');
    this.filterPriority.set(queryParams['priority'] || '');
    this.filterAssigneeId.set(queryParams['assignee_id'] || '');
    this.filterUnassigned.set(queryParams['unassigned'] === 'true');
    this.searchQuery.set(queryParams['search'] || '');
    this.page.set(queryParams['page'] ? parseInt(queryParams['page'], 10) : 1);
    this.pageSize.set(
      queryParams['page_size'] ? parseInt(queryParams['page_size'], 10) : 20
    );

    // Flag 'overdue' dari dashboard dipetakan ke batas tenggat hari ini.
    this.filterDueBefore.set(
      queryParams['overdue'] === 'true'
        ? this.today()
        : queryParams['due_before'] || ''
    );
  }

  private today(): string {
    return new Date().toISOString().slice(0, 10);
  }

  fetchItems(): void {
    const wid = this.workspaceId();
    if (!wid) return;

    this.loading.set(true);
    this.error.set(null);

    const filters: ItemFilterParams = {
      type: this.filterType() || undefined,
      status: this.filterStatus() || undefined,
      priority: this.filterPriority() || undefined,
      assignee_id: this.filterAssigneeId() || undefined,
      unassigned: this.filterUnassigned() || undefined,
      search: this.searchQuery() || undefined,
      due_before: this.filterDueBefore() || undefined,
      page: this.page(),
      page_size: this.pageSize()
    };

    this.itemService
      .getItems(wid, filters)
      .pipe(finalize(() => this.loading.set(false)))
      .subscribe({
        next: res => {
          this.items.set(res.data || []);
          this.total.set(res.meta?.total || 0);
          this.totalPages.set(res.meta?.total_pages || 0);
        },
        error: err =>
          this.error.set(
            err?.error?.error?.message || 'Gagal memuat daftar item. Silakan coba lagi.'
          )
      });
  }

  private updateQueryParams(params: Record<string, string | number | null>): void {
    this.router.navigate([], {
      relativeTo: this.route,
      queryParams: params,
      queryParamsHandling: 'merge'
    });
  }
}
