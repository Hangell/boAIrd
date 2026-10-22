import { ChangeDetectionStrategy, Component, DestroyRef, inject, OnInit, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { RouterLink } from '@angular/router';
import { EMPTY, expand, map, Observable, reduce, Subject, switchMap, catchError, startWith } from 'rxjs';
import { ApiClient } from '../../core/api/api-client';
import { Agent, ApiPage } from '../../core/api/api-models';

type AgentListState =
  | { kind: 'loading' }
  | { kind: 'error' }
  | { kind: 'empty' }
  | { kind: 'loaded'; agents: Agent[] };

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  selector: 'app-agent-list',
  template: `
    <header class="page-header">
      <div><p class="eyebrow" i18n="@@agents.workspaceLabel">boAIrd workspace</p><h1 i18n="@@agents.title">Agents</h1>
        <p class="page-description" i18n="@@agents.description">Configure agent runtimes, instructions, languages, and explicit capabilities.</p></div>
      <a class="button" routerLink="/agents/new" i18n="@@agents.create">Create agent</a>
    </header>
    @let currentState = state();
    @if (currentState.kind === 'loading') {
      <p class="notice" role="status" i18n="@@agents.loading">Loading agents…</p>
    } @else if (currentState.kind === 'error') {
      <section class="notice notice-error" role="alert"><p i18n="@@agents.loadError">Agents could not be loaded. Check that the local API is running, then try again.</p>
        <button class="button" type="button" (click)="retry()" i18n="@@common.retry">Retry</button></section>
    } @else if (currentState.kind === 'empty') {
      <p class="notice" i18n="@@agents.empty">No agents have been configured yet.</p>
    } @else {
      <ul class="agent-list" aria-label="Agents" i18n-aria-label="@@agents.listLabel">
        @for (agent of currentState.agents; track agent.id) {
          <li><a class="agent-card" [routerLink]="['/agents', agent.id]">
            <span class="agent-copy"><span class="agent-name">{{ agent.name }}</span><span class="secondary">{{ agent.role }} · {{ agent.runtime }}{{ agent.model ? ' · ' + agent.model : '' }}</span>
              @if (agent.capabilities.length) { <span class="secondary">{{ agent.capabilities.join(', ') }}</span> }
            </span><span class="tag">{{ agent.status }}</span>
          </a></li>
        }
      </ul>
    }
  `,
  styles: `
    :host { display:block; max-width:78rem; margin-inline:auto; }
    .page-header { align-items:center; display:flex; flex-wrap:wrap; gap:1rem; justify-content:space-between; margin-block:1.5rem 2rem; }
    .eyebrow { color:var(--boaird-accent); font-size:.75rem; font-weight:700; letter-spacing:.12em; margin:0 0 .75rem; text-transform:uppercase; }
    h1 { color:var(--boaird-ink); font-size:clamp(2rem,4vw,3rem); letter-spacing:-.04em; line-height:1.1; margin:0; }
    .page-description,.secondary { color:var(--boaird-muted); line-height:1.55; }
    .page-description { margin:.75rem 0 0; max-width:48rem; }
    .agent-list { display:grid; gap:1rem; grid-template-columns:repeat(auto-fit,minmax(min(100%,22rem),1fr)); list-style:none; margin:0; padding:0; }
    .agent-card { align-items:center; background:var(--boaird-surface); border:1px solid var(--boaird-border); border-radius:1rem; color:inherit; display:flex; gap:1rem; justify-content:space-between; min-height:6rem; padding:1.25rem; text-decoration:none; }
    .agent-card:hover { border-color:var(--boaird-accent); }
    .agent-card:focus-visible,.button:focus-visible { outline:3px solid var(--boaird-accent); outline-offset:3px; }
    .agent-copy { display:grid; gap:.35rem; min-width:0; }
    .agent-name { color:var(--boaird-ink); font-size:1.1rem; font-weight:700; }
    .secondary { font-size:.88rem; overflow-wrap:anywhere; }
    .tag { background:var(--boaird-status-bg); border-radius:999px; color:var(--boaird-ink); flex:0 0 auto; font-size:.75rem; font-weight:700; padding:.35rem .65rem; }
    .notice { background:var(--boaird-surface); border:1px solid var(--boaird-border); border-radius:.9rem; color:var(--boaird-muted); padding:1.25rem; }
    .notice-error { border-color:var(--boaird-danger); }
    .button { align-items:center; background:var(--boaird-ink); border:0; border-radius:.5rem; color:var(--boaird-on-ink); cursor:pointer; display:inline-flex; font:inherit; font-weight:700; padding:.7rem 1rem; text-decoration:none; }
  `,
})
export class AgentListComponent implements OnInit {
  private readonly api = inject(ApiClient);
  private readonly destroyRef = inject(DestroyRef);
  private readonly reload = new Subject<void>();
  protected readonly state = signal<AgentListState>({ kind: 'loading' });

  ngOnInit(): void {
    this.reload.pipe(
      startWith(undefined),
      switchMap(() => this.listAgents().pipe(
        map((agents): AgentListState => agents.length ? { kind: 'loaded', agents } : { kind: 'empty' }),
        catchError(() => [{ kind: 'error' } as AgentListState]),
        startWith({ kind: 'loading' } as AgentListState),
      )),
      takeUntilDestroyed(this.destroyRef),
    ).subscribe((state) => this.state.set(state));
  }

  protected retry(): void { this.reload.next(); }

  private listAgents(): Observable<Agent[]> {
    const page = (offset: number): Observable<ApiPage<Agent>> => this.api.get<ApiPage<Agent>>(`agents?limit=100&offset=${offset}`);
    return page(0).pipe(
      expand((response) => response.items.length === response.limit ? page(response.offset + response.limit) : EMPTY),
      map((response) => response.items),
      reduce((agents, items) => agents.concat(items), [] as Agent[]),
    );
  }
}
