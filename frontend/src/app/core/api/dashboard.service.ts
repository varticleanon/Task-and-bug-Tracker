import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';

export interface DashboardSummary {
  total_open: number;
  todo: number;
  in_progress: number;
  blocked: number;
  in_review: number;
  done: number;
  bugs: number;
  tasks: number;
  assigned_to_me: number;
  overdue: number;
}

@Injectable({
  providedIn: 'root'
})
export class DashboardService {
  constructor(private http: HttpClient) {}

  getDashboardSummary(workspaceId: string): Observable<{ data: DashboardSummary }> {
    return this.http.get<{ data: DashboardSummary }>(`${environment.apiUrl}/workspaces/${workspaceId}/dashboard`);
  }
}
