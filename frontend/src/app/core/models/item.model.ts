import { Attachment } from './attachment.model';

export interface ItemActivity {
  id: number;
  actor_user_id: string | null;
  actor_name: string;
  action: string;
  details?: string | Record<string, unknown>;
  created_at: string;
}

export interface WorkItem {
  id: string;
  workspace_id: string;
  type: 'TASK' | 'BUG';
  title: string;
  description: string;
  status: 'TODO' | 'IN_PROGRESS' | 'BLOCKED' | 'IN_REVIEW' | 'DONE';
  priority: 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';
  assignee_id: string | null;
  due_date: string | null;
  steps_to_reproduce?: string;
  expected_result?: string;
  actual_result?: string;
  version: number;
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
  attachments?: Attachment[];
}

export interface WorkItemListResponse {
  data: WorkItem[];
  meta: {
    page: number;
    page_size: number;
    total: number;
    total_pages: number;
  };
}

export interface ItemFilterParams {
  type?: string;
  status?: string;
  priority?: string;
  assignee_id?: string;
  unassigned?: boolean;
  search?: string;
  due_before?: string;
  due_after?: string;
  page?: number;
  page_size?: number;
}
