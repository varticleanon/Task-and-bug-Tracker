import {
  Component,
  Input,
  Output,
  EventEmitter,
  inject,
  signal
} from '@angular/core';
import { CommonModule } from '@angular/common';
import { HttpEventType } from '@angular/common/http';
import { Attachment } from '../../../core/models/attachment.model';
import { AttachmentService } from '../../../core/api/attachment.service';
import { IconComponent } from '../../../shared/components/icon/icon.component';

const MAX_FILE_BYTES = 5 * 1024 * 1024;
const ALLOWED_TYPES = ['image/jpeg', 'image/png', 'image/webp'];

@Component({
  selector: 'app-attachment-gallery',
  standalone: true,
  imports: [CommonModule, IconComponent],
  templateUrl: './attachment-gallery.component.html',
  styleUrls: ['./attachment-gallery.component.css']
})
export class AttachmentGalleryComponent {
  private readonly attachmentService = inject(AttachmentService);

  @Input({ required: true }) workspaceId!: string;
  @Input({ required: true }) itemId!: string;
  @Input() attachments: Attachment[] = [];
  @Input() canUpload = true;
  @Input() canDelete = true;

  @Output() attachmentChanged = new EventEmitter<void>();

  readonly uploadProgress = signal<number | null>(null);
  readonly uploadError = signal<string | null>(null);
  readonly selectedFile = signal<File | null>(null);
  readonly lightboxImage = signal<Attachment | null>(null);
  readonly deletingId = signal<string | null>(null);
  readonly dragOver = signal<boolean>(false);

  get maxReached(): boolean {
    return this.attachments.length >= 5;
  }

  getContentUrl(attachmentId: string): string {
    return this.attachmentService.getContentUrl(
      this.workspaceId,
      this.itemId,
      attachmentId
    );
  }

  validateFile(file: File): string | null {
    if (!ALLOWED_TYPES.includes(file.type)) {
      return 'Format file tidak didukung. Gunakan JPEG, PNG, atau WebP.';
    }
    if (file.size > MAX_FILE_BYTES) {
      return 'Ukuran file melebihi 5 MiB.';
    }
    return null;
  }

  onFileSelected(event: Event): void {
    const input = event.target as HTMLInputElement;
    if (input.files && input.files.length > 0) {
      this.handleFile(input.files[0]);
    }
    input.value = '';
  }

  onDragOver(event: DragEvent): void {
    event.preventDefault();
    event.stopPropagation();
    this.dragOver.set(true);
  }

  onDragLeave(event: DragEvent): void {
    event.preventDefault();
    event.stopPropagation();
    this.dragOver.set(false);
  }

  onDrop(event: DragEvent): void {
    event.preventDefault();
    event.stopPropagation();
    this.dragOver.set(false);

    if (!this.canUpload || this.maxReached) return;
    const files = event.dataTransfer?.files;
    if (files && files.length > 0) {
      this.handleFile(files[0]);
    }
  }

  private handleFile(file: File): void {
    if (!this.canUpload || this.maxReached) return;

    const validationError = this.validateFile(file);
    if (validationError) {
      this.uploadError.set(validationError);
      return;
    }

    this.selectedFile.set(file);
    this.uploadFile(file);
  }

  retryUpload(): void {
    const file = this.selectedFile();
    if (file) {
      this.uploadFile(file);
    }
  }

  uploadFile(file: File): void {
    if (this.maxReached) {
      this.uploadError.set('Maksimum 5 gambar per item telah tercapai.');
      return;
    }

    this.uploadProgress.set(0);
    this.uploadError.set(null);

    this.attachmentService
      .uploadAttachment(this.workspaceId, this.itemId, file)
      .subscribe({
        next: event => {
          if (event.type === HttpEventType.UploadProgress && event.total) {
            const percent = Math.round((100 * event.loaded) / event.total);
            this.uploadProgress.set(percent);
          } else if (event.type === HttpEventType.Response) {
            this.uploadProgress.set(null);
            this.selectedFile.set(null);
            this.attachmentChanged.emit();
          }
        },
        error: err => {
          this.uploadProgress.set(null);
          this.uploadError.set(
            err?.error?.error?.message ||
              'Gagal mengunggah gambar. Pastikan format JPEG/PNG/WebP dan maksimal 5 MiB.'
          );
        }
      });
  }

  deleteAttachment(attachmentId: string): void {
    if (!confirm('Apakah Anda yakin ingin menghapus gambar ini?')) return;

    this.deletingId.set(attachmentId);
    this.attachmentService
      .deleteAttachment(this.workspaceId, this.itemId, attachmentId)
      .subscribe({
        next: () => {
          this.deletingId.set(null);
          if (this.lightboxImage()?.id === attachmentId) {
            this.closeLightbox();
          }
          this.attachmentChanged.emit();
        },
        error: err => {
          this.deletingId.set(null);
          this.uploadError.set(
            err?.error?.error?.message || 'Gagal menghapus gambar.'
          );
        }
      });
  }

  openLightbox(attachment: Attachment): void {
    this.lightboxImage.set(attachment);
  }

  closeLightbox(): void {
    this.lightboxImage.set(null);
  }

  onLightboxKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      this.closeLightbox();
    }
  }
}
