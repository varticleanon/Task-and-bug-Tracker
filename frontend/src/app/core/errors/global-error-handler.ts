import { ErrorHandler, inject, Injectable, NgZone } from '@angular/core';

/**
 * ErrorHandler global.
 *
 * Tanpa handler ini, error runtime saat render komponen diheningkan oleh
 * Angular sehingga halaman hanya menampilkan header dan area konten kosong
 * (blank white) tanpa penjelasan. Handler ini menulis detail error ke
 * console agar terlihat di browser DevTools.
 */
@Injectable()
export class GlobalErrorHandler implements ErrorHandler {
  private readonly zone = inject(NgZone);

  handleError(error: unknown): void {
    this.zone.runOutsideAngular(() => {
      console.error('[app-error]', error);
      if (error instanceof Error && error.stack) {
        console.error(error.stack);
      }
    });
  }
}
