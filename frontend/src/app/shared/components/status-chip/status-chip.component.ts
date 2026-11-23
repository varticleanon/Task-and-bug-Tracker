import { ChangeDetectionStrategy, Component, Input } from '@angular/core';
import { IconComponent } from '../icon/icon.component';

export type ChipKind = 'status' | 'priority' | 'type' | 'role' | 'state';
export type ChipTone = 'accent' | 'success' | 'warn' | 'danger' | 'quiet';

const LABELS: Record<ChipKind, Record<string, string>> = {
  status: {
    TODO: 'Belum mulai',
    IN_PROGRESS: 'Dikerjakan',
    IN_REVIEW: 'Ditinjau',
    BLOCKED: 'Terhambat',
    DONE: 'Selesai'
  },
  priority: {
    LOW: 'Rendah',
    MEDIUM: 'Sedang',
    HIGH: 'Tinggi',
    CRITICAL: 'Kritis'
  },
  type: {
    TASK: 'Task',
    BUG: 'Bug'
  },
  role: {
    SUPER_ADMIN: 'Super Admin',
    ADMIN: 'Admin',
    MEMBER: 'Member',
    VIEWER: 'Viewer',
    USER: 'Pengguna'
  },
  state: {
    ACTIVE: 'Aktif',
    INACTIVE: 'Nonaktif',
    ARCHIVED: 'Terarsip'
  }
};

const TONES: Record<string, ChipTone> = {
  TODO: 'quiet',
  IN_PROGRESS: 'accent',
  IN_REVIEW: 'warn',
  BLOCKED: 'danger',
  DONE: 'success',
  LOW: 'quiet',
  MEDIUM: 'quiet',
  HIGH: 'warn',
  CRITICAL: 'danger',
  TASK: 'quiet',
  BUG: 'danger',
  MEMBER: 'quiet',
  VIEWER: 'quiet',
  ADMIN: 'accent',
  SUPER_ADMIN: 'accent',
  USER: 'quiet',
  ACTIVE: 'success',
  INACTIVE: 'quiet',
  ARCHIVED: 'quiet'
};

const ICONS: Record<string, string> = {
  TODO: 'circle',
  IN_PROGRESS: 'progress',
  IN_REVIEW: 'eye',
  BLOCKED: 'blocked',
  DONE: 'check-circle',
  TASK: 'task',
  BUG: 'bug'
};

const HOLLOW: Record<string, boolean> = {
  LOW: true,
  VIEWER: true,
  INACTIVE: true
};

@Component({
  selector: 'app-chip',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [IconComponent],
  template: `
    <span
      class="chip"
      [class.chip--accent]="tone === 'accent'"
      [class.chip--success]="tone === 'success'"
      [class.chip--warn]="tone === 'warn'"
      [class.chip--danger]="tone === 'danger'"
      [class.chip--quiet]="tone === 'quiet'"
    >
      @if (icon) {
        <span class="chip-icon"><app-icon [name]="icon" [size]="13" [strokeWidth]="1.9" /></span>
      } @else {
        <span class="chip-dot" [class.chip-dot--hollow]="hollow"></span>
      }
      {{ text }}
    </span>
  `
})
export class StatusChipComponent {
  @Input({ required: true }) kind: ChipKind = 'status';
  @Input({ required: true }) value = '';
  @Input() label: string | null = null;

  get text(): string {
    return this.label ?? LABELS[this.kind][this.value] ?? this.value;
  }

  get tone(): ChipTone {
    return TONES[this.value] ?? 'quiet';
  }

  get icon(): string | null {
    return ICONS[this.value] ?? null;
  }

  get hollow(): boolean {
    return HOLLOW[this.value] ?? false;
  }
}
