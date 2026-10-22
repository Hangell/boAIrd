import { ChangeDetectionStrategy, Component, DestroyRef, inject, OnInit, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { catchError, distinctUntilChanged, EMPTY, finalize, map, of, switchMap } from 'rxjs';
import { ApiClient } from '../../core/api/api-client';
import { ProjectSummary, TaskSummary } from '../../core/api/api-models';

interface TransitionForm {
  targetStatus: string;
  reason: string;
}

const taskStatuses = ['BACKLOG', 'READY', 'WORKING', 'REVIEW', 'HUMAN_REVIEW', 'BLOCKED', 'FAILED', 'DONE', 'CANCELLED'];

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink],
  selector: 'app-task-board',
  template: `
    <header class="page-header">
      <p class="eyebrow" i18n="@@tasks.workspaceLabel">boAIrd workspace</p>
      <h1 i18n="@@tasks.title">Kanban board</h1>
      <p class="page-description" i18n="@@tasks.description">View and move tasks through the backend-validated workflow.</p>
    </header>

    @if (state() === 'loading-projects') {
      <p class="notice" role="status" i18n="@@tasks.loadingProjects">Loading projects…</p>
    } @else if (state() === 'no-projects') {
      <p class="notice" i18n="@@tasks.noProjects">Create a project before viewing its task board.</p>
    } @else {
      <label class="project-picker">
        <span i18n="@@tasks.chooseProject">Project</span>
        <select [value]="selectedProjectId()" (change)="chooseProject($event)">
          <option value="" i18n="@@tasks.chooseProjectPlaceholder">Choose a project</option>
          @for (project of projects(); track project.id) {
            <option [value]="project.id">{{ project.name }}</option>
          }
        </select>
      </label>

      @if (state() === 'loading-tasks') {
        <p class="notice" role="status" i18n="@@tasks.loading">Loading tasks…</p>
      } @else if (state() === 'error') {
        <section class="notice notice-error" role="alert">
          <p i18n="@@tasks.loadError">Tasks could not be loaded. Check that the local API is running, then try again.</p>
          <button class="button" type="button" (click)="retry()" i18n="@@common.retry">Retry</button>
        </section>
      } @else if (state() === 'select-project') {
        <p class="notice" i18n="@@tasks.selectProject">Choose a project to load its board.</p>
      } @else if (state() === 'loaded') {
        <p class="live-message" aria-live="polite">{{ message() }}</p>
        <div class="board" aria-label="Task workflow" i18n-aria-label="@@tasks.boardLabel">
          @for (status of statuses; track status) {
            <section class="column" [attr.aria-labelledby]="'column-' + status">
              <h2 [id]="'column-' + status">{{ statusLabel(status) }} <span class="count">{{ tasksFor(status).length }}</span></h2>
              <div class="column-cards">
                @for (task of tasksFor(status); track task.id) {
                  <article class="task-card">
                    <div class="task-heading">
                      <a class="task-title" [routerLink]="['/tasks', task.id]" [queryParams]="{ projectId: selectedProjectId() }">{{ task.title }}</a>
                      <span class="priority">{{ task.priority }}</span>
                    </div>
                    <p class="task-meta">
                      <span>{{ statusLabel(task.status) }}</span>
                      @if (task.assigned_to; as agentId) {
                        <span i18n="@@tasks.assignee">Agent: {{ agentId }}</span>
                      } @else {
                        <span i18n="@@tasks.unassigned">Unassigned</span>
                      }
                    </p>
                    <form class="transition-form" (ngSubmit)="transition(task)">
                      <label>
                        <span i18n="@@tasks.moveTo">Move to</span>
                        <select [(ngModel)]="formFor(task.id).targetStatus" [name]="'target-' + task.id" required>
                          <option value="" i18n="@@tasks.chooseStatus">Choose a status</option>
                          @for (target of statuses; track target) {
                            @if (target !== task.status) {
                              <option [value]="target">{{ statusLabel(target) }}</option>
                            }
                          }
                        </select>
                      </label>
                      <label>
                        <span i18n="@@tasks.reason">Reason</span>
                        <input [(ngModel)]="formFor(task.id).reason" [name]="'reason-' + task.id" maxlength="2000" required>
                      </label>
                      <button class="button" type="submit" [disabled]="savingTaskId() === task.id" i18n="@@tasks.move">Move task</button>
                    </form>
                  </article>
                } @empty {
                  <p class="empty-column" i18n="@@tasks.emptyColumn">No tasks in this state.</p>
                }
              </div>
            </section>
          }
        </div>
      }
    }
  `,
  styles: `
    :host { display: block; max-width: 100%; }
    .page-header { margin-block: 1.5rem 2rem; }
    .eyebrow { color: var(--boaird-accent); font-size: .75rem; font-weight: 700; letter-spacing: .12em; margin: 0 0 .75rem; text-transform: uppercase; }
    h1 { color: var(--boaird-ink); font-size: clamp(2rem, 4vw, 3rem); letter-spacing: -.04em; line-height: 1.1; margin: 0; }
    .page-description, .task-meta { color: var(--boaird-muted); line-height: 1.5; }
    .page-description { margin: .75rem 0 0; }
    .project-picker, .transition-form label { display: grid; gap: .4rem; }
    .project-picker { color: var(--boaird-ink); font-weight: 700; max-width: 28rem; }
    select, input { background: var(--boaird-surface); border: 1px solid var(--boaird-border); border-radius: .5rem; color: var(--boaird-ink); font: inherit; min-height: 2.5rem; padding: .5rem; width: 100%; }
    select:focus-visible, input:focus-visible, a:focus-visible, .button:focus-visible { outline: 3px solid var(--boaird-accent); outline-offset: 2px; }
    .board { display: grid; gap: .8rem; grid-auto-columns: minmax(16rem, 1fr); grid-auto-flow: column; margin-block: 1rem 2rem; overflow-x: auto; overscroll-behavior-inline: contain; padding: .25rem .1rem 1rem; }
    .column { background: var(--boaird-surface-muted); border: 1px solid var(--boaird-border); border-radius: .9rem; min-height: 22rem; padding: .8rem; }
    .column h2 { align-items: center; color: var(--boaird-ink); display: flex; font-size: .85rem; justify-content: space-between; margin: 0 0 .8rem; }
    .count { background: var(--boaird-status-bg); border-radius: 999px; font-size: .7rem; padding: .2rem .5rem; }
    .column-cards { display: grid; gap: .7rem; }
    .task-card { background: var(--boaird-surface); border: 1px solid var(--boaird-border); border-radius: .7rem; padding: .8rem; }
    .task-heading { align-items: start; display: flex; gap: .5rem; justify-content: space-between; }
    .task-title { color: var(--boaird-ink); font-weight: 700; overflow-wrap: anywhere; text-underline-offset: .2em; }
    .priority { background: var(--boaird-status-bg); border-radius: 999px; color: var(--boaird-ink); flex: 0 0 auto; font-size: .65rem; font-weight: 700; padding: .25rem .45rem; }
    .task-meta { display: grid; font-size: .75rem; gap: .2rem; margin: .6rem 0; overflow-wrap: anywhere; }
    .transition-form { border-top: 1px solid var(--boaird-border); display: grid; gap: .6rem; padding-top: .7rem; }
    .transition-form label { color: var(--boaird-muted); font-size: .75rem; }
    .button { background: var(--boaird-ink); border: 0; border-radius: .5rem; color: var(--boaird-on-ink); cursor: pointer; font: inherit; font-size: .8rem; font-weight: 700; min-height: 2.4rem; padding: .5rem .75rem; }
    .button:disabled { cursor: wait; opacity: .6; }
    .empty-column { color: var(--boaird-muted); font-size: .8rem; margin: .5rem 0; }
    .notice { background: var(--boaird-surface); border: 1px solid var(--boaird-border); border-radius: .9rem; color: var(--boaird-muted); margin-block: 1rem; padding: 1.25rem; }
    .notice-error { border-color: var(--boaird-danger); }
    .live-message { color: var(--boaird-muted); min-height: 1.5rem; }
  `,
})
export class TaskBoardComponent implements OnInit {
  private readonly api = inject(ApiClient);
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly destroyRef = inject(DestroyRef);
  private readonly transitionForms = new Map<string, TransitionForm>();
  private projectsLoaded = false;
  private projectsError = false;

  protected readonly statuses = taskStatuses;
  protected readonly projects = signal<ProjectSummary[]>([]);
  protected readonly selectedProjectId = signal('');
  protected readonly tasks = signal<TaskSummary[]>([]);
  protected readonly state = signal<'loading-projects' | 'no-projects' | 'select-project' | 'loading-tasks' | 'error' | 'loaded'>('loading-projects');
  protected readonly message = signal('');
  protected readonly savingTaskId = signal('');

  ngOnInit(): void {
    this.loadProjects();

    this.route.queryParamMap.pipe(
      map((params) => params.get('projectId') ?? ''),
      distinctUntilChanged(),
      switchMap((projectId) => {
        this.selectedProjectId.set(projectId);
        if (!projectId) {
          this.state.set(!this.projectsLoaded ? 'loading-projects' : this.projectsError ? 'error' : this.projects().length === 0 ? 'no-projects' : 'select-project');
          return EMPTY;
        }
        this.state.set('loading-tasks');
        this.message.set('');
        return this.api.getAllPages<TaskSummary>(`projects/${encodeURIComponent(projectId)}/tasks`).pipe(
          map((tasks) => ({ tasks, error: false as const })),
          catchError(() => of({ tasks: [] as TaskSummary[], error: true as const })),
        );
      }),
      takeUntilDestroyed(this.destroyRef),
    ).subscribe((result) => {
      if (result.error) {
        this.state.set('error');
        return;
      }
      this.tasks.set(result.tasks);
      this.state.set('loaded');
    });
  }

  protected statusLabel(status: string): string {
    const labels: Record<string, string> = {
      BACKLOG: $localize`:@@tasks.status.backlog:Backlog`,
      READY: $localize`:@@tasks.status.ready:Ready`,
      WORKING: $localize`:@@tasks.status.working:In progress`,
      REVIEW: $localize`:@@tasks.status.review:Review`,
      HUMAN_REVIEW: $localize`:@@tasks.status.humanReview:Human review`,
      BLOCKED: $localize`:@@tasks.status.blocked:Blocked`,
      FAILED: $localize`:@@tasks.status.failed:Failed`,
      DONE: $localize`:@@tasks.status.done:Done`,
      CANCELLED: $localize`:@@tasks.status.cancelled:Cancelled`,
    };
    return labels[status] ?? status;
  }

  protected tasksFor(status: string): TaskSummary[] {
    return this.tasks().filter((task) => task.status === status);
  }

  protected formFor(taskId: string): TransitionForm {
    let form = this.transitionForms.get(taskId);
    if (!form) {
      form = { targetStatus: '', reason: '' };
      this.transitionForms.set(taskId, form);
    }
    return form;
  }

  protected chooseProject(event: Event): void {
    const projectId = (event.target as HTMLSelectElement).value;
    void this.router.navigate([], { relativeTo: this.route, queryParams: { projectId: projectId || null }, queryParamsHandling: 'merge' });
  }

  protected retry(): void {
    const projectId = this.selectedProjectId();
    if (!projectId) {
      this.loadProjects();
      return;
    }
    this.state.set('loading-tasks');
    this.api.getAllPages<TaskSummary>(`projects/${encodeURIComponent(projectId)}/tasks`).pipe(
      catchError(() => of(null)),
      takeUntilDestroyed(this.destroyRef),
    ).subscribe((tasks) => {
      if (tasks === null) {
        this.state.set('error');
        return;
      }
      this.tasks.set(tasks);
      this.state.set('loaded');
    });
  }

  protected transition(task: TaskSummary): void {
    const form = this.formFor(task.id);
    const targetStatus = form.targetStatus;
    const reason = form.reason.trim();
    if (!targetStatus || !reason || this.savingTaskId()) return;

    this.savingTaskId.set(task.id);
    this.message.set('');
    this.api.post<{ status: string; reason: string }, TaskSummary>(`tasks/${encodeURIComponent(task.id)}/transitions`, { status: targetStatus, reason }).pipe(
      switchMap(() => this.api.getAllPages<TaskSummary>(`projects/${encodeURIComponent(this.selectedProjectId())}/tasks`)),
      catchError(() => {
        this.message.set($localize`:@@tasks.transitionError:The transition could not be confirmed. Reload the board to check the latest API state.`);
        return EMPTY;
      }),
      finalize(() => this.savingTaskId.set('')),
      takeUntilDestroyed(this.destroyRef),
    ).subscribe((tasks) => {
      this.tasks.set(tasks);
      form.targetStatus = '';
      form.reason = '';
      this.message.set($localize`:@@tasks.transitionSuccess:Task state refreshed from the API.`);
    });
  }

  private loadProjects(): void {
    this.state.set('loading-projects');
    this.projectsLoaded = false;
    this.projectsError = false;
    this.api.getAllPages<ProjectSummary>('projects').pipe(
      catchError(() => of(null)),
      takeUntilDestroyed(this.destroyRef),
    ).subscribe((projects) => {
      this.projectsLoaded = true;
      if (projects === null) {
        this.projectsError = true;
        if (!this.selectedProjectId()) this.state.set('error');
        return;
      }
      this.projects.set(projects);
      if (!this.selectedProjectId()) this.state.set(projects.length === 0 ? 'no-projects' : 'select-project');
    });
  }
}
