import { inject } from '@angular/core';
import { CanActivateFn, Router, UrlTree } from '@angular/router';
import { AuthService } from '../services/auth.service';
import { User } from '../models/user.model';

/**
 * authGuard memastikan pengguna memiliki sesi valid sebelum masuk area
 * terproteksi. Jika sesi belum dimuat, guard memulihkannya dari cookie
 * HttpOnly melalui GET /auth/me. Pengguna dengan must_change_password
 * dipaksa ke halaman /change-password (sesuai SSOT §4).
 */
export const authGuard: CanActivateFn = () => {
  const auth = inject(AuthService);
  const router = inject(Router);

  const check = (user: User | null): boolean | UrlTree => {
    if (!user) {
      const returnUrl =
        router.getCurrentNavigation()?.finalUrl?.toString() ?? router.url;
      return router.createUrlTree(['/login'], { queryParams: { returnUrl } });
    }

    if (user.must_change_password) {
      const target =
        router.getCurrentNavigation()?.finalUrl?.toString() ?? router.url;
      if (!target.startsWith('/change-password')) {
        return router.createUrlTree(['/change-password']);
      }
    }

    return true;
  };

  if (auth.currentUser()) {
    return check(auth.currentUser());
  }

  return auth.restoreSession().then(() => check(auth.currentUser()));
};
