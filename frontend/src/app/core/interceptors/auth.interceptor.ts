import { inject } from '@angular/core';
import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { Router } from '@angular/router';
import { catchError, throwError } from 'rxjs';
import { AuthService } from '../services/auth.service';

/**
 * authInterceptor menangani kegagalan sesi secara global:
 * - 401: sesi tidak valid atau kedaluwarsa, kembali ke /login.
 * - 403 MUST_CHANGE_PASSWORD: paksa ke /change-password.
 */
export const authInterceptor: HttpInterceptorFn = (req, next) => {
  const auth = inject(AuthService);
  const router = inject(Router);

  return next(req).pipe(
    catchError((error: HttpErrorResponse) => {
      if (error.status === 401) {
        const isLoginRequest = req.url.endsWith('/auth/login');
        const isLogoutRequest = req.url.endsWith('/auth/logout');

        if (!isLoginRequest && !isLogoutRequest) {
          auth.markSessionInvalid();
          const currentUrl = router.url;
          if (!currentUrl.startsWith('/login')) {
            router.navigate(['/login'], { queryParams: { returnUrl: currentUrl } });
          }
        }
      } else if (
        error.status === 403 &&
        error.error?.error?.code === 'MUST_CHANGE_PASSWORD'
      ) {
        if (!router.url.startsWith('/change-password')) {
          router.navigate(['/change-password']);
        }
      }

      return throwError(() => error);
    })
  );
};
