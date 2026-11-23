import { Injectable } from '@angular/core';
import { HttpClient, HttpEvent, HttpRequest } from '@angular/common/http';
import { Observable } from 'rxjs';
import { Attachment } from '../models/attachment.model';
import { environment } from '../../../environments/environment';

@Injectable({
  providedIn: 'root'
})
export class AttachmentService {
  constructor(private http: HttpClient) {}

  uploadAttachment(workspaceId: string, itemId: string, file: File): Observable<HttpEvent<{ data: Attachment }>> {
    const formData = new FormData();
    formData.append('image', file, file.name);

    const req = new HttpRequest(
      'POST',
      `${environment.apiUrl}/workspaces/${workspaceId}/items/${itemId}/attachments`,
      formData,
      {
        reportProgress: true
      }
    );

    return this.http.request<{ data: Attachment }>(req);
  }

  deleteAttachment(workspaceId: string, itemId: string, attachmentId: string): Observable<void> {
    return this.http.delete<void>(`${environment.apiUrl}/workspaces/${workspaceId}/items/${itemId}/attachments/${attachmentId}`);
  }

  getContentUrl(workspaceId: string, itemId: string, attachmentId: string): string {
    return `${environment.apiUrl}/workspaces/${workspaceId}/items/${itemId}/attachments/${attachmentId}/content`;
  }
}
