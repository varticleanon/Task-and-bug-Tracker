import { ChangeDetectionStrategy, Component, EventEmitter, Input, Output } from '@angular/core';
import { IconComponent } from '../icon/icon.component';

@Component({
  selector: 'app-empty-state',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [IconComponent],
  template: `
    <div class="state">
      <span class="state-icon"><app-icon [name]="icon" [size]="20" /></span>
      <h2 class="state-title">{{ title }}</h2>
      <p class="state-text">{{ description }}</p>
      @if (actionLabel) {
        <div class="state-actions">
          <button type="button" class="btn btn-secondary" (click)="actionClicked.emit()">
            {{ actionLabel }}
          </button>
        </div>
      }
    </div>
  `
})
export class EmptyStateComponent {
  @Input() title = 'Belum ada data';
  @Input() description = '';
  @Input() icon = 'inbox';
  @Input() actionLabel: string | null = null;
  @Output() actionClicked = new EventEmitter<void>();
}
