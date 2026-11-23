import { ChangeDetectionStrategy, Component, Input } from '@angular/core';

const ICON_PATHS: Record<string, string[]> = {
  dashboard: ['M4 4h6v6H4z', 'M14 4h6v6h-6z', 'M4 14h6v6H4z', 'M14 14h6v6h-6z'],
  list: ['M8 6h13', 'M8 12h13', 'M8 18h13', 'M4 6h.01', 'M4 12h.01', 'M4 18h.01'],
  task: ['M9.5 12.5l2.5 2.5L20 6', 'M20 13v5.5a1.5 1.5 0 0 1-1.5 1.5h-13A1.5 1.5 0 0 1 4 18.5v-13A1.5 1.5 0 0 1 5.5 4H15'],
  bug: [
    'M9.5 8a2.5 2.5 0 0 1 5 0v9a2.5 2.5 0 0 1-5 0z',
    'M12 5.5V4',
    'M4 11h3.5',
    'M16.5 11H20',
    'M4 17h3.5',
    'M16.5 17H20',
    'M6.5 8L4.5 6.5',
    'M17.5 8l2-1.5',
    'M6.5 20l-2 1.5',
    'M17.5 20l2 1.5'
  ],
  users: [
    'M15.5 20v-1.5a4 4 0 0 0-4-4h-4a4 4 0 0 0-4 4V20',
    'M9.5 10.5a3.5 3.5 0 1 0 0-7a3.5 3.5 0 0 0 0 7',
    'M17.5 14.5a3 3 0 0 1 3.5 3V20',
    'M16 3.2a3.5 3.5 0 0 1 0 6.6'
  ],
  settings: [
    'M4 7h8',
    'M16 7h4',
    'M4 12h4',
    'M12 12h8',
    'M4 17h10',
    'M18 17h2',
    'M14 5v4',
    'M10 10v4',
    'M16 15v4'
  ],
  folder: ['M3 7.5A1.5 1.5 0 0 1 4.5 6h4L10.5 8.5h7A1.5 1.5 0 0 1 19 10v7.5a1.5 1.5 0 0 1-1.5 1.5h-13A1.5 1.5 0 0 1 3 17.5z'],
  alert: ['M12 4.5L21 19.5H3z', 'M12 10v4', 'M12 17h.01'],
  blocked: ['M12 21a9 9 0 1 0 0-18a9 9 0 0 0 0 18', 'M5.6 5.6l12.8 12.8'],
  circle: ['M12 21a9 9 0 1 0 0-18a9 9 0 0 0 0 18'],
  progress: ['M12 3a9 9 0 0 1 0 18', 'M12 3v18'],
  eye: ['M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12z', 'M12 14.8a2.8 2.8 0 1 0 0-5.6a2.8 2.8 0 0 0 0 5.6'],
  'eye-off': [
    'M4 4l16 16',
    'M9.9 5.8A9.7 9.7 0 0 1 12 5.5c6 0 9.5 6.5 9.5 6.5a16 16 0 0 1-3.1 3.9',
    'M6.3 7.6A16 16 0 0 0 2.5 12S6 18.5 12 18.5a9.7 9.7 0 0 0 3.2-.5',
    'M9.9 9.9a3 3 0 0 0 4.2 4.2'
  ],
  'check-circle': ['M12 21a9 9 0 1 0 0-18a9 9 0 0 0 0 18', 'M8 12.2l2.6 2.6L16.2 9'],
  user: ['M12 11.5a4 4 0 1 0 0-8a4 4 0 0 0 0 8', 'M4.5 20a7.5 7.5 0 0 1 15 0'],
  clock: ['M12 21a9 9 0 1 0 0-18a9 9 0 0 0 0 18', 'M12 7.5V12l3 2'],
  search: ['M11 18.5a7.5 7.5 0 1 0 0-15a7.5 7.5 0 0 0 0 15', 'M20.5 20.5l-4.2-4.2'],
  plus: ['M12 5.5v13', 'M5.5 12h13'],
  pencil: ['M4 20h4l10-10a2.5 2.5 0 0 0-3.5-3.5L4 16.5z', 'M14 7l3 3'],
  trash: ['M4.5 7h15', 'M9.5 7V5h5v2', 'M6.5 7l1 13h9l1-13', 'M10.5 11v5.5', 'M13.5 11v5.5'],
  x: ['M6 6l12 12', 'M18 6L6 18'],
  menu: ['M4 7h16', 'M4 12h16', 'M4 17h16'],
  logout: ['M14.5 4H19a1.5 1.5 0 0 1 1.5 1.5v13A1.5 1.5 0 0 1 19 20h-4.5', 'M10 8l-4 4 4 4', 'M6 12h9'],
  sun: [
    'M12 16.5a4.5 4.5 0 1 0 0-9a4.5 4.5 0 0 0 0 9',
    'M12 2.5V4',
    'M12 20v1.5',
    'M4.2 4.2l1.1 1.1',
    'M18.7 18.7l1.1 1.1',
    'M2.5 12H4',
    'M20 12h1.5',
    'M4.2 19.8l1.1-1.1',
    'M18.7 5.3l1.1-1.1'
  ],
  moon: ['M20.5 14.8A8.8 8.8 0 0 1 9.2 3.5a8.8 8.8 0 1 0 11.3 11.3z'],
  'chevron-down': ['M6 9.5l6 6 6-6'],
  'chevron-left': ['M14.5 6l-6 6 6 6'],
  'chevron-right': ['M9.5 6l6 6-6 6'],
  upload: ['M12 16V4', 'M8 8l4-4 4 4', 'M4 16v2.5A1.5 1.5 0 0 0 5.5 20h13a1.5 1.5 0 0 0 1.5-1.5V16'],
  lock: ['M6.5 11h11v9h-11z', 'M9 11V8a3 3 0 0 1 6 0v3'],
  mail: ['M3 6.5h18v11H3z', 'M3.5 7l8.5 6 8.5-6'],
  inbox: ['M3 13.5h5l1.5 3h5l1.5-3h5', 'M3 13.5L6 5h12l3 8.5V19H3z'],
  filter: ['M3.5 5.5h17l-6.5 8v5.5l-4 1.5v-7z'],
  refresh: ['M20 12a8 8 0 1 1-2.4-5.7', 'M20 4.5V10h-5.5'],
  'arrow-right': ['M4 12h15', 'M13 6l6 6-6 6'],
  copy: ['M9 9h9a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H9a1 1 0 0 1-1-1v-9a1 1 0 0 1 1-1z', 'M5 15H4a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h9a1 1 0 0 1 1 1v1'],
  archive: ['M3.5 5.5h17V10h-17z', 'M5.5 10v9h13v-9', 'M10 14h4']
};

@Component({
  selector: 'app-icon',
  standalone: true,
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <svg
      [attr.width]="size"
      [attr.height]="size"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      [attr.stroke-width]="strokeWidth"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      @for (d of paths; track d) {
        <path [attr.d]="d" />
      }
    </svg>
  `,
  styles: `
    :host {
      display: inline-flex;
      line-height: 0;
    }
  `
})
export class IconComponent {
  @Input({ required: true }) name = '';
  @Input() size = 18;
  @Input() strokeWidth = 1.6;

  get paths(): string[] {
    return ICON_PATHS[this.name] ?? [];
  }
}
