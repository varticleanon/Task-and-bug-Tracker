import { ActivatedRoute, ParamMap } from '@angular/router';
import { Observable, combineLatest, map, distinctUntilChanged } from 'rxjs';

const INVALID_IDS = new Set(['', 'undefined', 'null']);

/**
 * Membersihkan nilai workspaceId: string kosong, "undefined", dan "null"
 * dianggap tidak valid. Selalu mengembalikan UUID yang siap dipakai atau null.
 */
export function normalizeWorkspaceId(
  raw: string | null | undefined
): string | null {
  if (!raw) return null;
  const id = raw.trim();
  return INVALID_IDS.has(id) ? null : id;
}

/**
 * Mencari workspaceId dari seluruh level route (root -> child), lalu
 * fallback ke state workspace aktif di service.
 *
 * Urutan route-first dipakai agar ID selalu mengikuti URL yang sedang
 * dibuka, bukan state lama dari workspace sebelumnya.
 */
export function findWorkspaceId(
  route: ActivatedRoute,
  activeWorkspaceId?: string | null
): string | null {
  for (const current of route.pathFromRoot) {
    const id = normalizeWorkspaceId(
      current.snapshot?.paramMap?.get('workspaceId')
    );
    if (id) return id;
  }
  return normalizeWorkspaceId(activeWorkspaceId);
}

/**
 * Stream reaktif workspaceId.
 *
 * Menggabungkan paramMap seluruh level route sehingga:
 * - tetap bekerja tanpa bergantung pada paramsInheritanceStrategy,
 * - langsung bereaksi saat parameter workspaceId berubah (ganti workspace),
 * - selalu menghasilkan UUID yang valid atau null.
 */
export function workspaceIdStream(
  route: ActivatedRoute,
  activeWorkspaceId?: () => string | null
): Observable<string | null> {
  const sources: Observable<ParamMap>[] = route.pathFromRoot.map(r => r.paramMap);

  return combineLatest(sources).pipe(
    map(maps => {
      for (const params of maps) {
        const id = normalizeWorkspaceId(params.get('workspaceId'));
        if (id) return id;
      }
      return normalizeWorkspaceId(activeWorkspaceId?.());
    }),
    distinctUntilChanged()
  );
}
