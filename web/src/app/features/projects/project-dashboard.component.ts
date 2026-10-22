import { ChangeDetectionStrategy, Component, DestroyRef, inject, OnInit, signal } from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { catchError, forkJoin, map, Observable, of, switchMap, throwError } from 'rxjs';
import { ApiClient } from '../../core/api/api-client';
import { AgentSummary, ApiPage, BrainSummary, ProjectSummary, RunSummary, TaskSummary } from '../../core/api/api-models';

interface ProjectDashboardData {
  project: ProjectSummary;
  tasks: TaskSummary[];
  runs: RunSummary[];
  brain: BrainSummary | null;
  agents: AgentSummary[];
}

type ProjectDashboardState =
  | { kind: 'loading' }
  | { kind: 'error' }
  | { kind: 'loaded'; data: ProjectDashboardData };

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  selector: 'app-project-dashboard',
  template: `
    <p class="back-link"><a routerLink="/projects" i18n="@@projectDashboard.back">← All projects</a></p>
    @let currentState = state();
    @if (currentState.kind === 'loading') {
      <p class="notice" role="status" i18n="@@projectDashboard.loading">Loading project overview…</p>
    } @else if (currentState.kind === 'error') {
      <section class="notice notice-error" role="alert">
        <p i18n="@@projectDashboard.loadError">The project overview could not be loaded. Check that the local API is running, then try again.</p>
        <button class="button" type="button" (click)="retry()" i18n="@@common.retry">Retry</button>
      </section>
    } @else if (currentState.kind === 'loaded') {
      @let data = currentState.data;
      <header class="page-header">
        <p class="eyebrow" i18n="@@projectDashboard.workspaceLabel">Project workspace</p>
        <h1>{{ data.project.name }}</h1>
        <p class="page-description">{{ data.project.description }}</p>
        <span class="project-status">{{ data.project.status }}</span>
      </header>

      <section class="summary-grid" aria-label="Project status" i18n-aria-label="@@projectDashboard.summaryLabel">
        <article class="summary-card">
          <h2 i18n="@@projectDashboard.brain">Brain</h2>
          @if (data.brain; as brain) {
            <p class="metric">{{ brain.active ? activeLabel : inactiveLabel }}</p>
            <p class="secondary" i18n="@@projectDashboard.brainAgent">Agent: {{ brain.agent_id }}</p>
          } @else {
            <p class="metric metric-warning" i18n="@@projectDashboard.missingBrain">Not configured</p>
          }
        </article>
        <article class="summary-card">
          <h2 i18n="@@projectDashboard.agents">Assigned agents</h2>
          @if (data.agents.length > 0) {
            <ul class="compact-list">
              @for (agent of data.agents; track agent.id) {
                <li><span>{{ agent.name }}</span><span class="tag">{{ agent.status }}</span></li>
              }
            </ul>
          } @else {
            <p class="metric" i18n="@@projectDashboard.noAgents">No agents are assigned to this project.</p>
          }
        </article>
        <article class="summary-card">
          <h2 i18n="@@projectDashboard.tasks">Tasks</h2>
          <p class="metric">{{ data.tasks.length }}</p>
          <p class="secondary" i18n="@@projectDashboard.blockedCount">{{ blockedCount(data.tasks) }} blocked</p>
        </article>
        <article class="summary-card">
          <h2 i18n="@@projectDashboard.runs">Recent runs</h2>
          <p class="metric">{{ data.runs.length }}</p>
          <p class="secondary" i18n="@@projectDashboard.failedRuns">{{ failedRunCount(data.runs) }} failed</p>
        </article>
      </section>

      <div class="detail-grid">
        <section class="detail-card" aria-labelledby="blocked-heading">
          <h2 id="blocked-heading" i18n="@@projectDashboard.blockedHeading">Blocked work</h2>
          @if (blockedTasks(data.tasks).length > 0) {
            <ul class="work-list">
              @for (task of blockedTasks(data.tasks); track task.id) {
                <li>
                  <span>{{ task.title }}</span>
                  <span class="tag tag-danger">{{ task.status }}</span>
                </li>
              }
            </ul>
          } @else {
            <p class="secondary" i18n="@@projectDashboard.noBlockedTasks">There are no blocked tasks.</p>
          }
        </section>

        <section class="detail-card" aria-labelledby="runs-heading">
          <h2 id="runs-heading" i18n="@@projectDashboard.runsHeading">Latest runs</h2>
          @if (data.runs.length > 0) {
            <ul class="work-list">
              @for (run of data.runs.slice(0, 5); track run.id) {
                <li>
                  <span class="run-copy">
                    <span>{{ run.model }}</span>
                    <span class="secondary">{{ run.task_id || run.agent_id }}</span>
                  </span>
                  <span class="tag" [class.tag-danger]="run.status === 'FAILED'">{{ run.status }}</span>
                </li>
              }
            </ul>
          } @else {
            <p class="secondary" i18n="@@projectDashboard.noRuns">No runs have been recorded for this project.</p>
          }
        </section>
      </div>
    }
  `,
  styles: `
    :host { display: block; max-width: 78rem; margin-inline: auto; }
    .back-link { margin: .5rem 0 1rem; }
    a { color: var(--boaird-accent); font-weight: 700; text-underline-offset: .2em; }
    a:focus-visible, .button:focus-visible { outline: 3px solid var(--boaird-accent); outline-offset: 3px; }
    .page-header { margin-block: 1.5rem 2rem; }
    .eyebrow { color: var(--boaird-accent); font-size: .75rem; font-weight: 700; letter-spacing: .12em; margin: 0 0 .75rem; text-transform: uppercase; }
    h1 { color: var(--boaird-ink); font-size: clamp(2rem, 4vw, 3rem); letter-spacing: -.04em; line-height: 1.1; margin: 0; }
    h2 { color: var(--boaird-ink); font-size: 1rem; margin: 0 0 1rem; }
    .page-description, .secondary { color: var(--boaird-muted); line-height: 1.55; }
    .page-description { margin: .75rem 0 1rem; max-width: 48rem; }
    .project-status, .tag { background: var(--boaird-status-bg); border-radius: 999px; color: var(--boaird-ink); display: inline-block; font-size: .75rem; font-weight: 700; padding: .35rem .65rem; }
    .summary-grid { display: grid; gap: 1rem; grid-template-columns: repeat(auto-fit, minmax(min(100%, 15rem), 1fr)); }
    .summary-card, .detail-card { background: var(--boaird-surface); border: 1px solid var(--boaird-border); border-radius: 1rem; padding: 1.25rem; }
    .metric { color: var(--boaird-ink); font-size: 1.35rem; font-weight: 700; margin: 0; }
    .metric-warning, .tag-danger { color: var(--boaird-danger); }
    .secondary { font-size: .9rem; margin: .5rem 0 0; }
    .compact-list, .work-list { list-style: none; margin: 0; padding: 0; }
    .compact-list li, .work-list li { align-items: center; border-top: 1px solid var(--boaird-border); display: flex; gap: 1rem; justify-content: space-between; padding: .8rem 0; }
    .compact-list li:first-child, .work-list li:first-child { border-top: 0; padding-top: 0; }
    .detail-grid { display: grid; gap: 1rem; grid-template-columns: repeat(auto-fit, minmax(min(100%, 22rem), 1fr)); margin-block: 1rem 2rem; }
    .run-copy { display: grid; gap: .2rem; min-width: 0; }
    .run-copy .secondary { overflow-wrap: anywhere; }
    .notice { background: var(--boaird-surface); border: 1px solid var(--boaird-border); border-radius: .9rem; color: var(--boaird-muted); padding: 1.25rem; }
    .notice-error { border-color: var(--boaird-danger); }
    .button { background: var(--boaird-ink); border: 0; border-radius: .5rem; color: var(--boaird-on-ink); cursor: pointer; font: inherit; font-weight: 700; padding: .65rem 1rem; }
  `,
})
export class ProjectDashboardComponent implements OnInit {
  private readonly api = inject(ApiClient);
  private readonly route = inject(ActivatedRoute);
  private readonly destroyRef = inject(DestroyRef);
  private projectId = '';

  protected readonly state = signal<ProjectDashboardState>({ kind: 'loading' });
  protected readonly activeLabel = $localize`:@@projectDashboard.active:Active`;
  protected readonly inactiveLabel = $localize`:@@projectDashboard.inactive:Inactive`;

  ngOnInit(): void {
    this.route.paramMap.pipe(
      map((params) => params.get('projectId') ?? ''),
      switchMap((projectId) => {
        this.projectId = projectId;
        return this.loadProject(projectId);
      }),
      takeUntilDestroyed(this.destroyRef),
    ).subscribe((state) => this.state.set(state));
  }

  protected retry(): void {
    if (!this.projectId) return;
    this.state.set({ kind: 'loading' });
    this.loadProject(this.projectId).pipe(takeUntilDestroyed(this.destroyRef)).subscribe((state) => this.state.set(state));
  }

  protected blockedTasks(tasks: TaskSummary[]): TaskSummary[] {
    return tasks.filter((task) => task.status === 'BLOCKED');
  }

  protected blockedCount(tasks: TaskSummary[]): number {
    return this.blockedTasks(tasks).length;
  }

  protected failedRunCount(runs: RunSummary[]): number {
    return runs.filter((run) => run.status === 'FAILED').length;
  }

  private loadProject(projectId: string): Observable<ProjectDashboardState> {
    if (!projectId) return of({ kind: 'error' });

    const encodedID = encodeURIComponent(projectId);
    const optionalBrain = this.api.get<BrainSummary>(`projects/${encodedID}/brain`).pipe(
      catchError((error: unknown) => error instanceof HttpErrorResponse && error.status === 404 ? of(null) : throwError(() => error)),
    );

    return forkJoin({
      project: this.api.get<ProjectSummary>(`projects/${encodedID}`),
      tasks: this.api.getAllPages<TaskSummary>(`projects/${encodedID}/tasks`),
      runs: this.api.get<ApiPage<RunSummary>>(`projects/${encodedID}/runs?limit=5&offset=0`).pipe(map((page) => page.items)),
      brain: optionalBrain,
    }).pipe(
      switchMap((overview) => {
        const agentIDs = [...new Set([
          ...overview.tasks.map((task) => task.assigned_to),
          overview.brain?.agent_id,
        ].filter((id): id is string => Boolean(id)))];
        const agents = agentIDs.map((id) => this.api.get<AgentSummary>(`agents/${encodeURIComponent(id)}`).pipe(
          catchError((error: unknown) => error instanceof HttpErrorResponse && error.status === 404 ? of(null) : throwError(() => error)),
        ));
        const agentResults = agents.length > 0 ? forkJoin(agents) : of([]);
        return agentResults.pipe(map((loadedAgents) => ({
          kind: 'loaded',
          data: { ...overview, agents: loadedAgents.filter((agent): agent is AgentSummary => agent !== null) },
        }) as ProjectDashboardState));
      }),
      catchError(() => of({ kind: 'error' } as ProjectDashboardState)),
    );
  }

}
