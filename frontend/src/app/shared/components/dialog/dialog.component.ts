import {
  AfterViewInit,
  Component,
  ElementRef,
  EventEmitter,
  Input,
  Output,
  viewChild
} from '@angular/core';
import { IconComponent } from '../icon/icon.component';

let dialogSequence = 0;

@Component({
  selector: 'app-dialog',
  standalone: true,
  imports: [IconComponent],
  template: `
    <dialog
      #dialog
      class="dialog"
      [class.dialog--wide]="wide"
      [attr.aria-labelledby]="titleId"
      (cancel)="onCancel()"
      (close)="closed.emit()"
      (click)="onBackdropClick($event)"
    >
      <div class="dialog-header">
        <h2 class="dialog-title" [id]="titleId">{{ title }}</h2>
        <button
          type="button"
          class="icon-btn"
          aria-label="Tutup dialog"
          (click)="close()"
        >
          <app-icon name="x" [size]="18" />
        </button>
      </div>

      <div class="dialog-body">
        <ng-content />
      </div>

      <div class="dialog-footer">
        <ng-content select="[dialogFooter]" />
      </div>
    </dialog>
  `
})
export class DialogComponent implements AfterViewInit {
  private readonly dialogRef = viewChild.required<ElementRef<HTMLDialogElement>>('dialog');

  @Input({ required: true }) title = '';
  @Input() wide = false;
  @Output() closed = new EventEmitter<void>();

  readonly titleId = `dialog-title-${++dialogSequence}`;

  ngAfterViewInit(): void {
    this.dialogRef().nativeElement.showModal();
  }

  close(): void {
    this.dialogRef().nativeElement.close();
  }

  onCancel(): void {
    // Escape ditutup oleh elemen native; event 'close' yang resmi memberitahu parent.
  }

  onBackdropClick(event: MouseEvent): void {
    if (event.target === this.dialogRef().nativeElement) {
      this.close();
    }
  }
}
