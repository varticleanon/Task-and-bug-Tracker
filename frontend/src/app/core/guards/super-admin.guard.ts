import { inject } from '@angular/core';
import { CanActivateFn, Router, UrlTree } from '@angular/router';
import { AuthService } from '../services/auth.service';

/**
 * superAdminGuard adalah pembatas UX untuk halaman administrasi global.
 * Otorisasi final tetap dilakukan backend pada setiap endpoint.
 */
export const superAdminGuard: CanActivateFn = (): boolean | UrlTree => {
  const auth = inject(AuthService);
  const router = inject(Router);

  if (!auth.isAuthenticated) {
    return router.createUrlTree(['/login']);
  }

  if (auth.isSuperAdmin) {
    return true;
  }

  return router.createUrlTree(['/app/workspaces']);
};
