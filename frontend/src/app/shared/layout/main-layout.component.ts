import {
  AfterViewInit,
  Component,
  ElementRef,
  ViewChild,
  computed,
  inject
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import {
  NavigationEnd,
  Router,
  RouterLink,
  RouterLinkActive,
  RouterOutlet
} from '@angular/router';
import { filter } from 'rxjs';
import { AuthService } from '../../core/services/auth.service';
import { ThemeService } from '../../core/services/theme.service';
import { IconComponent } from '../components/icon/icon.component';
import { AccountMenuComponent } from '../components/account-menu/account-menu.component';

/**
 * Shell global untuk halaman yang TIDAK berada di dalam sebuah workspace
 * (daftar workspace, admin users, admin workspaces).
 *
 * Komponen ini murni presentasional: ia TIDAK mengelola state workspace.
 * Pemilihan workspace ada di halaman daftar workspace (tombol Buka) dan di
 * sidebar workspace aktif, bukan di header.
 */
@Component({
  selector: 'app-main-layout',
  standalone: true,
  imports: [RouterLink, RouterLinkActive, RouterOutlet, IconComponent, AccountMenuComponent],
  template: `
    <div class="app-shell">
      <div class="main-area">
        <header class="topbar topbar--nav">
          <div class="topbar-left">
            <a routerLink="/app/workspaces" class="brand">
              <img class="brand-logo" src="logo.svg" alt="" width="28" height="28" />
              <span class="brand-name truncate">Task &amp; Bug Tracker</span>
            </a>
          </div>

          <nav class="topbar-nav" aria-label="Navigasi global">
            <a
              class="topbar-link"
              routerLink="/app/workspaces"
              routerLinkActive="active"
              [routerLinkActiveOptions]="{ exact: true }"
              title="Daftar workspace"
            >
              Workspace
            </a>
            @if (isSuperAdmin()) {
              <a
                class="topbar-link"
                routerLink="/app/admin/users"
                routerLinkActive="active"
                [routerLinkActiveOptions]="{ exact: true }"
                title="Manajemen pengguna"
              >
                Pengguna
              </a>
              <a
                class="topbar-link"
                routerLink="/app/admin/workspaces"
                routerLinkActive="active"
                [routerLinkActiveOptions]="{ exact: true }"
                title="Manajemen workspace"
              >
                Administrasi
              </a>
            }
          </nav>

          <div class="topbar-right">
            <button
              class="icon-btn"
              type="button"
              [attr.aria-label]="themeLabel()"
              (click)="theme.toggle()"
            >
              <app-icon [name]="theme.theme() === 'dark' ? 'sun' : 'moon'" [size]="18" />
            </button>
            <app-account-menu />
          </div>
        </header>

        <main #content>
          <router-outlet />
        </main>
      </div>
    </div>
  `,
})
export class MainLayoutComponent implements AfterViewInit {
  protected readonly auth = inject(AuthService);
  protected readonly theme = inject(ThemeService);
  private readonly router = inject(Router);

  @ViewChild('content') private contentRef?: ElementRef<HTMLElement>;

  /** Hanya Super Admin yang punya tujuan halaman admin. */
  readonly isSuperAdmin = computed<boolean>(
    () => this.auth.currentUser()?.global_role === 'SUPER_ADMIN'
  );

  ngAfterViewInit(): void {
    this.resetScroll();
  }

  constructor() {
    this.router.events
      .pipe(
        filter((event): event is NavigationEnd => event instanceof NavigationEnd),
        takeUntilDestroyed()
      )
      .subscribe(() => this.resetScroll());
  }

  /**
   * Area konten adalah container scroll sendiri, jadi posisinya tidak lagi
   * ikut direset browser saat halaman berganti. Tanpa ini, pengguna yang
   * sudah menggulir daftar panjang akan mendarat di tengah halaman berikutnya.
   */
  private resetScroll(): void {
    this.contentRef?.nativeElement.scrollTo({ top: 0 });
  }

  themeLabel(): string {
    return this.theme.theme() === 'dark'
      ? 'Beralih ke mode terang'
      : 'Beralih ke mode gelap';
  }

}
