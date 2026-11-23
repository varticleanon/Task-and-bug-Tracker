import { Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { Observable } from 'rxjs';
import {
  WorkItem,
  WorkItemListResponse,
  ItemFilterParams,
  ItemActivity
} from '../models/item.model';
import { WorkspaceMember } from '../models/workspace.model';
import { environment } from '../../../environments/environment';

export interface CreateItemPayload {
  type: 'TASK' | 'BUG';
  title: string;
  description?: string;
  status?: string;
  priority?: string;
  assignee_id?: string | null;
  due_date?: string | null;
  steps_to_reproduce?: string;
  expected_result?: string;
  actual_result?: string;
}

export interface UpdateItemPayload {
  version: number;
  type?: string;
  title?: string;
  description?: string;
  status?: string;
  priority?: string;
  assignee_id?: string | null;
  due_date?: string | null;
  steps_to_reproduce?: string;
  expected_result?: string;
  actual_result?: string;
}

@Injectable({
  providedIn: 'root'
})
export class WorkItemService {
  constructor(private http: HttpClient) {}

  getItems(workspaceId: string, filters: ItemFilterParams): Observable<WorkItemListResponse> {
    let params = new HttpParams();
    if (filters.type) params = params.set('type', filters.type);
    if (filters.status) params = params.set('status', filters.status);
    if (filters.priority) params = params.set('priority', filters.priority);
    if (filters.assignee_id) params = params.set('assignee_id', filters.assignee_id);
    if (filters.unassigned) params = params.set('unassigned', 'true');
    if (filters.search && filters.search.length >= 2) params = params.set('search', filters.search);
    if (filters.due_before) params = params.set('due_before', filters.due_before);
    if (filters.due_after) params = params.set('due_after', filters.due_after);
    if (filters.page) params = params.set('page', filters.page.toString());
    if (filters.page_size) params = params.set('page_size', filters.page_size.toString());

    return this.http.get<WorkItemListResponse>(`${environment.apiUrl}/workspaces/${workspaceId}/items`, { params });
  }

  getItem(workspaceId: string, id: string): Observable<{ data: WorkItem }> {
    return this.http.get<{ data: WorkItem }>(`${environment.apiUrl}/workspaces/${workspaceId}/items/${id}`);
  }

  createItem(workspaceId: string, payload: CreateItemPayload): Observable<{ data: WorkItem }> {
    return this.http.post<{ data: WorkItem }>(`${environment.apiUrl}/workspaces/${workspaceId}/items`, payload);
  }

  updateItem(workspaceId: string, id: string, payload: UpdateItemPayload): Observable<{ data: WorkItem }> {
    return this.http.patch<{ data: WorkItem }>(`${environment.apiUrl}/workspaces/${workspaceId}/items/${id}`, payload);
  }

  deleteItem(workspaceId: string, id: string): Observable<void> {
    return this.http.delete<void>(`${environment.apiUrl}/workspaces/${workspaceId}/items/${id}`);
  }

  getItemActivities(
    workspaceId: string,
    id: string
  ): Observable<{ data: ItemActivity[] }> {
    return this.http.get<{ data: ItemActivity[] }>(
      `${environment.apiUrl}/workspaces/${workspaceId}/items/${id}/activities`
    );
  }

  getWorkspaceMembers(
    workspaceId: string
  ): Observable<{ data: WorkspaceMember[] }> {
    return this.http.get<{ data: WorkspaceMember[] }>(
      `${environment.apiUrl}/workspaces/${workspaceId}/members`
    );
  }
}
