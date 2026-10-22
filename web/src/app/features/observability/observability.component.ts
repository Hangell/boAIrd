import { ChangeDetectionStrategy, Component, DestroyRef, inject, OnInit, signal } from '@angular/core';
import { DatePipe } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { forkJoin, map, Observable, of, startWith, Subject, switchMap } from 'rxjs';
import { ApiClient } from '../../core/api/api-client';
import { AgentSummary, ProjectSummary, RunAuditEvent, RunSummary, TaskAuditEvent, TaskDetail, TaskSummary } from '../../core/api/api-models';

interface ActivityItem {
  id: string;
  at: string;
  type: string;
  subject: string;
  status: string;
  actor: string;
}

interface ObservabilityData {
  projects: ProjectSummary[];
  selectedProject: string;
  runs: RunSummary[];
  tasks: TaskSummary[];
  agents: AgentSummary[];
  activity: ActivityItem[];
}

type ViewState = { kind: 'loading' } | { kind: 'error' } | { kind: 'loaded'; data: ObservabilityData };

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DatePipe],
  selector: 'app-observability',
  template: `
    @let current = state();
    @if (current.kind === 'loading') { <p class="notice" role="status" i18n="@@observability.loading">Loading project operations…</p> }
    @else if (current.kind === 'error') {
      <section class="notice notice-error" role="alert"><p i18n="@@observability.error">Operational data could not be loaded. Retry the request.</p><button class="button" (click)="reload()" i18n="@@common.retry">Retry</button></section>
    } @else {
      @let data = current.data;
      <header class="page-header"><p class="eyebrow" i18n="@@observability.eyebrow">Project operations</p><h1>{{ isLogs ? logsTitle : runsTitle }}</h1>
        <p class="page-description">{{ isLogs ? logsDescription : runsDescription }}</p></header>
      <section class="filters" aria-label="Operational filters" i18n-aria-label="@@observability.filtersLabel">
        <label><span i18n="@@observability.project">Project</span><select [value]="data.selectedProject" (change)="selectProject($event)">
          <option value="" i18n="@@observability.chooseProject">Choose a project</option>
          @for (project of data.projects; track project.id) { <option [value]="project.id">{{ project.name }}</option> }
        </select></label>
        <label><span i18n="@@observability.search">Search</span><input type="search" [value]="search()" (input)="setSearch($event)" placeholder="Task, Agent, run or event" i18n-placeholder="@@observability.searchPlaceholder"></label>
        @if (!isLogs) { <label><span i18n="@@observability.status">Status</span><select [value]="statusFilter()" (change)="setStatus($event)"><option value="" i18n="@@observability.allStatuses">All statuses</option>@for (status of statuses; track status) { <option [value]="status">{{ status }}</option> }</select></label> }
      </section>
      @if (!data.selectedProject) { <p class="notice" i18n="@@observability.selectProject">Select a project to inspect its operational data.</p> }
      @else if (!isLogs) {
        <section class="metrics" aria-label="Project metrics" i18n-aria-label="@@observability.metricsLabel">
          <article><h2 i18n="@@observability.agents">Agents</h2><p class="metric">{{ data.agents.length }}</p><ul class="agent-list">@for (agent of data.agents; track agent.id) { <li><span>{{ agent.name }}</span><span class="tag">{{ agent.status }}</span></li> }</ul></article>
          <article><h2 i18n="@@observability.runs">Runs</h2><p class="metric">{{ data.runs.length }}</p><p class="secondary">{{ completedThisWeek(data.runs) }} <span i18n="@@observability.completedWeek">completed in the last 7 days</span></p></article>
          <article><h2 i18n="@@observability.failures">Failures</h2><p class="metric" [class.danger]="failedCount(data.runs) > 0">{{ failedCount(data.runs) }}</p><p class="secondary" i18n="@@observability.failedRuns">Failed runs in the loaded history</p></article>
          <article><h2 i18n="@@observability.blocked">Blocked work</h2><p class="metric" [class.danger]="blockedCount(data.tasks) > 0">{{ blockedCount(data.tasks) }}</p><p class="secondary" i18n="@@observability.blockedTasks">Tasks currently blocked</p></article>
        </section>
        <section class="panel"><h2 i18n="@@observability.runHistory">Run history</h2>
          @if (filteredRuns(data.runs).length) { <ul class="entries">@for (run of filteredRuns(data.runs); track run.id) { <li><div class="entry-main"><strong>{{ run.model || run.runtime }}</strong><span class="secondary">{{ run.task_id || run.agent_id }} · {{ run.started_at | date:'medium' }}</span>@if (run.output) { <p class="output">{{ run.output }}</p> }@if (run.error) { <p class="output danger">{{ run.error }}</p> }</div><span class="tag" [class.danger]="run.status === 'FAILED'">{{ run.status }}</span></li> }</ul> }
          @else { <p class="secondary" i18n="@@observability.noRuns">No runs match these filters.</p> }
        </section>
      } @else {
        <section class="panel"><h2 i18n="@@observability.activity">Project activity</h2>
          @if (filteredActivity(data.activity).length) { <ul class="entries">@for (item of filteredActivity(data.activity); track item.id) { <li><div class="entry-main"><strong>{{ item.type }}</strong><span class="secondary">{{ item.subject }} · {{ item.actor }}</span></div><div class="entry-meta"><span class="tag">{{ item.status }}</span><time class="secondary">{{ item.at | date:'medium' }}</time></div></li> }</ul> }
          @else { <p class="secondary" i18n="@@observability.noActivity">No activity matches these filters.</p> }
          <p class="footnote" i18n="@@observability.activitySource">Activity uses redacted run audit events and task audit history returned by the project API. Task history includes the 50 most recently updated tasks.</p>
        </section>
      }
    }
  `,
  styles: `
    :host { display:block; max-width:78rem; margin-inline:auto; }
    .page-header { margin-block:1.5rem 2rem; }.eyebrow { color:var(--boaird-accent); font-size:.75rem; font-weight:700; letter-spacing:.12em; margin:0 0 .75rem; text-transform:uppercase; }
    h1 { color:var(--boaird-ink); font-size:clamp(2rem,4vw,3rem); letter-spacing:-.04em; margin:0; } h2 { color:var(--boaird-ink); font-size:1rem; margin:0 0 1rem; }
    .page-description,.secondary,.footnote { color:var(--boaird-muted); line-height:1.55; }.page-description { max-width:48rem; margin:.75rem 0 0; }
    .filters { align-items:end; display:flex; flex-wrap:wrap; gap:1rem; margin-bottom:1rem; }.filters label { color:var(--boaird-muted); display:grid; font-size:.85rem; font-weight:700; gap:.4rem; min-width:12rem; }
    input,select { background:var(--boaird-surface); border:1px solid var(--boaird-border); border-radius:.55rem; color:var(--boaird-ink); font:inherit; min-height:2.7rem; padding:.55rem .7rem; }
    .metrics { display:grid; gap:1rem; grid-template-columns:repeat(auto-fit,minmax(min(100%,13rem),1fr)); margin-block:1rem; }.metrics article,.panel { background:var(--boaird-surface); border:1px solid var(--boaird-border); border-radius:1rem; padding:1.25rem; }
    .metric { color:var(--boaird-ink); font-size:1.5rem; font-weight:700; margin:0; }.danger { color:var(--boaird-danger)!important; }.agent-list,.entries { list-style:none; margin:0; padding:0; }.agent-list li,.entries li { align-items:center; border-top:1px solid var(--boaird-border); display:flex; gap:1rem; justify-content:space-between; padding:.8rem 0; }.entries li:first-child,.agent-list li:first-child { border-top:0; padding-top:0; }
    .tag { background:var(--boaird-status-bg); border-radius:999px; color:var(--boaird-ink); flex:0 0 auto; font-size:.75rem; font-weight:700; padding:.35rem .65rem; }.entry-main { display:grid; gap:.25rem; min-width:0; }.entry-main .secondary { overflow-wrap:anywhere; }.entry-meta { display:grid; gap:.35rem; justify-items:end; }.output { color:var(--boaird-muted); margin:.4rem 0 0; max-height:8rem; overflow:auto; white-space:pre-wrap; }.footnote { font-size:.8rem; margin:1rem 0 0; }.notice { background:var(--boaird-surface); border:1px solid var(--boaird-border); border-radius:.9rem; color:var(--boaird-muted); padding:1.25rem; }.notice-error { border-color:var(--boaird-danger); }.button { background:var(--boaird-ink); border:0; border-radius:.5rem; color:var(--boaird-on-ink); cursor:pointer; font:inherit; font-weight:700; padding:.65rem 1rem; }
  `,
})
export class ObservabilityComponent implements OnInit {
  private readonly api = inject(ApiClient);
  private readonly route = inject(ActivatedRoute);
  private readonly destroyRef = inject(DestroyRef);
  private readonly refresh = new Subject<void>();
  private readonly projectSelection = new Subject<string>();
  protected readonly state = signal<ViewState>({ kind: 'loading' });
  protected readonly search = signal('');
  protected readonly statusFilter = signal('');
  protected readonly statuses = ['PENDING', 'RUNNING', 'SUCCESS', 'FAILED', 'CANCELLED'];
  protected readonly isLogs = this.route.snapshot.data['view'] === 'logs';
  protected readonly runsTitle = $localize`:@@observability.runsTitle:Run history`;
  protected readonly logsTitle = $localize`:@@observability.logsTitle:Activity logs`;
  protected readonly runsDescription = $localize`:@@observability.runsDescription:Inspect project throughput, Agent availability, failures, blocked work, and run results.`;
  protected readonly logsDescription = $localize`:@@observability.logsDescription:Review timestamped task and run audit events for one project.`;

  ngOnInit(): void {
    this.projectSelection.pipe(switchMap((projectId) => this.loadData(projectId)), takeUntilDestroyed(this.destroyRef)).subscribe({
      next: (data) => this.state.set({ kind: 'loaded', data }),
      error: () => this.state.set({ kind: 'error' }),
    });
    this.refresh.pipe(startWith(undefined), switchMap(() => this.loadProjects()), takeUntilDestroyed(this.destroyRef)).subscribe((projects) => {
      const currentState = this.state();
      const selected = currentState.kind === 'loaded' ? currentState.data.selectedProject : '';
      this.loadSelected(projects, selected);
    }, () => this.state.set({ kind: 'error' }));
  }

  protected reload(): void { this.state.set({ kind: 'loading' }); this.refresh.next(); }
  protected selectProject(event: Event): void { const id = (event.target as HTMLSelectElement).value; this.state.set({ kind: 'loading' }); this.projectSelection.next(id); }
  protected setSearch(event: Event): void { this.search.set((event.target as HTMLInputElement).value.trim().toLowerCase()); }
  protected setStatus(event: Event): void { this.statusFilter.set((event.target as HTMLSelectElement).value); }
  protected failedCount(runs: RunSummary[]): number { return runs.filter((run) => run.status === 'FAILED').length; }
  protected blockedCount(tasks: TaskSummary[]): number { return tasks.filter((task) => task.status === 'BLOCKED').length; }
  protected completedThisWeek(runs: RunSummary[]): number { const since = Date.now() - 7 * 24 * 60 * 60 * 1000; return runs.filter((run) => run.status === 'SUCCESS' && Date.parse(run.finished_at ?? '') >= since).length; }
  protected filteredRuns(runs: RunSummary[]): RunSummary[] { const query = this.search(); const status = this.statusFilter(); return runs.filter((run) => (!status || run.status === status) && (!query || [run.id, run.task_id, run.agent_id, run.model, run.status, run.output, run.error].some((v) => v?.toLowerCase().includes(query)))); }
  protected filteredActivity(items: ActivityItem[]): ActivityItem[] { const query = this.search(); return items.filter((item) => !query || [item.type, item.subject, item.status, item.actor].some((v) => v.toLowerCase().includes(query))); }

  private loadProjects(): Observable<ProjectSummary[]> { return this.api.getAllPages<ProjectSummary>('projects'); }
  private loadSelected(projects: ProjectSummary[], selected: string): void {
    const id = projects.some((project) => project.id === selected) ? selected : (this.route.snapshot.queryParamMap.get('projectId') && projects.some((project) => project.id === this.route.snapshot.queryParamMap.get('projectId')) ? this.route.snapshot.queryParamMap.get('projectId')! : projects[0]?.id ?? '');
    this.projectSelection.next(id);
  }
  private loadData(projectId: string): Observable<ObservabilityData> {
    if (!projectId) return of({ projects: [], selectedProject: '', runs: [], tasks: [], agents: [], activity: [] });
    const encoded = encodeURIComponent(projectId);
    return forkJoin({ projects: this.loadProjects(), runs: this.api.getAllPages<RunSummary>(`projects/${encoded}/runs`), tasks: this.api.getAllPages<TaskSummary>(`projects/${encoded}/tasks`) }).pipe(
      switchMap(({ projects, runs, tasks }) => {
        const agentIDs = [...new Set(tasks.map((task) => task.assigned_to).filter((id): id is string => Boolean(id)))].slice(0, 50);
        const agents$ = agentIDs.length ? forkJoin(agentIDs.map((id) => this.api.get<AgentSummary>(`agents/${encodeURIComponent(id)}`))) : of([]);
        if (!this.isLogs) return agents$.pipe(map((agents) => ({ projects, selectedProject: projectId, runs, tasks, agents, activity: [] })));
        const recentTasks = [...tasks].sort((a, b) => (b.updated_at ?? '').localeCompare(a.updated_at ?? '')).slice(0, 50);
        const details$ = recentTasks.length ? forkJoin(recentTasks.map((task) => this.api.get<TaskDetail>(`tasks/${encodeURIComponent(task.id)}`))) : of([]);
        return forkJoin({ agents: agents$, details: details$ }).pipe(map(({ details }) => ({ projects, selectedProject: projectId, runs, tasks, agents: [], activity: this.toActivity(runs, details) })));
      }),
    );
  }
  private toActivity(runs: RunSummary[], tasks: TaskDetail[]): ActivityItem[] {
    const runItems = runs.flatMap((run) => (run.audit_events ?? []).map((event: RunAuditEvent) => ({ id: `run-${event.id}`, at: event.created_at, type: event.event_type, subject: run.task_id || run.id, status: event.to_status || run.status, actor: `${event.actor_type}:${event.actor_id}` })));
    const taskItems = tasks.flatMap((task) => (task.audit_events ?? []).map((event: TaskAuditEvent) => ({ id: `task-${event.id}`, at: event.created_at, type: event.event_type, subject: task.id, status: event.to_status || task.status, actor: `${event.actor_type}:${event.actor_id}` })));
    return [...runItems, ...taskItems].sort((a, b) => b.at.localeCompare(a.at));
  }
}
