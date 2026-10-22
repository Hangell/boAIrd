import { HttpErrorResponse } from '@angular/common/http';
import { ChangeDetectionStrategy, Component, DestroyRef, inject, OnInit, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { catchError, forkJoin, map, of, switchMap, throwError } from 'rxjs';
import { ApiClient } from '../../core/api/api-client';
import { ApiPage, BrainAgent, BrainConfiguration, BrainSummary, ProjectSummary, RunSummary, TaskSummary } from '../../core/api/api-models';

type DashboardState = 'loading' | 'error' | 'empty' | 'ready';
const autonomyModes: BrainConfiguration['autonomy'][] = ['MANUAL', 'SEMI_AUTO', 'AUTONOMOUS'];

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink],
  selector: 'app-brain-dashboard',
  template: `
    <header class="page-header">
      <p class="eyebrow" i18n="@@brainDashboard.eyebrow">Project supervision</p>
      <h1 i18n="@@brainDashboard.title">Brain dashboard</h1>
      <p class="page-description" i18n="@@brainDashboard.description">Configure the project Brain and review its recent proposals and requests for human attention.</p>
      <label class="project-picker"><span i18n="@@brainDashboard.project">Project</span>
        <select name="project" [ngModel]="projectId()" (ngModelChange)="selectProject($event)">
          <option value="" i18n="@@brainDashboard.chooseProject">Choose a project</option>
          @for (project of projects(); track project.id) { <option [value]="project.id">{{ project.name }}</option> }
        </select>
      </label>
    </header>

    @if (state() === 'loading') { <p class="notice" role="status" i18n="@@brainDashboard.loading">Loading Brain dashboard…</p> }
    @else if (state() === 'error') { <section class="notice notice-error" role="alert"><p i18n="@@brainDashboard.loadError">Brain information could not be loaded. Check the local API and try again.</p><button type="button" class="button" (click)="reload()" i18n="@@common.retry">Retry</button></section> }
    @else if (state() === 'empty') { <p class="notice" i18n="@@brainDashboard.noProjects">Create a project before configuring its Brain.</p> }
    @else if (projectId()) {
      @if (brain(); as currentBrain) {
        <section class="panel brain-summary" aria-labelledby="current-brain-heading">
          <div><p class="eyebrow" i18n="@@brainDashboard.activeBrain">Active project Brain</p><h2 id="current-brain-heading">{{ currentAgent()?.name || currentBrain.agent_id }}</h2>
            <p class="secondary">{{ currentAgent()?.role }} · {{ currentAgent()?.runtime }} · {{ currentAgent()?.provider }} · {{ currentAgent()?.model }} · {{ currentBrain.autonomy }}</p>
            @if (currentAgent()?.capabilities?.length) { <p class="field-note"><span i18n="@@brainDashboard.capabilities">Explicit capabilities:</span> {{ currentAgent()?.capabilities?.join(', ') }}</p> }
            @if (currentAgent(); as agent) { <a class="agent-link" [routerLink]="['/agents', agent.id]" i18n="@@brainDashboard.editAgent">Edit agent configuration</a> }
          </div><span class="tag">{{ currentAgent()?.status || inactiveLabel }}</span>
        </section>
      } @else {
        <p class="notice" i18n="@@brainDashboard.notConfigured">This project does not have an active Brain. Select an eligible agent to configure one.</p>
      }

      <form class="panel config-form" (ngSubmit)="saveConfiguration()">
        <h2>{{ brain() ? replaceHeading : configureHeading }}</h2>
        <label><span i18n="@@brainDashboard.agent">Brain agent</span><select name="agent" required [(ngModel)]="configuration.agent_id">
          <option value="" i18n="@@brainDashboard.chooseAgent">Choose an eligible API agent</option>
          @for (agent of eligibleAgents(); track agent.id) { <option [value]="agent.id">{{ agent.name }} · {{ agent.role }}</option> }
        </select></label>
        <p class="field-note" i18n="@@brainDashboard.agentEligibility">Only API agents with the explicit brain.execute capability are eligible. The backend validates eligibility on every change.</p>
        <label><span i18n="@@brainDashboard.autonomy">Autonomy mode</span><select name="autonomy" required [(ngModel)]="configuration.autonomy">
          @for (mode of autonomyOptions; track mode) { <option [value]="mode">{{ mode }}</option> }
        </select></label>
        @if (brain()) {
          <label><span i18n="@@brainDashboard.reason">Replacement reason</span><textarea name="reason" required maxlength="1000" rows="3" [(ngModel)]="replacementReason"></textarea></label>
        }
        @if (validationError()) { <p class="notice notice-error" role="alert">{{ validationError() }}</p> }
        @if (saveError()) { <p class="notice notice-error" role="alert" i18n="@@brainDashboard.saveError">Brain configuration could not be saved. Check your administrator session, agent eligibility, and the replacement reason. Internal server details are hidden.</p> }
        @if (saved()) { <p class="notice notice-success" role="status" i18n="@@brainDashboard.saved">Brain configuration saved.</p> }
        <button class="button" type="submit" [disabled]="saving() || eligibleAgents().length === 0">{{ saving() ? savingLabel : (brain() ? replaceLabel : configureLabel) }}</button>
      </form>

      <div class="dashboard-grid">
        <section class="panel" aria-labelledby="proposals-heading"><h2 id="proposals-heading" i18n="@@brainDashboard.proposals">Recent Brain proposals</h2>
          @if (runs().length) { <ul class="event-list">@for (run of runs(); track run.id) { <li><div class="event-meta"><strong>{{ run.model }}</strong><span class="tag" [class.tag-danger]="run.status === 'FAILED'">{{ run.status }}</span></div>
            @if (run.output) { <p class="proposal">{{ run.output }}</p> } @else { <p class="secondary" i18n="@@brainDashboard.noProposal">No proposal text was recorded for this run.</p> }
          </li> }</ul> } @else { <p class="secondary" i18n="@@brainDashboard.noRuns">No Brain runs have been recorded for this project.</p> }
        </section>
        <section class="panel" aria-labelledby="interventions-heading"><h2 id="interventions-heading" i18n="@@brainDashboard.interventions">Requests for human attention</h2>
          @if (interventions().length) { <ul class="event-list">@for (task of interventions(); track task.id) { <li><strong>{{ task.title }}</strong><span class="tag tag-warning" i18n="@@brainDashboard.humanReview">Human review</span></li> }</ul> }
          @else { <p class="secondary" i18n="@@brainDashboard.noInterventions">There are no tasks waiting for human review.</p> }
        </section>
        <section class="panel" aria-labelledby="blocked-heading"><h2 id="blocked-heading" i18n="@@brainDashboard.blocked">Blocked decisions</h2>
          @if (blockedTasks().length) { <ul class="event-list">@for (task of blockedTasks(); track task.id) { <li><strong>{{ task.title }}</strong><span class="tag tag-danger">{{ task.status }}</span></li> }</ul> }
          @else { <p class="secondary" i18n="@@brainDashboard.noBlocked">There are no blocked tasks.</p> }
        </section>
      </div>
    }
  `,
  styles: `
    :host { display:block; max-width:78rem; margin-inline:auto; }
    .page-header { margin-block:1.5rem 2rem; } .eyebrow { color:var(--boaird-accent); font-size:.75rem; font-weight:700; letter-spacing:.12em; margin:0 0 .75rem; text-transform:uppercase; }
    h1 { color:var(--boaird-ink); font-size:clamp(2rem,4vw,3rem); letter-spacing:-.04em; line-height:1.1; margin:0; } h2 { color:var(--boaird-ink); font-size:1.2rem; margin:0 0 1rem; }
    .page-description,.secondary,.field-note { color:var(--boaird-muted); line-height:1.55; } .page-description { margin:.75rem 0 1.25rem; max-width:48rem; }
    .project-picker,.config-form label { color:var(--boaird-ink); display:grid; font-weight:700; gap:.4rem; max-width:38rem; }
    input,select,textarea { background:var(--boaird-surface); border:1px solid var(--boaird-border); border-radius:.5rem; color:var(--boaird-ink); font:inherit; font-weight:400; padding:.7rem; width:100%; }
    input:focus-visible,select:focus-visible,textarea:focus-visible,.button:focus-visible { outline:3px solid var(--boaird-accent); outline-offset:2px; } textarea { resize:vertical; }
    .panel,.notice { background:var(--boaird-surface); border:1px solid var(--boaird-border); border-radius:1rem; padding:1.25rem; }
    .brain-summary { align-items:center; display:flex; flex-wrap:wrap; gap:1rem; justify-content:space-between; margin-bottom:1rem; } .brain-summary h2 { margin:0; }
    .agent-link { color:var(--boaird-accent); font-weight:700; text-underline-offset:.2em; }
    .config-form { display:grid; gap:1rem; margin-bottom:1rem; } .config-form h2 { margin:0; } .field-note { font-size:.9rem; margin:-.5rem 0 0; }
    .dashboard-grid { display:grid; gap:1rem; grid-template-columns:repeat(auto-fit,minmax(min(100%,24rem),1fr)); margin-bottom:2rem; }
    .event-list { list-style:none; margin:0; padding:0; } .event-list li { border-top:1px solid var(--boaird-border); display:grid; gap:.55rem; padding:1rem 0; } .event-list li:first-child { border-top:0; padding-top:0; }
    .event-meta { align-items:center; display:flex; justify-content:space-between; } .proposal { color:var(--boaird-ink); line-height:1.6; margin:0; overflow-wrap:anywhere; white-space:pre-wrap; }
    .tag { background:var(--boaird-status-bg); border-radius:999px; color:var(--boaird-ink); display:inline-block; font-size:.75rem; font-weight:700; padding:.35rem .65rem; } .tag-danger,.tag-warning { color:var(--boaird-danger); }
    .notice { color:var(--boaird-muted); } .notice-error { border-color:var(--boaird-danger); } .notice-success { border-color:var(--boaird-accent); }
    .button { background:var(--boaird-ink); border:0; border-radius:.5rem; color:var(--boaird-on-ink); cursor:pointer; font:inherit; font-weight:700; justify-self:start; padding:.7rem 1rem; } .button:disabled { cursor:wait; opacity:.65; }
  `,
})
export class BrainDashboardComponent implements OnInit {
  private readonly api = inject(ApiClient);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly state = signal<DashboardState>('loading');
  protected readonly projects = signal<ProjectSummary[]>([]);
  protected readonly agents = signal<BrainAgent[]>([]);
  protected readonly brain = signal<BrainSummary | null>(null);
  protected readonly runs = signal<RunSummary[]>([]);
  protected readonly interventions = signal<TaskSummary[]>([]);
  protected readonly blockedTasks = signal<TaskSummary[]>([]);
  protected readonly projectId = signal('');
  protected readonly currentAgent = signal<BrainAgent | null>(null);
  protected readonly configuration: BrainConfiguration = { agent_id: '', autonomy: 'MANUAL' };
  protected readonly validationError = signal('');
  protected readonly saveError = signal(false);
  protected readonly saved = signal(false);
  protected readonly saving = signal(false);
  protected replacementReason = '';
  protected readonly autonomyOptions = autonomyModes;
  protected readonly inactiveLabel = $localize`:@@brainDashboard.agentStatusUnknown:Status unavailable`;
  protected readonly configureHeading = $localize`:@@brainDashboard.configure:Configure project Brain`;
  protected readonly replaceHeading = $localize`:@@brainDashboard.replace:Replace project Brain`;
  protected readonly configureLabel = $localize`:@@brainDashboard.configureButton:Configure Brain`;
  protected readonly replaceLabel = $localize`:@@brainDashboard.replaceButton:Replace Brain`;
  protected readonly savingLabel = $localize`:@@common.saving:Saving…`;

  ngOnInit(): void {
    this.api.getAllPages<ProjectSummary>('projects').pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: (projects) => {
        this.projects.set(projects);
        if (!projects.length) { this.state.set('empty'); return; }
        this.selectProject(projects[0].id);
      },
      error: () => this.state.set('error'),
    });
  }

  protected eligibleAgents(): BrainAgent[] {
    return this.agents().filter((agent) => agent.runtime === 'API' && agent.capabilities.includes('brain.execute'));
  }

  protected selectProject(projectId: string): void {
    this.projectId.set(projectId);
    this.state.set('loading'); this.brain.set(null); this.runs.set([]); this.interventions.set([]); this.blockedTasks.set([]); this.currentAgent.set(null);
    if (!projectId) { this.state.set('empty'); return; }
    const encodedID = encodeURIComponent(projectId);
    const brainRequest = this.api.get<BrainSummary>(`projects/${encodedID}/brain`).pipe(
      catchError((error: unknown) => error instanceof HttpErrorResponse && error.status === 404 ? of(null) : throwError(() => error)),
    );
    forkJoin({
      brain: brainRequest,
      agents: this.api.getAllPages<BrainAgent>('agents'),
      interventions: this.api.getAllPages<TaskSummary>(`projects/${encodedID}/tasks?status=HUMAN_REVIEW`),
      blockedTasks: this.api.getAllPages<TaskSummary>(`projects/${encodedID}/tasks?status=BLOCKED`),
    }).pipe(
      switchMap((data) => {
        const runs = data.brain
          ? this.api.get<ApiPage<RunSummary>>(`projects/${encodedID}/runs?agent_id=${encodeURIComponent(data.brain.agent_id)}&limit=10&offset=0`).pipe(map((page) => page.items))
          : of([] as RunSummary[]);
        return runs.pipe(map((loadedRuns) => ({ ...data, runs: loadedRuns })));
      }),
      takeUntilDestroyed(this.destroyRef),
    ).subscribe({
      next: (data) => {
        this.brain.set(data.brain); this.agents.set(data.agents); this.runs.set(data.runs); this.interventions.set(data.interventions); this.blockedTasks.set(data.blockedTasks);
        this.currentAgent.set(data.agents.find((agent) => agent.id === data.brain?.agent_id) ?? null);
        this.configuration.agent_id = data.brain?.agent_id ?? '';
        this.configuration.autonomy = data.brain?.autonomy ?? 'MANUAL';
        this.state.set('ready');
      },
      error: () => this.state.set('error'),
    });
  }

  protected saveConfiguration(): void {
    this.validationError.set(''); this.saveError.set(false); this.saved.set(false);
    if (!this.eligibleAgents().some((agent) => agent.id === this.configuration.agent_id)) {
      this.validationError.set($localize`:@@brainDashboard.invalidAgent:Choose an API agent with the explicit brain.execute capability.`); return;
    }
    if (this.brain() && !this.replacementReason.trim()) {
      this.validationError.set($localize`:@@brainDashboard.authRequired:Provide a reason to replace the Brain.`); return;
    }
    const body = { ...this.configuration };
    const request = this.brain()
      ? this.api.put<BrainConfiguration & { reason: string }, BrainSummary>(
        `projects/${encodeURIComponent(this.projectId())}/brain`, { ...body, reason: this.replacementReason.trim() },
      )
      : this.api.post<BrainConfiguration, BrainSummary>(`projects/${encodeURIComponent(this.projectId())}/brain`, body);
    this.saving.set(true);
    request.pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: (brain) => { this.brain.set(brain); this.replacementReason = ''; this.saved.set(true); this.saving.set(false); this.selectProject(this.projectId()); },
      error: () => { this.saveError.set(true); this.saving.set(false); },
    });
  }

  protected reload(): void { if (this.projectId()) this.selectProject(this.projectId()); }
}
