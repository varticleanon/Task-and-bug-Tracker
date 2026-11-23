import { Injectable, signal, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Router } from '@angular/router';
import { Observable, tap } from 'rxjs';
import { Workspace, WorkspaceMember } from '../models/workspace.model';
import { AuthService } from './auth.service';
import { normalizeWorkspaceId } from '../helpers/workspace-route.helper';
import { environment } from '../../../environments/environment';

export type WorkspaceRole = 'SUPER_ADMIN' | 'ADMIN' | 'MEMBER' | 'VIEWER' | null;

/**
 * WorkspaceService menyimpan state workspace aktif, anggota, dan hak akses efektif
 * pengguna saat ini. Backend tetap otoritas final; ini hanya untuk UX.
 */
@Injectable({
  providedIn: 'root'
})
export class WorkspaceService {
  private baseUrl = `${environment.apiUrl}/workspaces`;

  constructor(
    private http: HttpClient,
    private router: Router,
    private auth: AuthService
  ) {}

  readonly currentWorkspace = signal<Workspace | null>(null);
  readonly activeWorkspaceId = signal<string>('');
  readonly workspaceMembers = signal<WorkspaceMember[]>([]);
  readonly userWorkspaces = signal<Workspace[]>([]);
  readonly loadingContext = signal<boolean>(false);
  readonly contextError = signal<string | null>(null);

  /**
   * Penanda urutan permintaan konteks. Setiap panggilan loadContext menaikkan
   * angka ini, sehingga respons dari workspace lama yang tertinggal tidak
   * menimpa state workspace yang sedang aktif.
   */
  private contextRequestId = 0;

  /** Peran pengguna saat ini di workspace aktif. */
  readonly currentRole = computed<WorkspaceRole>(() => {
    const user = this.auth.currentUser();
    if (!user) return null;
    if (user.global_role === 'SUPER_ADMIN') return 'SUPER_ADMIN';
    const member = this.workspaceMembers().find(m => m.user_id === user.id);
    return (member?.role as WorkspaceRole) ?? null;
  });

  /** Boleh membuat/mengubah item (ADMIN, MEMBER, SUPER_ADMIN) di workspace aktif. */
  readonly canWrite = computed(
    () =>
      !this.currentWorkspace()?.archived_at &&
      (this.currentRole() === 'ADMIN' ||
        this.currentRole() === 'MEMBER' ||
        this.currentRole() === 'SUPER_ADMIN')
  );

  /** Boleh mengelola anggota & pengaturan workspace (ADMIN, SUPER_ADMIN). */
  readonly canManage = computed(
    () =>
      !this.currentWorkspace()?.archived_at &&
      (this.currentRole() === 'ADMIN' || this.currentRole() === 'SUPER_ADMIN')
  );

  /** Boleh menghapus item (ADMIN, SUPER_ADMIN). */
  readonly canDelete = computed(
    () =>
      !this.currentWorkspace()?.archived_at &&
      (this.currentRole() === 'ADMIN' || this.currentRole() === 'SUPER_ADMIN')
  );

  getWorkspaces(): Observable<{ data: Workspace[] }> {
    return this.http.get<{ data: Workspace[] }>(this.baseUrl).pipe(
      tap(res => this.userWorkspaces.set(res.data))
    );
  }

  getWorkspace(id: string): Observable<{ data: Workspace }> {
    return this.http.get<{ data: Workspace }>(`${this.baseUrl}/${id}`);
  }

  createWorkspace(
    name: string,
    slug: string,
    description: string
  ): Observable<{ data: Workspace }> {
    return this.http.post<{ data: Workspace }>(this.baseUrl, {
      name,
      slug,
      description
    });
  }

  updateWorkspace(
    id: string,
    data: { name?: string; slug?: string; description?: string }
  ): Observable<{ data: Workspace }> {
    return this.http
      .patch<{ data: Workspace }>(`${this.baseUrl}/${id}`, data)
      .pipe(tap(res => this.currentWorkspace.set(res.data)));
  }

  archiveWorkspace(id: string): Observable<{ message: string }> {
    return this.http.post<{ message: string }>(`${this.baseUrl}/${id}/archive`, {});
  }

  restoreWorkspace(id: string): Observable<{ message: string }> {
    return this.http.post<{ message: string }>(`${this.baseUrl}/${id}/restore`, {});
  }

  getMembers(workspaceId: string): Observable<{ data: WorkspaceMember[] }> {
    return this.http.get<{ data: WorkspaceMember[] }>(
      `${this.baseUrl}/${workspaceId}/members`
    );
  }

  addMember(
    workspaceId: string,
    email: string,
    role: 'ADMIN' | 'MEMBER' | 'VIEWER'
  ): Observable<{ message: string }> {
    return this.http.post<{ message: string }>(
      `${this.baseUrl}/${workspaceId}/members`,
      { email, role }
    );
  }

  updateMemberRole(
    workspaceId: string,
    userId: string,
    role: 'ADMIN' | 'MEMBER' | 'VIEWER'
  ): Observable<{ message: string }> {
    return this.http.patch<{ message: string }>(
      `${this.baseUrl}/${workspaceId}/members/${userId}`,
      { role }
    );
  }

  removeMember(workspaceId: string, userId: string): Observable<void> {
    return this.http.delete<void>(
      `${this.baseUrl}/${workspaceId}/members/${userId}`
    );
  }

  /**
   * Memuat workspace + anggota sekaligus. Dipakai layout workspace
   * setiap kali parameter workspaceId berubah.
   */
  loadContext(workspaceId: string): void {
    if (!workspaceId) return;

    // Naikkan penanda supaya respons sebelumnya (workspace lama) diabaikan.
    const requestId = ++this.contextRequestId;

    this.loadingContext.set(true);
    this.contextError.set(null);
    // Bersihkan anggota lama supaya peran tidak sempat salah selama memuat.
    this.workspaceMembers.set([]);

    this.getWorkspace(workspaceId).subscribe({
      next: wsRes => {
        if (requestId !== this.contextRequestId) return;
        this.currentWorkspace.set(wsRes.data);
        this.getMembers(workspaceId).subscribe({
          next: memberRes => {
            if (requestId !== this.contextRequestId) return;
            this.workspaceMembers.set(memberRes.data);
            this.loadingContext.set(false);
          },
          error: err => {
            if (requestId !== this.contextRequestId) return;
            this.loadingContext.set(false);
            this.contextError.set(
              err?.error?.error?.message || 'Gagal memuat anggota workspace.'
            );
          }
        });
      },
      error: err => {
        if (requestId !== this.contextRequestId) return;
        this.loadingContext.set(false);
        this.contextError.set(
          err?.error?.error?.message || 'Workspace tidak ditemukan atau tidak dapat diakses.'
        );
      }
    });
  }

  /** Nama anggota berdasarkan user_id (untuk kolom PIC). */
  getMemberName(userId: string | null): string {
    if (!userId) return '-';
    const member = this.workspaceMembers().find(m => m.user_id === userId);
    return member?.name ?? 'Pengguna';
  }

  /** SSOT §5.1: pindah workspace, reset cache lokal. */
  switchWorkspace(workspace: Workspace): void {
    const id = normalizeWorkspaceId(workspace?.id);
    if (!id) return;
    // Batalkan respons context workspace sebelumnya yang masih dalam penerbangan.
    this.contextRequestId++;
    this.activeWorkspaceId.set(id);
    this.contextError.set(null);
    this.currentWorkspace.set(workspace);
    // currentRole adalah computed dari workspaceMembers, jadi mengosongkan
    // anggota langsung meniadakan peran lama. Layout tujuan memanggil
    // loadContext() untuk mengisi ulang anggota workspace yang baru.
    this.workspaceMembers.set([]);
    this.router.navigate(['/app/w', id, 'dashboard']);
  }
}
