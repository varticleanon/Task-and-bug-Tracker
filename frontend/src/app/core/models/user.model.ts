export interface User {
  id: string;
  email: string;
  name: string;
  global_role: 'SUPER_ADMIN' | 'USER';
  is_active: boolean;
  must_change_password: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface UserListMeta {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

/**
 * Response list pengguna. Semua field opsional karena backend bisa
 * mengirim body kosong (mis. handler belum terpasang), frontend wajib
 * accesses dengan null-safe operator.
 */
export interface UserListResponse {
  data?: User[] | null;
  meta?: UserListMeta | null;
}

export interface CreateUserData {
  user?: User | null;
  temporary_password?: string | null;
}

export interface CreateUserResponse {
  data?: CreateUserData | null;
}

export interface ResetPasswordData {
  message?: string | null;
  temporary_password?: string | null;
}

export interface ResetPasswordResponse {
  data?: ResetPasswordData | null;
}
