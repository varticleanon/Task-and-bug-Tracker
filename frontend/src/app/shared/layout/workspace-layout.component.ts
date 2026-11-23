import {
  AfterViewInit,
  Component,
  computed,
  ElementRef,
  HostListener,
  inject,
  signal,
  ViewChild
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import {
  ActivatedRoute,
  NavigationEnd,
  Router,
  RouterLink,
  RouterLinkActive,
  RouterOutlet
} from '@angular/router';
import { filter, map } from 'rxjs';
import { ThemeService } from '../../core/services/theme.service';
import { WorkspaceService } from '../../core/services/workspace.service';
import { Workspace } from '../../core/models/workspace.model';
import {
  findWorkspaceId,
  workspaceIdStream
} from '../../core/helpers/workspace-route.helper';
import { IconComponent } from '../components/icon/icon.component';
import { EmptyStateComponent } from '../components/empty-state/empty-state.component';
import { AccountMenuComponent } from '../components/account-menu/account-menu.component';

@Component({
  selector: 'app-workspace-layout',
  standalone: true,
  imports: [
    RouterOutlet,
    RouterLink,
    RouterLinkActive,
    IconComponent,
    EmptyStateComponent,
    AccountMenuComponent
  ],
  template: `
    <div class="app-shell app-shell--workspace">
      <aside class="sidebar" [class.collapsed]="collapsed()" [class.open]="menuOpen()">
        <div class="sidebar-head">
          <a routerLink="/app/workspaces" class="sidebar-brand">
            <img class="brand-logo" src="logo.svg" alt="" width="28" height="28" />
            <span class="brand-name truncate" [class.sr-only]="collapsed()">Task &amp; Bug Tracker</span>
          </a>
          <button
            class="sidebar-close"
            type="button"
            aria-label="Tutup menu navigasi"
            (click)="menuOpen.set(false)"
          >
            <app-icon name="x" [size]="20" />
          </button>
        </div>

        <div class="sidebar-scroll">
          <nav id="sidebar-nav" class="sidebar-nav" aria-label="Navigasi workspace">
            <a
              class="sidebar-link"
              [routerLink]="['/app/w', workspaceId(), 'dashboard']"
              routerLinkActive="active"
              [routerLinkActiveOptions]="{ exact: true }"
            >
              <span class="link-icon"><app-icon name="dashboard" [size]="18" /></span>
              <span [class.sr-only]="collapsed()">Dashboard</span>
            </a>
            <a
              class="sidebar-link"
              [class.active]="isItemsActive(null)"
              [routerLink]="['/app/w', workspaceId(), 'items']"
            >
              <span class="link-icon"><app-icon name="list" [size]="18" /></span>
              <span [class.sr-only]="collapsed()">Semua Item</span>
            </a>
            <a
              class="sidebar-link"
              [class.active]="isItemsActive('TASK')"
              [routerLink]="['/app/w', workspaceId(), 'items']"
              [queryParams]="{ type: 'TASK' }"
            >
              <span class="link-icon"><app-icon name="task" [size]="18" /></span>
              <span [class.sr-only]="collapsed()">Task</span>
            </a>
            <a
              class="sidebar-link"
              [class.active]="isItemsActive('BUG')"
              [routerLink]="['/app/w', workspaceId(), 'items']"
              [queryParams]="{ type: 'BUG' }"
            >
              <span class="link-icon"><app-icon name="bug" [size]="18" /></span>
              <span [class.sr-only]="collapsed()">Bug</span>
            </a>
            @if (ws.canManage()) {
              <a
                class="sidebar-link"
                [routerLink]="['/app/w', workspaceId(), 'members']"
                routerLinkActive="active"
              >
                <span class="link-icon"><app-icon name="users" [size]="18" /></span>
                <span [class.sr-only]="collapsed()">Anggota</span>
              </a>
              <a
                class="sidebar-link"
                [routerLink]="['/app/w', workspaceId(), 'settings']"
                routerLinkActive="active"
              >
                <span class="link-icon"><app-icon name="settings" [size]="18" /></span>
                <span [class.sr-only]="collapsed()">Pengaturan</span>
              </a>
            }
          </nav>
        </div>

      </aside>

      <button
        class="sidebar-toggle"
        type="button"
        [class.collapsed]="collapsed()"
        aria-controls="sidebar-nav"
        [attr.aria-expanded]="!collapsed()"
        [attr.aria-label]="collapsed() ? 'Buka sidebar' : 'Tutup sidebar'"
        [attr.title]="collapsed() ? 'Buka sidebar' : 'Tutup sidebar'"
        (click)="toggleCollapsed()"
      >
        <app-icon [name]="collapsed() ? 'chevron-right' : 'chevron-left'" [size]="18" />
      </button>

      @if (menuOpen()) {
        <button
          class="drawer-backdrop"
          type="button"
          aria-label="Tutup menu navigasi"
          (click)="menuOpen.set(false)"
        ></button>
      }

      <div class="main-area">
        <header class="topbar">
          <button
            class="nav-toggle"
            type="button"
            aria-label="Buka menu navigasi"
            title="Buka menu navigasi"
            [attr.aria-expanded]="menuOpen()"
            (click)="menuOpen.set(!menuOpen())"
          >
            <app-icon name="menu" [size]="20" />
          </button>

          @if (collapsed()) {
            <button
              class="sidebar-reopen"
              type="button"
              aria-label="Buka sidebar"
              title="Buka sidebar"
              (click)="toggleCollapsed()"
            >
              <app-icon name="menu" [size]="20" />
            </button>
          }

          <div class="topbar-title">
            <label class="sr-only" for="workspace-switcher">Pilih workspace</label>
            <select
              id="workspace-switcher"
              class="workspace-switch"
              aria-label="Pilih workspace"
              [value]="workspaceId()"
              [disabled]="!canSwitchWorkspace()"
              [title]="
                canSwitchWorkspace()
                  ? 'Ganti workspace'
                  : 'Ganti workspace tersedia dari dashboard workspace'
              "
              (change)="switchWorkspace($event)"
            >
              <option value="">
                {{ workspaceOptions().length ? '-- Pilih workspace --' : '-- Belum ada workspace --' }}
              </option>
              @for (opt of workspaceOptions(); track opt.id) {
                <option [value]="opt.id">{{ opt.name }}</option>
              }
            </select>
          </div>

          <div class="topbar-actions">
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

        @if (ws.currentWorkspace()?.archived_at) {
          <div class="alert alert-warning banner" role="status">
            <app-icon name="archive" [size]="16" />
            <span>
              Workspace diarsipkan. Seluruh data hanya bisa dibaca sampai workspace dipulihkan.
            </span>
          </div>
        }

        <main #content>
          <div class="content-surface">
            @if (ws.contextError(); as error) {
              <div class="page">
                <app-empty-state
                  icon="alert"
                  title="Tidak dapat mengakses workspace"
                  [description]="error"
                  actionLabel="Kembali ke daftar workspace"
                  (actionClicked)="backToWorkspaces()"
                />
              </div>
            } @else {
              <router-outlet />
            }
          </div>
        </main>
      </div>
    </div>
  `,
  styles: `
    /*
     * Workspace switcher ada di topbar, bukan di sidebar, sesuai SSOT 8.
     * Bentuknya memakai pola form control yang sama dengan input lain supaya
     * konsisten dan tetap punya tinggi sentuh yang nyaman.
     */
    .workspace-switch {
      font-size: 0.9375rem;
      font-weight: 600;
      color: var(--ink);
      min-height: 38px;
      max-width: 260px;
      padding: 0 var(--space-4);
      border: 1px solid var(--line-strong);
      border-radius: var(--radius-sm);
      background: var(--surface);
      cursor: pointer;
    }

    .workspace-switch:hover:not(:disabled) {
      border-color: var(--ink-3);
    }

    .workspace-switch:disabled {
      background: var(--surface-3);
      color: var(--ink-3);
      cursor: not-allowed;
    }

    /* Di bawah 720px switcher workspace mengambil sisa ruang yang ada. */
    @media (max-width: 720px) {
      .workspace-switch {
        max-width: none;
        flex: 1;
        min-width: 0;
      }
    }

    .banner {
      margin: 0;
      border-radius: 0;
      border-left: none;
      border-right: none;
      align-items: center;
    }
  `
})
export class WorkspaceLayoutComponent implements AfterViewInit {
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  protected readonly theme = inject(ThemeService);
  protected readonly ws = inject(WorkspaceService);

  @ViewChild('content') private contentRef?: ElementRef<HTMLElement>;

  readonly menuOpen = signal(false);
  readonly workspaceId = signal<string>('');
  readonly workspaceOptions = signal<Workspace[]>([]);
  readonly currentUrl = signal(this.router.url);

  /**
   * Sidebar tertutup jadi rail ikon selebar 64px. Preferensi ini disimpan di
   * localStorage karena ini preferensi tampilan, bukan token atau rahasia.
   */
  readonly collapsed = signal(false);

  /**
   * Ganti workspace hanya boleh dari dashboard workspace.
   *
   * Alasan: dari halaman lain (item, anggota, pengaturan) perpindahan
   * workspace melempar konteks halaman, jadi pengguna tiba di dashboard
   * tanpa tahu halaman mana yang sedang dikerjakan. Switcher tetap terlihat
   * dan terbaca, tapi tidak bisa dipakai.
   */
  readonly canSwitchWorkspace = computed<boolean>(() => {
    const path = this.currentUrl().split('?')[0];
    return path.endsWith('/dashboard');
  });

  constructor() {
    this.collapsed.set(this.readCollapsedPreference());

    // applyWorkspace() memuat context workspace (nama, role, anggota) sekaligus
    // daftar workspace untuk dropdown sidebar. Kalau hanya set signal di sini,
    // stream di bawah akan melihat wid yang sama dan tidak pernah memanggil
    // applyWorkspace, sehingga dropdown sidebar tetap kosong.
    const wid = findWorkspaceId(this.route, this.ws.activeWorkspaceId());
    if (wid) {
      this.applyWorkspace(wid);
    }

    workspaceIdStream(this.route, () => this.ws.activeWorkspaceId())
      .pipe(takeUntilDestroyed())
      .subscribe(wid => {
        if (!wid) {
          this.router.navigate(['/app/workspaces']);
          return;
        }
        if (wid !== this.workspaceId()) {
          this.applyWorkspace(wid);
        }
      });

    this.router.events
      .pipe(
        filter((event): event is NavigationEnd => event instanceof NavigationEnd),
        map(() => this.router.url),
        takeUntilDestroyed()
      )
      .subscribe(url => {
        this.currentUrl.set(url);
        this.resetScroll();
      });
  }

  ngAfterViewInit(): void {
    this.resetScroll();
  }

  toggleCollapsed(): void {
    const next = !this.collapsed();
    this.collapsed.set(next);
    try {
      localStorage.setItem('tb.sidebar.collapsed', next ? '1' : '0');
    } catch {
      // Penyimpanan browser penuh atau diblokir, preferensi hanya berlaku di sesi ini.
    }
  }

  private readCollapsedPreference(): boolean {
    try {
      return localStorage.getItem('tb.sidebar.collapsed') === '1';
    } catch {
      return false;
    }
  }

  /**
   * Area konten adalah container scroll sendiri, jadi posisinya tidak lagi
   * ikut direset browser saat halaman berganti. Tanpa ini, pengguna yang
   * sudah menggulir daftar panjang akan mendarat di tengah halaman berikutnya.
   */
  private resetScroll(): void {
    this.contentRef?.nativeElement.scrollTo({ top: 0 });
  }

  @HostListener('document:keydown.escape')
  onEscape(): void {
    this.menuOpen.set(false);
  }

  isItemsActive(type: string | null): boolean {
    const [path, query] = this.currentUrl().split('?');
    if (!path.endsWith('/items')) return false;
    const params = new URLSearchParams(query ?? '');
    return type ? params.get('type') === type : !params.get('type');
  }

  private applyWorkspace(wid: string): void {
    this.workspaceId.set(wid);
    this.ws.activeWorkspaceId.set(wid);
    this.ws.loadContext(wid);
    this.loadWorkspaceOptions(wid);
  }

  private loadWorkspaceOptions(currentId: string): void {
    this.ws.getWorkspaces().subscribe({
      next: res => {
        const data = (res && res.data) || [];
        let options: Workspace[] = Array.isArray(data) ? data : [];

        // Pastikan workspace yang sedang aktif selalu punya entri di dropdown,
        // walau API tidak mengembalikannya (mis. admin dengan akses langsung).
        const current = this.ws.currentWorkspace();
        const active = current && current.id === currentId ? current : options.find(w => w.id === currentId);
        if (active && !options.some(w => w.id === active.id)) {
          options = [active, ...options];
        }

        this.workspaceOptions.set(options);
      },
      error: () => {
        const current = this.ws.currentWorkspace();
        this.workspaceOptions.set(current ? [current] : []);
      }
    });
  }

  switchWorkspace(event: Event): void {
    const id = (event.target as HTMLSelectElement).value;
    if (!id || id === this.workspaceId()) return;

    // currentRole dihitung dari workspaceMembers. Muat ulang konteks lewat
    // applyWorkspace() supaya anggota diambil untuk workspace yang baru,
    // bukan dibiarkan kosong atau memakai data workspace sebelumnya.
    const selected = this.workspaceOptions().find(w => w.id === id);
    if (selected) this.ws.currentWorkspace.set(selected);
    this.applyWorkspace(id);
    this.router.navigate(['/app/w', id, 'dashboard']);
  }

  backToWorkspaces(): void {
    this.router.navigate(['/app/workspaces']);
  }

  themeLabel(): string {
    return this.theme.theme() === 'dark'
      ? 'Beralih ke mode terang'
      : 'Beralih ke mode gelap';
  }

}
