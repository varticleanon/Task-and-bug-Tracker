import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import {
  UserListResponse,
  CreateUserResponse,
  ResetPasswordResponse,
  User
} from '../models/user.model';
import { environment } from '../../../environments/environment';

@Injectable({
  providedIn: 'root'
})
export class AdminUserService {
  private baseUrl = `${environment.apiUrl}/admin/users`;

  constructor(private http: HttpClient) {}

  getUsers(page = 1, pageSize = 20, search = ''): Observable<UserListResponse> {
    let params = new HttpParams()
      .set('page', page.toString())
      .set('page_size', pageSize.toString());
    if (search.trim()) {
      params = params.set('search', search.trim());
    }
    return this.http.get<UserListResponse>(this.baseUrl, { params });
  }

  createUser(email: string, name: string): Observable<CreateUserResponse> {
    return this.http.post<CreateUserResponse>(this.baseUrl, { email, name });
  }

  updateUser(
    userId: string,
    data: { name?: string; is_active?: boolean }
  ): Observable<{ data?: User | null }> {
    return this.http.patch<{ data?: User | null }>(`${this.baseUrl}/${userId}`, data);
  }

  resetPassword(userId: string): Observable<ResetPasswordResponse> {
    return this.http.post<ResetPasswordResponse>(`${this.baseUrl}/${userId}/reset-password`, {});
  }
}
