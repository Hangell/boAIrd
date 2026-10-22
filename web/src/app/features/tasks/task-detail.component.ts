import { DatePipe } from '@angular/common';
import { ChangeDetectionStrategy, Component, DestroyRef, inject, OnInit, signal } from '@angular/core';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { catchError, forkJoin, map, of, switchMap } from 'rxjs';
import { ApiClient } from '../../core/api/api-client';
import { ApiPage, RunSummary, TaskConversation, TaskDetail } from '../../core/api/api-models';

interface TaskDetailData {
  task: TaskDetail;
  conversation: TaskConversation | null;
  runs: RunSummary[];
}

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DatePipe, RouterLink],
  selector: 'app-task-detail',
  template: `
    <p class="back-link"><a routerLink="/tasks" [queryParams]="{ projectId: projectId() }" i18n="@@taskDetail.back">← Back to board</a></p>
    @if (state() === 'loading') {
      <p class="notice" role="status" i18n="@@taskDetail.loading">Loading task details…</p>
    } @else if (state() === 'error') {
      <section class="notice notice-error" role="alert">
        <p i18n="@@taskDetail.loadError">Task details could not be loaded. Check the local API connection, then try again.</p>
        <button class="button" type="button" (click)="retry()" i18n="@@common.retry">Retry</button>
      </section>
    } @else if (data(); as detail) {
      <header class="page-header">
        <p class="eyebrow" i18n="@@taskDetail.eyebrow">Task details</p>
        <h1>{{ detail.task.title }}</h1>
        <div class="badges">
          <span class="tag">{{ detail.task.status }}</span>
          <span class="tag">{{ detail.task.priority }}</span>
          @if (detail.task.human_review_required) {
            <span class="tag tag-warning" i18n="@@taskDetail.humanReviewRequired">Human review required</span>
          }
        </div>
        <p class="description">{{ detail.task.description || emptyDescription }}</p>
      </header>

      <div class="details-grid">
        <section class="detail-card" aria-labelledby="history-heading">
          <h2 id="history-heading" i18n="@@taskDetail.historyHeading">Task history</h2>
          @if (detail.task.audit_events.length > 0) {
            <ol class="event-list">
              @for (event of detail.task.audit_events; track event.id) {
                <li>
                  <span class="event-title">
                    @if (event.from_status || event.to_status) {
                      {{ event.from_status || '—' }} → {{ event.to_status || '—' }}
                    } @else {
                      {{ event.event_type }}
                    }
                  </span>
                  <span class="secondary">{{ event.actor_type }} · {{ event.actor_id }}</span>
                  <time class="secondary" [attr.datetime]="event.created_at">{{ event.created_at | date: 'medium' }}</time>
                </li>
              }
            </ol>
          } @else {
            <p class="secondary" i18n="@@taskDetail.noHistory">No history has been recorded.</p>
          }
        </section>

        <section class="detail-card" aria-labelledby="conversation-heading">
          <h2 id="conversation-heading" i18n="@@taskDetail.conversationHeading">Conversation</h2>
          @if (detail.conversation) {
            @if (detail.conversation.next_before) {
              <button class="button older-button" type="button" [disabled]="loadingOlder()" (click)="loadOlderMessages()" i18n="@@taskDetail.loadOlder">{{ loadingOlder() ? loadingLabel : loadOlderLabel }}</button>
            }
            @if (olderError()) { <p class="notice notice-error" role="alert" i18n="@@taskDetail.olderError">Older messages could not be loaded. Try again.</p> }
            @if (detail.conversation.messages.length > 0) {
            <ol class="message-list">
              @for (message of detail.conversation.messages; track message.id) {
                <li>
                  <div class="message-heading">
                    <strong>{{ message.sender_type }}</strong>
                    <time class="secondary" [attr.datetime]="message.created_at">{{ message.created_at | date: 'medium' }}</time>
                  </div>
                  @if (message.content_type === 'text/plain') {
                    <p class="message-content">{{ message.content }}</p>
                  } @else {
                    <p class="secondary" i18n="@@taskDetail.opaqueMessage">Message payload is available as {{ message.content_type }}.</p>
                  }
                </li>
              }
            </ol>
            } @else {
            <p class="secondary" i18n="@@taskDetail.noMessages">No messages have been added to this conversation.</p>
            }
          } @else {
            <p class="notice notice-error" role="alert"><span i18n="@@taskDetail.conversationAuthError">The conversation could not be loaded. Configure or update the local administrator token in </span><a routerLink="/settings" i18n="@@taskDetail.settingsLink">Settings</a>.</p>
            <button class="button" type="button" [disabled]="loadingConversation()" (click)="retryConversation()" i18n="@@taskDetail.retryConversation">{{ loadingConversation() ? loadingLabel : retryConversationLabel }}</button>
          }
        </section>

        <section class="detail-card" aria-labelledby="runs-heading">
          <h2 id="runs-heading" i18n="@@taskDetail.runsHeading">Task runs</h2>
          @if (detail.runs.length > 0) {
            <ul class="run-list">
              @for (run of detail.runs; track run.id) {
                <li>
                  <span>{{ run.model }}</span>
                  <span class="tag" [class.tag-warning]="run.status === 'FAILED'">{{ run.status }}</span>
                </li>
              }
            </ul>
          } @else {
            <p class="secondary" i18n="@@taskDetail.noRuns">No runs are linked to this task.</p>
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
    h1 { color: var(--boaird-ink); font-size: clamp(2rem, 4vw, 3rem); letter-spacing: -.04em; line-height: 1.1; margin: 0; overflow-wrap: anywhere; }
    h2 { color: var(--boaird-ink); font-size: 1rem; margin: 0 0 1rem; }
    .badges { display: flex; flex-wrap: wrap; gap: .5rem; margin-block: 1rem; }
    .tag { background: var(--boaird-status-bg); border-radius: 999px; color: var(--boaird-ink); display: inline-block; font-size: .75rem; font-weight: 700; padding: .35rem .65rem; }
    .tag-warning { background: #fff1d6; color: #784900; }
    .description, .secondary { color: var(--boaird-muted); line-height: 1.55; }
    .description { white-space: pre-wrap; overflow-wrap: anywhere; }
    .details-grid { display: grid; gap: 1rem; grid-template-columns: repeat(auto-fit, minmax(min(100%, 24rem), 1fr)); margin-block: 1rem 2rem; }
    .detail-card { background: var(--boaird-surface); border: 1px solid var(--boaird-border); border-radius: 1rem; min-width: 0; padding: 1.25rem; }
    .event-list, .message-list, .run-list { list-style: none; margin: 0; padding: 0; }
    .event-list li, .message-list li, .run-list li { border-top: 1px solid var(--boaird-border); padding-block: .9rem; }
    .event-list li:first-child, .message-list li:first-child, .run-list li:first-child { border-top: 0; padding-top: 0; }
    .event-title { color: var(--boaird-ink); display: block; font-weight: 700; }
    .secondary { display: block; font-size: .85rem; margin-block: .25rem; overflow-wrap: anywhere; }
    .message-heading, .run-list li { align-items: center; display: flex; gap: 1rem; justify-content: space-between; }
    .message-content { color: var(--boaird-ink); line-height: 1.55; overflow-wrap: anywhere; white-space: pre-wrap; }
    .notice { background: var(--boaird-surface); border: 1px solid var(--boaird-border); border-radius: .9rem; color: var(--boaird-muted); padding: 1.25rem; }
    .notice-error { border-color: var(--boaird-danger); }
    .button { background: var(--boaird-ink); border: 0; border-radius: .5rem; color: var(--boaird-on-ink); cursor: pointer; font: inherit; font-weight: 700; padding: .65rem 1rem; }
    .older-button { margin-bottom: .75rem; }
  `,
})
export class TaskDetailComponent implements OnInit {
  private readonly api = inject(ApiClient);
  private readonly route = inject(ActivatedRoute);
  private readonly destroyRef = inject(DestroyRef);
  private taskId = '';

  protected readonly state = signal<'loading' | 'error' | 'loaded'>('loading');
  protected readonly data = signal<TaskDetailData | null>(null);
  protected readonly loadingOlder = signal(false);
  protected readonly loadingConversation = signal(false);
  protected readonly olderError = signal(false);
  protected readonly projectId = signal('');
  protected readonly loadingLabel = $localize`:@@common.loading:Loading…`;
  protected readonly loadOlderLabel = $localize`:@@taskDetail.loadOlderLabel:Load older messages`;
  protected readonly retryConversationLabel = $localize`:@@taskDetail.retryConversationLabel:Retry conversation`;
  protected readonly emptyDescription = $localize`:@@taskDetail.emptyDescription:No description provided.`;

  ngOnInit(): void {
    this.route.queryParamMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
      this.projectId.set(params.get('projectId') ?? '');
    });
    this.route.paramMap.pipe(
      map((params) => params.get('taskId') ?? ''),
      switchMap((taskId) => {
        this.taskId = taskId;
        return this.loadTask(taskId);
      }),
      takeUntilDestroyed(this.destroyRef),
    ).subscribe((data) => this.applyResult(data));
  }

  protected retry(): void {
    if (!this.taskId) return;
    this.state.set('loading');
    this.loadTask(this.taskId).pipe(takeUntilDestroyed(this.destroyRef)).subscribe((data) => this.applyResult(data));
  }

  private loadTask(taskId: string) {
    this.state.set('loading');
    this.loadingOlder.set(false);
    this.loadingConversation.set(false);
    this.olderError.set(false);
    if (!taskId) return of(null);
    const encodedID = encodeURIComponent(taskId);
    this.olderError.set(false);
    return this.api.get<TaskDetail>(`tasks/${encodedID}`).pipe(
      switchMap((task) => forkJoin({
        task: of(task),
        conversation: this.api.get<TaskConversation>(`tasks/${encodedID}/messages?limit=50`).pipe(catchError(() => of(null))),
        runs: this.api.get<ApiPage<RunSummary>>(`projects/${encodeURIComponent(task.project_id)}/runs?task_id=${encodeURIComponent(taskId)}&limit=10&offset=0`).pipe(map((page) => page.items)),
      })),
      map((data): TaskDetailData | null => data),
      catchError(() => of(null)),
    );
  }

  private applyResult(data: TaskDetailData | null): void {
    if (!data) {
      this.data.set(null);
      this.state.set('error');
      return;
    }
    this.data.set(data);
    this.state.set('loaded');
  }

  protected loadOlderMessages(): void {
    const current = this.data();
    const taskID = this.taskId;
    const cursor = current?.conversation?.next_before;
    if (!current || !cursor || this.loadingOlder()) return;

    this.loadingOlder.set(true);
    this.olderError.set(false);
    const path = `tasks/${encodeURIComponent(this.taskId)}/messages?limit=50&before=${encodeURIComponent(cursor)}`;
    this.api.get<TaskConversation>(path).pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: (older) => {
        const latest = this.data();
        if (!latest?.conversation || latest.task.id !== taskID) {
          this.loadingOlder.set(false);
          return;
        }
        const knownIDs = new Set(latest.conversation.messages.map((message) => message.id));
        const messages = [...older.messages.filter((message) => !knownIDs.has(message.id)), ...latest.conversation.messages];
        this.data.set({
          ...latest,
          conversation: { ...latest.conversation, messages, next_before: older.next_before },
        });
        this.loadingOlder.set(false);
      },
      error: () => {
        if (this.data()?.task.id === taskID) this.olderError.set(true);
        this.loadingOlder.set(false);
      },
    });
  }

  protected retryConversation(): void {
    const current = this.data();
    if (!current || this.loadingConversation()) return;
    this.loadingConversation.set(true);
    this.api.get<TaskConversation>(`tasks/${encodeURIComponent(current.task.id)}/messages?limit=50`)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe({
        next: (conversation) => {
          const latest = this.data();
          if (latest?.task.id === current.task.id) this.data.set({ ...latest, conversation });
          this.loadingConversation.set(false);
        },
        error: () => this.loadingConversation.set(false),
      });
  }
}
