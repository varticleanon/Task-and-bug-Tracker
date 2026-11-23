import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { normalizeWorkspaceId } from '../helpers/workspace-route.helper';

/**
 * Menolak navigasi ke /app/w/:workspaceId bila parameter kosong, "undefined",
 * atau "null" supaya request ke /api/v1/workspaces/undefined/... tidak pernah terjadi.
 */
export const workspaceIdGuard: CanActivateFn = route => {
  const router = inject(Router);
  const id = normalizeWorkspaceId(route.paramMap.get('workspaceId'));
  return id ? true : router.createUrlTree(['/app/workspaces']);
};
