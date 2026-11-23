export interface Attachment {
  id: string;
  workspace_id: string;
  item_id: string;
  uploader_id: string;
  original_filename: string;
  mime_type: string;
  byte_size: number;
  width: number;
  height: number;
  created_at: string;
}
