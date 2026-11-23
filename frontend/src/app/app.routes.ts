import { Routes } from '@angular/router';
import { authGuard } from './core/guards/auth.guard';
import { superAdminGuard } from './core/guards/super-admin.guard';
import { workspaceIdGuard } from './core/guards/workspace-id.guard';
import { WorkspaceLayoutComponent } from './shared/layout/workspace-layout.component';
import { MainLayoutComponent } from './shared/layout/main-layout.component';

/**
 * Route map mengikuti SSOT §8.
 * Semua route di bawah /app melewati authGuard; guard backend tetap otoritas final.
 */
export const routes: Routes = [
  {
    path: 'login',
    loadComponent: () =>
      import('./features/login/login.component').then(m => m.LoginComponent),
    title: 'Masuk'
  },
  {
    path: 'change-password',
    canActivate: [authGuard],
    loadComponent: () =>
      import('./features/change-password/change-password.component').then(
        m => m.ChangePasswordComponent
      ),
    title: 'Ganti Password'
  },
  {
    path: 'app',
    canActivate: [authGuard],
    children: [
      {
        path: 'workspaces',
        component: MainLayoutComponent,
        children: [
          {
            path: '',
            loadComponent: () =>
              import('./features/workspaces/workspace-list.component').then(
                m => m.WorkspaceListComponent
              ),
            title: 'Workspace'
          }
        ]
      },
      {
        path: 'admin/users',
        component: MainLayoutComponent,
        children: [
          {
            path: '',
            canActivate: [superAdminGuard],
            loadComponent: () =>
              import('./features/admin/users/users.component').then(
                m => m.UsersComponent
              ),
            title: 'Manajemen Pengguna'
          }
        ]
      },
      {
        path: 'admin/workspaces',
        component: MainLayoutComponent,
        children: [
          {
            path: '',
            canActivate: [superAdminGuard],
            loadComponent: () =>
              import(
                './features/admin/workspaces/admin-workspaces.component'
              ).then(m => m.AdminWorkspacesComponent),
            title: 'Manajemen Workspace'
          }
        ]
      },
      {
        path: 'w/:workspaceId',
        canActivate: [workspaceIdGuard],
        component: WorkspaceLayoutComponent,
        children: [
          { path: '', pathMatch: 'full', redirectTo: 'dashboard' },
          {
            path: 'dashboard',
            loadComponent: () =>
              import('./features/dashboard/dashboard.component').then(
                m => m.DashboardComponent
              ),
            title: 'Dashboard'
          },
          {
            path: 'items',
            loadComponent: () =>
              import('./features/items/item-list/item-list.component').then(
                m => m.ItemListComponent
              ),
            title: 'Daftar Item'
          },
          {
            path: 'items/new',
            loadComponent: () =>
              import('./features/items/item-form/item-form.component').then(
                m => m.ItemFormComponent
              ),
            title: 'Item Baru'
          },
          {
            path: 'items/:itemId',
            loadComponent: () =>
              import('./features/items/item-detail/item-detail.component').then(
                m => m.ItemDetailComponent
              ),
            title: 'Detail Item'
          },
          {
            path: 'items/:itemId/edit',
            loadComponent: () =>
              import('./features/items/item-form/item-form.component').then(
                m => m.ItemFormComponent
              ),
            title: 'Edit Item'
          },
          {
            path: 'members',
            loadComponent: () =>
              import(
                './features/workspaces/workspace-members.component'
              ).then(m => m.WorkspaceMembersComponent),
            title: 'Anggota Workspace'
          },
          {
            path: 'settings',
            loadComponent: () =>
              import(
                './features/workspaces/workspace-settings/workspace-settings.component'
              ).then(m => m.WorkspaceSettingsComponent),
            title: 'Pengaturan Workspace'
          }
        ]
      }
    ]
  },
  {
    path: '404',
    loadComponent: () =>
      import('./features/not-found/not-found.component').then(
        m => m.NotFoundComponent
      ),
    title: 'Halaman Tidak Ditemukan'
  },
  { path: '', pathMatch: 'full', redirectTo: '/login' },
  { path: '**', redirectTo: '/404' }
];
