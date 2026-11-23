export interface Workspace {
  id: string;
  name: string;
  slug: string;
  description: string;
  created_by: string;
  created_at: string;
  updated_at: string;
  archived_at?: string | null;
}

export interface WorkspaceMember {
  user_id: string;
  email: string;
  name: string;
  role: 'ADMIN' | 'MEMBER' | 'VIEWER';
  joined_at: string;
}
