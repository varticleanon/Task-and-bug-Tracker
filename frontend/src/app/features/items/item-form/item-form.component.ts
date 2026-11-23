import { Component, computed, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { combineLatest, finalize } from 'rxjs';
import { WorkItemService, CreateItemPayload, UpdateItemPayload } from '../../../core/api/work-item.service';
import { WorkItem } from '../../../core/models/item.model';
import { WorkspaceMember } from '../../../core/models/workspace.model';
import { WorkspaceService } from '../../../core/services/workspace.service';
import { workspaceIdStream } from '../../../core/helpers/workspace-route.helper';
import { IconComponent } from '../../../shared/components/icon/icon.component';

@Component({
  selector: 'app-item-form',
  standalone: true,
  imports: [ReactiveFormsModule, RouterLink, IconComponent],
  templateUrl: './item-form.component.html',
  styleUrls: ['./item-form.component.css']
})
export class ItemFormComponent {
  private readonly fb = inject(FormBuilder);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly itemService = inject(WorkItemService);
  private readonly ws = inject(WorkspaceService);

  readonly workspaceId = signal<string>('');
  readonly itemId = signal<string | null>(null);
  readonly isEdit = signal<boolean>(false);
  readonly itemVersion = signal<number>(1);

  readonly members = signal<WorkspaceMember[]>([]);
  readonly loading = signal<boolean>(false);
  readonly submitting = signal<boolean>(false);
  readonly errorMessage = signal<string | null>(null);
  readonly versionConflict = signal<boolean>(false);

  readonly isBug = computed(() => this.itemForm?.get('type')?.value === 'BUG');

  readonly itemForm: FormGroup = this.fb.group({
    type: ['TASK', [Validators.required]],
    title: ['', [Validators.required, Validators.minLength(3), Validators.maxLength(180)]],
    description: [''],
    status: ['TODO', [Validators.required]],
    priority: ['MEDIUM', [Validators.required]],
    assignee_id: [null],
    due_date: [null],
    steps_to_reproduce: [''],
    expected_result: [''],
    actual_result: ['']
  });

  // Kunci cache agar item yang sama tidak di-load berulang.
  private loadedKey = '';

  constructor() {
    combineLatest([
      workspaceIdStream(this.route, () => this.ws.activeWorkspaceId()),
      this.route.paramMap
    ])
      .pipe(takeUntilDestroyed())
      .subscribe(([wid, params]) => {
        if (!wid) {
          this.loading.set(false);
          this.errorMessage.set('Workspace ID tidak ditemukan dalam URL.');
          return;
        }

        if (wid !== this.workspaceId()) {
          this.workspaceId.set(wid);
          this.loadMembers();
        }

        const id = params.get('itemId');
        if (!id) return;

        this.itemId.set(id);
        this.isEdit.set(true);
        const key = `${wid}:${id}`;
        if (key !== this.loadedKey) {
          this.loadedKey = key;
          this.loadExistingItem();
        }
      });
  }

  titleError(): string | null {
    const control = this.itemForm.get('title');
    if (!control?.touched) return null;
    if (control.hasError('required')) return 'Judul wajib diisi.';
    if (control.hasError('minlength')) return 'Judul minimal 3 karakter.';
    if (control.hasError('maxlength')) return 'Judul maksimal 180 karakter.';
    return null;
  }

  backLink(): string[] {
    const wid = this.workspaceId();
    return wid ? ['/app/w', wid, 'items'] : ['/app/workspaces'];
  }

  reloadItem(): void {
    this.versionConflict.set(false);
    this.errorMessage.set(null);
    this.loadedKey = '';
    this.loadExistingItem();
  }

  onSubmit(): void {
    if (this.itemForm.invalid) {
      this.itemForm.markAllAsTouched();
      return;
    }

    const wid = this.workspaceId();
    if (!wid) {
      this.errorMessage.set('Workspace ID tidak valid. Tidak dapat menyimpan pekerjaan.');
      return;
    }

    this.submitting.set(true);
    this.errorMessage.set(null);
    this.versionConflict.set(false);

    const formVal = this.itemForm.value;
    const isBug = formVal.type === 'BUG';

    if (this.isEdit()) {
      const payload: UpdateItemPayload = {
        version: this.itemVersion(),
        type: formVal.type,
        title: formVal.title,
        description: formVal.description,
        status: formVal.status,
        priority: formVal.priority,
        assignee_id: formVal.assignee_id || null,
        due_date: formVal.due_date || null,
        steps_to_reproduce: isBug ? formVal.steps_to_reproduce : '',
        expected_result: isBug ? formVal.expected_result : '',
        actual_result: isBug ? formVal.actual_result : ''
      };

      this.itemService
        .updateItem(wid, this.itemId()!, payload)
        .pipe(finalize(() => this.submitting.set(false)))
        .subscribe({
          next: res => this.router.navigate(['/app/w', wid, 'items', res.data.id]),
          error: err => {
            if (err?.status === 409 || err?.error?.error?.code === 'VERSION_CONFLICT') {
              this.versionConflict.set(true);
              this.errorMessage.set(
                'Item telah diperbarui oleh pengguna lain. Silakan muat ulang data.'
              );
            } else {
              this.errorMessage.set(
                err?.error?.error?.message || 'Gagal memperbarui pekerjaan.'
              );
            }
          }
        });
      return;
    }

    const payload: CreateItemPayload = {
      type: formVal.type,
      title: formVal.title,
      description: formVal.description,
      status: formVal.status,
      priority: formVal.priority,
      assignee_id: formVal.assignee_id || null,
      due_date: formVal.due_date || null,
      steps_to_reproduce: isBug ? formVal.steps_to_reproduce : '',
      expected_result: isBug ? formVal.expected_result : '',
      actual_result: isBug ? formVal.actual_result : ''
    };

    this.itemService
      .createItem(wid, payload)
      .pipe(finalize(() => this.submitting.set(false)))
      .subscribe({
        next: res => this.router.navigate(['/app/w', wid, 'items', res.data.id]),
        error: err =>
          this.errorMessage.set(
            err?.error?.error?.message || 'Gagal membuat pekerjaan baru.'
          )
      });
  }

  private loadMembers(): void {
    this.itemService.getWorkspaceMembers(this.workspaceId()).subscribe({
      next: res => this.members.set(res.data || []),
      error: () => {
        // Daftar anggota yang gagal dimuat tidak menghalangi pengisian form.
      }
    });
  }

  private loadExistingItem(): void {
    const wid = this.workspaceId();
    const id = this.itemId();
    if (!wid || !id) return;

    this.loading.set(true);
    this.itemService
      .getItem(wid, id)
      .pipe(finalize(() => this.loading.set(false)))
      .subscribe({
        next: res => {
          const item: WorkItem = res.data;
          this.itemVersion.set(item.version);
          this.itemForm.patchValue({
            type: item.type,
            title: item.title,
            description: item.description,
            status: item.status,
            priority: item.priority,
            assignee_id: item.assignee_id,
            due_date: item.due_date,
            steps_to_reproduce: item.steps_to_reproduce || '',
            expected_result: item.expected_result || '',
            actual_result: item.actual_result || ''
          });
        },
        error: err =>
          this.errorMessage.set(
            err?.error?.error?.message || 'Gagal memuat detail pekerjaan.'
          )
      });
  }
}
