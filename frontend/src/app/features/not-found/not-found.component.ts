import { Component, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterLink } from '@angular/router';
import { AuthService } from '../../core/services/auth.service';
import { IconComponent } from '../../shared/components/icon/icon.component';

@Component({
  selector: 'app-not-found',
  standalone: true,
  imports: [CommonModule, RouterLink, IconComponent],
  template: `
    <main class="auth-screen">
      <div class="not-found-card">
        <p class="not-found-code">404</p>
        <h1 class="auth-title">Halaman tidak ditemukan</h1>
        <p class="auth-subtitle">
          Halaman yang Anda cari tidak tersedia atau Anda tidak memiliki akses ke sana.
        </p>
        <div class="not-found-actions">
          @if (auth.isAuthenticated) {
            <a routerLink="/app/workspaces" class="btn btn-primary">
              <app-icon name="folder" [size]="16" />
              Kembali ke workspace
            </a>
          } @else {
            <a routerLink="/login" class="btn btn-primary">
              <app-icon name="logout" [size]="16" />
              Kembali ke halaman masuk
            </a>
          }
        </div>
      </div>
    </main>
  `,
  styles: `
    .not-found-card {
      width: 100%;
      max-width: 420px;
      padding: var(--space-8);
      text-align: center;
      background: var(--surface);
      border: 1px solid var(--line);
      border-radius: var(--radius-lg);
    }

    .not-found-code {
      font-size: 3rem;
      font-weight: 700;
      line-height: 1;
      color: var(--accent-text);
      font-variant-numeric: tabular-nums;
      margin-bottom: var(--space-3);
    }

    .not-found-actions {
      display: flex;
      justify-content: center;
      margin-top: var(--space-6);
    }
  `
})
export class NotFoundComponent {
  protected readonly auth = inject(AuthService);
}
