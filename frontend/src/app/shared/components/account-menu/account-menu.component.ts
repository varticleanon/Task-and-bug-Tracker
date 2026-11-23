import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  HostListener,
  ViewChild,
  computed,
  inject,
  signal
} from '@angular/core';
import { Router } from '@angular/router';
import { AuthService } from '../../../core/services/auth.service';
import { WorkspaceService } from '../../../core/services/workspace.service';
import { IconComponent } from '../icon/icon.component';

const ROLE_LABELS: Record<string, string> = {
  SUPER_ADMIN: 'Super Admin',
  ADMIN: 'Admin',
  MEMBER: 'Member',
  VIEWER: 'Viewer'
};

/**
 * Akun pengguna di ujung kanan topbar. Peran di workspace aktif ikut ditampilkan
 * supaya perpindahan workspace tetap terlihat tanpa harus membuka sidebar.
 */
@Component({
  selector: 'app-account-menu',
  standalone: true,
  imports: [IconComponent],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="account" #root>
      <button
        class="account-trigger"
        type="button"
        aria-haspopup="menu"
        [attr.aria-expanded]="open()"
        [attr.aria-label]="triggerLabel()"
        (click)="toggle()"
      >
        <span class="account-avatar" aria-hidden="true">{{ initial() }}</span>
        <span class="account-name">{{ auth.currentUser()?.name }}</span>
        <app-icon name="chevron-down" [size]="16" />
      </button>

      @if (open()) {
        <div class="account-menu" role="menu" aria-label="Menu akun">
          <div class="account-meta">
            <div class="account-meta-name">{{ auth.currentUser()?.name }}</div>
            <div class="account-meta-role">{{ roleLabel() }}</div>
          </div>
          <button class="account-logout" type="button" role="menuitem" (click)="logout()">
            <app-icon name="logout" [size]="16" />
            Keluar
          </button>
        </div>
      }
    </div>
  `
})
export class AccountMenuComponent {
  protected readonly auth = inject(AuthService);
  private readonly ws = inject(WorkspaceService);
  private readonly router = inject(Router);

  @ViewChild('root') private root?: ElementRef<HTMLElement>;

  readonly open = signal(false);

  readonly initial = computed(() => {
    const name = this.auth.currentUser()?.name?.trim();
    return name ? name.charAt(0).toUpperCase() : '?';
  });

  readonly triggerLabel = computed(() => `Akun: ${this.auth.currentUser()?.name ?? 'Pengguna'}`);

  readonly roleLabel = computed(() => {
    const role = this.ws.currentRole();
    if (role) return ROLE_LABELS[role] ?? 'Pengguna';
    return this.auth.currentUser()?.global_role === 'SUPER_ADMIN'
      ? 'Super Admin'
      : 'Pengguna';
  });

  toggle(): void {
    this.open.update(value => !value);
  }

  close(): void {
    this.open.set(false);
  }

  @HostListener('document:click', ['$event'])
  onDocumentClick(event: MouseEvent): void {
    if (this.open() && this.root && !this.root.nativeElement.contains(event.target as Node)) {
      this.close();
    }
  }

  @HostListener('document:keydown.escape')
  onEscape(): void {
    this.close();
  }

  logout(): void {
    this.close();
    this.auth.logout().subscribe({
      next: () => this.router.navigate(['/login']),
      error: () => this.router.navigate(['/login'])
    });
  }
}
