import { AsyncPipe } from '@angular/common';
import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { RouterLink } from '@angular/router';
import { catchError, map, Observable, startWith, Subject, switchMap } from 'rxjs';
import { ApiClient } from '../../core/api/api-client';
import { ProjectSummary } from '../../core/api/api-models';

type ProjectListState =
  | { kind: 'loading' }
  | { kind: 'error' }
  | { kind: 'empty' }
  | { kind: 'loaded'; projects: ProjectSummary[] };

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [AsyncPipe, RouterLink],
  selector: 'app-project-list',
  template: `
    <header class="page-header">
      <p class="eyebrow" i18n="@@projects.workspaceLabel">boAIrd workspace</p>
      <h1 i18n="@@projects.title">Projects</h1>
      <p class="page-description" i18n="@@projects.description">Choose a project to see its current operational status.</p>
    </header>

    @if (state$ | async; as state) {
      @switch (state.kind) {
        @case ('loading') {
          <p class="notice" role="status" i18n="@@projects.loading">Loading projects…</p>
        }
        @case ('error') {
          <section class="notice notice-error" role="alert">
            <p i18n="@@projects.loadError">Projects could not be loaded. Check that the local API is running, then try again.</p>
            <button type="button" class="button" (click)="retry()" i18n="@@common.retry">Retry</button>
          </section>
        }
        @case ('empty') {
          <p class="notice" i18n="@@projects.empty">No projects have been created yet.</p>
        }
        @case ('loaded') {
          <ul class="project-list" aria-label="Projects" i18n-aria-label="@@projects.listLabel">
            @for (project of state.projects; track project.id) {
              <li>
                <a class="project-card" [routerLink]="['/projects', project.id]">
                  <span class="project-copy">
                    <span class="project-name">{{ project.name }}</span>
                    <span class="project-description">{{ project.description || emptyDescription }}</span>
                  </span>
                  <span class="project-status">{{ project.status }}</span>
                </a>
              </li>
            }
          </ul>
        }
      }
    }
  `,
  styles: `
    :host { display: block; max-width: 78rem; margin-inline: auto; }
    .page-header { margin-block: 1.5rem 2rem; }
    .eyebrow { color: var(--boaird-accent); font-size: .75rem; font-weight: 700; letter-spacing: .12em; margin: 0 0 .75rem; text-transform: uppercase; }
    h1 { color: var(--boaird-ink); font-size: clamp(2rem, 4vw, 3rem); letter-spacing: -.04em; line-height: 1.1; margin: 0; }
    .page-description { color: var(--boaird-muted); font-size: 1rem; line-height: 1.6; margin: .75rem 0 0; max-width: 48rem; }
    .project-list { display: grid; gap: 1rem; grid-template-columns: repeat(auto-fit, minmax(min(100%, 19rem), 1fr)); list-style: none; margin: 0; padding: 0; }
    .project-card { align-items: center; background: var(--boaird-surface); border: 1px solid var(--boaird-border); border-radius: 1rem; color: inherit; display: flex; gap: 1rem; justify-content: space-between; min-height: 6rem; padding: 1.25rem; text-decoration: none; }
    .project-card:hover { border-color: var(--boaird-accent); }
    .project-card:focus-visible, .button:focus-visible { outline: 3px solid var(--boaird-accent); outline-offset: 3px; }
    .project-copy { display: grid; gap: .4rem; min-width: 0; }
    .project-name { color: var(--boaird-ink); font-size: 1.1rem; font-weight: 700; }
    .project-description { color: var(--boaird-muted); overflow-wrap: anywhere; }
    .project-status { background: var(--boaird-status-bg); border-radius: 999px; color: var(--boaird-ink); flex: 0 0 auto; font-size: .75rem; font-weight: 700; padding: .35rem .65rem; }
    .notice { background: var(--boaird-surface); border: 1px solid var(--boaird-border); border-radius: .9rem; color: var(--boaird-muted); padding: 1.25rem; }
    .notice-error { border-color: var(--boaird-danger); }
    .button { background: var(--boaird-ink); border: 0; border-radius: .5rem; color: var(--boaird-on-ink); cursor: pointer; font: inherit; font-weight: 700; padding: .65rem 1rem; }
  `,
})
export class ProjectListComponent {
  private readonly api = inject(ApiClient);
  private readonly reload = new Subject<void>();

  protected readonly emptyDescription = $localize`:@@projects.emptyDescription:No description provided.`;
  protected readonly state$ = this.reload.pipe(
    startWith(undefined),
    switchMap(() => this.listAllProjects().pipe(
      map((projects): ProjectListState => projects.length === 0 ? { kind: 'empty' } : { kind: 'loaded', projects }),
      catchError(() => [{ kind: 'error' } as ProjectListState]),
      startWith({ kind: 'loading' } as ProjectListState),
    )),
  );

  protected retry(): void {
    this.reload.next();
  }

  private listAllProjects(): Observable<ProjectSummary[]> {
    return this.api.getAllPages<ProjectSummary>('projects');
  }
}
