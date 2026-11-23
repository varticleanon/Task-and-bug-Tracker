import { Injectable, signal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Router } from '@angular/router';
import { BehaviorSubject, Observable, finalize, tap, timeout } from 'rxjs';
import { User } from '../models/user.model';
import { environment } from '../../../environments/environment';

export interface AuthResponse {
  data: { user: User };
}

export interface ChangePasswordResponse {
  data: { user: User; message: string };
}

/**
 * AuthService adalah satu-satunya sumber kebenaran status sesi di frontend.
 * Sesi dikelola server melalui HttpOnly cookie; frontend tidak pernah
 * menyimpan token di localStorage/sessionStorage.
 */
@Injectable({
  providedIn: 'root'
})
export class AuthService {
  constructor(private http: HttpClient, private router: Router) {}

  /** Signal agar template reaktif. */
  readonly currentUser = signal<User | null>(null);

  /** Observable untuk consumer yang memakai pola subscribe (mis. dashboard). */
  private readonly userSubject = new BehaviorSubject<User | null>(null);
  readonly currentUser$ = this.userSubject.asObservable();

  private initialized = false;
  private initPromise: Promise<boolean> | null = null;

  setUser(user: User | null): void {
    this.currentUser.set(user);
    this.userSubject.next(user);
  }

  get isAuthenticated(): boolean {
    return this.currentUser() !== null;
  }

  get isSuperAdmin(): boolean {
    return this.currentUser()?.global_role === 'SUPER_ADMIN';
  }

  /** Memanggil POST /auth/login dan menyimpan user di signal. */
  login(email: string, password: string): Observable<AuthResponse> {
    return this.http
      .post<AuthResponse>(`${environment.apiUrl}/auth/login`, { email, password })
      .pipe(
        timeout(30000),
        tap(res => this.setUser(res.data.user))
      );
  }

  /** Memanggil GET /auth/me. */
  me(): Observable<AuthResponse> {
    return this.http
      .get<AuthResponse>(`${environment.apiUrl}/auth/me`)
      .pipe(tap(res => this.setUser(res.data.user)));
  }

  /** Memanggil POST /auth/logout lalu membersihkan state lokal. */
  logout(): Observable<void> {
    return this.http.post<void>(`${environment.apiUrl}/auth/logout`, {}).pipe(
      finalize(() => {
        this.setUser(null);
        this.initialized = false;
        this.initPromise = null;
      })
    );
  }

  /** Memanggil POST /auth/change-password; server menerbitkan sesi baru. */
  changePassword(
    currentPassword: string,
    newPassword: string
  ): Observable<ChangePasswordResponse> {
    return this.http
      .post<ChangePasswordResponse>(`${environment.apiUrl}/auth/change-password`, {
        current_password: currentPassword,
        new_password: newPassword
      })
      .pipe(tap(res => this.setUser(res.data.user)));
  }

  /**
   * Memulihkan sesi dari cookie HttpOnly saat aplikasi pertama kali dimuat.
   * Hasilnya di-cache; panggil `force` untuk memaksa pemeriksaan ulang.
   */
  restoreSession(force = false): Promise<boolean> {
    if (this.initialized && !force) {
      return Promise.resolve(this.isAuthenticated);
    }

    if (!this.initPromise || force) {
      this.initPromise = new Promise<boolean>(resolve => {
        this.http.get<AuthResponse>(`${environment.apiUrl}/auth/me`).subscribe({
          next: res => {
            this.setUser(res.data.user);
            this.initialized = true;
            resolve(true);
          },
          error: () => {
            this.setUser(null);
            this.initialized = true;
            resolve(false);
          }
        });
      });
    }

    return this.initPromise;
  }

  /** Menandai sesi tidak valid (dipakai interceptor 401). */
  markSessionInvalid(): void {
    this.setUser(null);
    this.initialized = false;
    this.initPromise = null;
  }

  /**
   * Navigasi setelah login.
   *
   * returnUrl dipakai kalau pengguna sebelumnya dilempar ke halaman login
   * dari halaman tertentu, supaya ia kembali ke sana.
   *
   * Selain itu pengguna selalu mendarat di pemilih workspace, bukan langsung
   * ke dashboard workspace terakhir. Alasannya: workspace terakhir bisa jadi
   * workspace milik orang lain, workspace yang sudah diarsipkan, atau
   * workspace yang tidak bisa diakses akun ini, sehingga landing-nya terasa
   * acak. Di pemilih workspace pengguna yang memutuskan sendiri mau masuk ke
   * mana.
   */
  navigateAfterLogin(returnUrl: string | null): void {
    if (returnUrl && returnUrl !== '/login' && returnUrl !== '/change-password') {
      this.router.navigateByUrl(returnUrl);
      return;
    }

    this.router.navigate(['/app/workspaces']);
  }
}
