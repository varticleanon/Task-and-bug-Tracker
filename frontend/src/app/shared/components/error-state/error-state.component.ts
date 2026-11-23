import { ChangeDetectionStrategy, Component, EventEmitter, Input, Output } from '@angular/core';

@Component({
  selector: 'app-error-state',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="alert alert-error" role="alert">
      <span class="grow">{{ message }}</span>
      @if (showRetry) {
        <button type="button" class="btn btn-secondary btn-sm" (click)="retryClicked.emit()">
          Coba lagi
        </button>
      }
    </div>
  `
})
export class ErrorStateComponent {
  @Input() message = 'Terjadi kesalahan saat memuat data.';
  @Input() showRetry = true;
  @Output() retryClicked = new EventEmitter<void>();
}
