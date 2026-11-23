import { Component, computed, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router } from '@angular/router';
import { finalize, timeout } from 'rxjs';
import { DashboardService, DashboardSummary } from '../../core/api/dashboard.service';
import { AuthService } from '../../core/services/auth.service';
import { WorkspaceService } from '../../core/services/workspace.service';
import { workspaceIdStream } from '../../core/helpers/workspace-route.helper';
import { IconComponent } from '../../shared/components/icon/icon.component';
import { EmptyStateComponent } from '../../shared/components/empty-state/empty-state.component';
import { ErrorStateComponent } from '../../shared/components/error-state/error-state.component';

interface Stage {
  key: string;
  label: string;
  value: number;
  tone: string;
}

@Component({
  selector: 'app-dashboard',
  standalone: true,
  imports: [IconComponent, EmptyStateComponent, ErrorStateComponent],
  templateUrl: './dashboard.component.html',
  styleUrls: ['./dashboard.component.css']
})
export class DashboardComponent {
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly dashboardService = inject(DashboardService);
  protected readonly auth = inject(AuthService);
  protected readonly ws = inject(WorkspaceService);

  readonly workspaceId = signal<string>('');
  readonly summary = signal<DashboardSummary | null>(null);
  readonly loading = signal<boolean>(true);
  readonly error = signal<string | null>(null);

  readonly currentUserId = computed(() => this.auth.currentUser()?.id ?? null);
  readonly canWrite = computed(() => this.ws.canWrite() ?? false);
  readonly workspaceName = computed(() => this.ws.currentWorkspace()?.name ?? 'Dashboard Workspace');

  readonly stages = computed<Stage[]>(() => {
    const s = this.summary();
    if (!s) return [];
    return [
      { key: 'TODO', label: 'Belum mulai', value: s.todo, tone: 'todo' },
      { key: 'IN_PROGRESS', label: 'Dikerjakan', value: s.in_progress, tone: 'progress' },
      { key: 'IN_REVIEW', label: 'Ditinjau', value: s.in_review ?? 0, tone: 'review' },
      { key: 'DONE', label: 'Selesai', value: s.done, tone: 'done' }
    ];
  });

  readonly hasAttention = computed(() => {
    const s = this.summary();
    return !!s && (s.blocked > 0 || s.overdue > 0);
  });

  readonly isEmpty = computed(() => {
    const s = this.summary();
    if (!s) return false;
    return s.total_open === 0 && s.done === 0 && s.bugs === 0 && s.tasks === 0;
  });

  constructor() {
    workspaceIdStream(this.route, () => this.ws.activeWorkspaceId())
      .pipe(takeUntilDestroyed())
      .subscribe(wid => {
        if (!wid) {
          this.loading.set(false);
          this.error.set('Workspace ID tidak ditemukan dalam URL.');
          return;
        }
        if (wid !== this.workspaceId()) {
          this.workspaceId.set(wid);
          this.loadDashboard(wid);
        }
      });
  }

  loadDashboard(wid: string): void {
    if (!wid) return;
    this.loading.set(true);
    this.error.set(null);

    this.dashboardService
      .getDashboardSummary(wid)
      .pipe(
        timeout(15000),
        finalize(() => this.loading.set(false))
      )
      .subscribe({
        next: res => this.summary.set(res.data),
        error: err =>
          this.error.set(
            err?.error?.error?.message || 'Gagal memuat ringkasan dashboard.'
          )
      });
  }

  openItems(queryParams: Record<string, string>): void {
    const wid = this.workspaceId();
    if (!wid) {
      this.router.navigate(['/app/workspaces']);
      return;
    }
    this.router.navigate(['/app/w', wid, 'items'], { queryParams });
  }

  openAssignedToMe(): void {
    const uid = this.currentUserId();
    this.openItems(uid ? { assignee_id: uid } : {});
  }

  goToNewItem(): void {
    const wid = this.workspaceId();
    if (wid) {
      this.router.navigate(['/app/w', wid, 'items', 'new']);
    }
  }

  goToWorkspaces(): void {
    this.router.navigate(['/app/workspaces']);
  }
}
