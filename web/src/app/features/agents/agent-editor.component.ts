import { ChangeDetectionStrategy, Component, DestroyRef, inject, OnInit, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { FormsModule } from '@angular/forms';
import { catchError, forkJoin, of, switchMap } from 'rxjs';
import { ApiClient } from '../../core/api/api-client';
import { Agent, AgentConfiguration, AgentRuntime, CredentialMetadata } from '../../core/api/api-models';

const runtimes: AgentRuntime[] = ['API', 'MCP', 'LOCAL', 'REMOTE'];
const capabilityPattern = /^[a-z][a-z0-9]*(?:\.[a-z0-9]+)*$/;

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink],
  selector: 'app-agent-editor',
  template: `
    <p class="back-link"><a routerLink="/agents" i18n="@@agentEditor.back">← All agents</a></p>
    @if (loading()) { <p class="notice" role="status" i18n="@@agentEditor.loading">Loading agent configuration…</p> }
    @else if (loadError()) { <section class="notice notice-error" role="alert"><p i18n="@@agentEditor.loadError">Agent configuration could not be loaded. Check the local API and try again.</p><button class="button" type="button" (click)="retry()" i18n="@@common.retry">Retry</button></section> }
    @else {
      <header class="page-header"><p class="eyebrow" i18n="@@agentEditor.eyebrow">Agent configuration</p><h1>{{ agent() ? agent()!.name : createTitle }}</h1>
        @if (agent(); as currentAgent) { <p><span class="tag">{{ currentAgent.status }}</span> <span class="secondary" i18n="@@agentEditor.statusReadonly">Status is reported by the runtime and cannot be edited here.</span></p> }
      </header>
      @if (saveError()) { <p class="notice notice-error" role="alert" i18n="@@agentEditor.saveError">The configuration could not be saved. Review the fields and try again. Internal server details are hidden.</p> }
      @if (saved()) { <p class="notice notice-success" role="status" i18n="@@agentEditor.saved">Agent configuration saved.</p> }
      <form class="editor" (ngSubmit)="save()">
        <label><span i18n="@@agentEditor.name">Name</span><input name="name" required maxlength="200" [(ngModel)]="form.name"></label>
        <label><span i18n="@@agentEditor.role">Role label</span><input name="role" required maxlength="100" [(ngModel)]="form.role"></label>
        <p class="field-note" i18n="@@agentEditor.roleHint">This is descriptive text only; authorization is never inferred from a role.</p>
        <label><span i18n="@@agentEditor.avatar">Avatar URL or identifier</span><input name="avatar" maxlength="1000" [(ngModel)]="form.avatar"></label>
        <label><span i18n="@@agentEditor.description">Description</span><textarea name="description" maxlength="4000" rows="3" [(ngModel)]="form.description"></textarea></label>
        <label><span i18n="@@agentEditor.runtime">Runtime</span><select name="runtime" required [(ngModel)]="form.runtime"><option value="API">API</option><option value="MCP">MCP</option><option value="LOCAL">LOCAL</option><option value="REMOTE">REMOTE</option></select></label>
        @if (form.runtime === 'API') {
          <fieldset><legend i18n="@@agentEditor.providerSettings">API provider settings</legend>
            <label><span i18n="@@agentEditor.provider">Provider</span><input name="provider" required maxlength="100" [(ngModel)]="form.provider"></label>
            <label><span i18n="@@agentEditor.model">Model</span><input name="model" required maxlength="200" [(ngModel)]="form.model"></label>
            <label><span i18n="@@agentEditor.baseUrl">Custom base URL (optional)</span><input name="providerBaseUrl" type="url" maxlength="2048" [(ngModel)]="form.provider_base_url"></label>
            <label><span i18n="@@agentEditor.credential">Stored credential</span><select name="credential" required [(ngModel)]="form.credential_id"><option value="" i18n="@@agentEditor.chooseCredential">Choose a stored credential</option>@for (credential of credentials(); track credential.id) { <option [value]="credential.id">{{ credential.name }} ({{ credential.provider }})</option> }</select></label>
            <p class="field-note" i18n="@@agentEditor.credentialHint">Only credential metadata is shown here. Secret values are never loaded into this screen.</p>
          </fieldset>
        }
        <label><span i18n="@@agentEditor.instructions">Instructions</span><textarea name="instructions" maxlength="16000" rows="8" [(ngModel)]="form.instructions"></textarea></label>
        <label><span i18n="@@agentEditor.languages">Language preferences</span><textarea name="languages" rows="3" [(ngModel)]="languagesText"></textarea></label>
        <label><span i18n="@@agentEditor.capabilities">Explicit capabilities</span><textarea name="capabilities" rows="4" [(ngModel)]="capabilitiesText"></textarea></label>
        <p class="field-note" i18n="@@agentEditor.capabilityHint">Enter one capability identifier per line (for example, brain.execute). Capabilities are configured explicitly and are not derived from the role label.</p>
        @if (validationError()) { <p class="notice notice-error" role="alert">{{ validationError() }}</p> }
        <div class="actions"><a class="button button-secondary" routerLink="/agents" i18n="@@common.cancel">Cancel</a><button class="button" type="submit" [disabled]="saving()">{{ saving() ? savingLabel : saveLabel }}</button></div>
      </form>
    }
  `,
  styles: `
    :host { display:block; max-width:58rem; margin-inline:auto; }
    .back-link { margin:.5rem 0 1rem; } a { color:var(--boaird-accent); font-weight:700; text-underline-offset:.2em; }
    .page-header { margin-block:1.5rem; } .eyebrow { color:var(--boaird-accent); font-size:.75rem; font-weight:700; letter-spacing:.12em; margin:0 0 .75rem; text-transform:uppercase; }
    h1 { color:var(--boaird-ink); font-size:clamp(2rem,4vw,3rem); letter-spacing:-.04em; margin:0; } .secondary,.field-note { color:var(--boaird-muted); font-size:.9rem; line-height:1.5; }
    .editor { background:var(--boaird-surface); border:1px solid var(--boaird-border); border-radius:1rem; display:grid; gap:1rem; padding:clamp(1rem,3vw,2rem); }
    label { color:var(--boaird-ink); display:grid; font-weight:700; gap:.4rem; } input,select,textarea { background:var(--boaird-surface); border:1px solid var(--boaird-border); border-radius:.5rem; color:var(--boaird-ink); font:inherit; font-weight:400; padding:.7rem; width:100%; }
    textarea { resize:vertical; } input:focus-visible,select:focus-visible,textarea:focus-visible,.button:focus-visible { outline:3px solid var(--boaird-accent); outline-offset:2px; }
    fieldset { border:1px solid var(--boaird-border); border-radius:.75rem; display:grid; gap:1rem; padding:1rem; } legend { color:var(--boaird-ink); font-weight:700; padding-inline:.4rem; }
    .field-note { margin:-.5rem 0 0; } .actions { display:flex; flex-wrap:wrap; gap:.75rem; justify-content:flex-end; }
    .button { background:var(--boaird-ink); border:0; border-radius:.5rem; color:var(--boaird-on-ink); cursor:pointer; display:inline-flex; font:inherit; font-weight:700; padding:.7rem 1rem; text-decoration:none; }
    .button:disabled { cursor:wait; opacity:.65; } .button-secondary { background:transparent; border:1px solid var(--boaird-border); color:var(--boaird-ink); }
    .tag { background:var(--boaird-status-bg); border-radius:999px; color:var(--boaird-ink); display:inline-block; font-size:.75rem; font-weight:700; padding:.35rem .65rem; }
    .notice { background:var(--boaird-surface); border:1px solid var(--boaird-border); border-radius:.9rem; color:var(--boaird-muted); padding:1.25rem; } .notice-error { border-color:var(--boaird-danger); } .notice-success { border-color:var(--boaird-accent); }
  `,
})
export class AgentEditorComponent implements OnInit {
  private readonly api = inject(ApiClient);
  private readonly route = inject(ActivatedRoute);
  private readonly destroyRef = inject(DestroyRef);
  private agentId = '';

  protected readonly loading = signal(true);
  protected readonly loadError = signal(false);
  protected readonly saveError = signal(false);
  protected readonly saved = signal(false);
  protected readonly saving = signal(false);
  protected readonly agent = signal<Agent | null>(null);
  protected readonly credentials = signal<CredentialMetadata[]>([]);
  protected readonly validationError = signal('');
  protected readonly form: AgentConfiguration = this.emptyConfiguration();
  protected languagesText = '';
  protected capabilitiesText = '';
  protected readonly createTitle = $localize`:@@agentEditor.createTitle:Create agent`;
  protected readonly saveLabel = $localize`:@@common.save:Save configuration`;
  protected readonly savingLabel = $localize`:@@common.saving:Saving…`;

  ngOnInit(): void {
    this.route.paramMap.pipe(
      switchMap((params) => {
        this.agentId = params.get('agentId') ?? '';
        this.loading.set(true);
        return this.loadEditor(this.agentId);
      }),
      takeUntilDestroyed(this.destroyRef),
    ).subscribe((result) => {
      if (!result) { this.loadError.set(true); this.loading.set(false); return; }
      const { agent, credentials } = result;
      this.agent.set(agent);
      this.credentials.set(credentials.items);
      if (agent) this.fillForm(agent);
      this.loading.set(false);
    });
  }

  protected retry(): void { this.loadError.set(false); this.loading.set(true); this.loadEditor(this.agentId).pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result) => {
    if (!result) { this.loadError.set(true); this.loading.set(false); return; }
    this.agent.set(result.agent); this.credentials.set(result.credentials.items); if (result.agent) this.fillForm(result.agent); this.loading.set(false);
  }); }

  protected save(): void {
    this.validationError.set(''); this.saveError.set(false); this.saved.set(false);
    const configuration = this.buildConfiguration();
    const invalidCapability = configuration.capabilities.find((capability) => !capabilityPattern.test(capability));
    if (configuration.language_preferences.length > 20 || configuration.language_preferences.some((language) => language.length > 100)) {
      this.validationError.set($localize`:@@agentEditor.invalidLanguages:Use up to 20 language values, each no longer than 100 characters.`); return;
    }
    if (configuration.capabilities.length > 100 || invalidCapability) {
      this.validationError.set($localize`:@@agentEditor.invalidCapabilities:Use up to 100 lowercase capability identifiers such as brain.execute.`); return;
    }
    if (configuration.runtime === 'API' && (!configuration.provider?.trim() || !configuration.model?.trim() || !configuration.credential_id)) {
      this.validationError.set($localize`:@@agentEditor.apiRequired:API agents require a provider, model, and stored credential.`); return;
    }
    this.saving.set(true);
    const request = this.isEditing()
      ? this.api.patch<AgentConfiguration, Agent>(`agents/${encodeURIComponent(this.agentId)}`, configuration)
      : this.api.post<AgentConfiguration, Agent>('agents', configuration);
    request.pipe(takeUntilDestroyed(this.destroyRef)).subscribe({
      next: (agent) => { this.agent.set(agent); this.saved.set(true); this.saving.set(false); },
      error: () => { this.saveError.set(true); this.saving.set(false); },
    });
  }

  private loadEditor(agentId: string) {
    const credentials = this.api.get<{ items: CredentialMetadata[] }>('credentials');
    const agent = agentId && agentId !== 'new'
      ? this.api.get<Agent>(`agents/${encodeURIComponent(agentId)}`)
      : of(null);
    return forkJoin({ credentials, agent }).pipe(
      // Do not render raw server error details; the template exposes a stable message only.
      catchError(() => of(null)),
    );
  }

  private fillForm(agent: Agent): void {
    Object.assign(this.form, {
      name: agent.name, avatar: agent.avatar, description: agent.description, role: agent.role, runtime: agent.runtime,
      provider: agent.provider ?? '', provider_base_url: agent.provider_base_url ?? '', model: agent.model ?? '',
      credential_id: agent.credential_id ?? '', instructions: agent.instructions,
      language_preferences: [...agent.language_preferences], capabilities: [...agent.capabilities],
    });
    this.languagesText = agent.language_preferences.join('\n');
    this.capabilitiesText = agent.capabilities.join('\n');
  }

  private buildConfiguration(): AgentConfiguration {
    const configuration: AgentConfiguration = {
      ...this.form,
      name: this.form.name.trim(), role: this.form.role.trim(),
      language_preferences: this.lines(this.languagesText), capabilities: this.lines(this.capabilitiesText),
    };
    if (configuration.runtime !== 'API') {
      delete configuration.provider; delete configuration.provider_base_url; delete configuration.model; delete configuration.credential_id;
    }
    return configuration;
  }

  private lines(value: string): string[] { return value.split('\n').map((line) => line.trim()).filter(Boolean); }

  private isEditing(): boolean { return this.agentId !== '' && this.agentId !== 'new'; }

  private emptyConfiguration(): AgentConfiguration {
    return { name: '', avatar: '', description: '', role: '', runtime: runtimes[0], provider: '', provider_base_url: '', model: '', credential_id: '', instructions: '', language_preferences: [], capabilities: [] };
  }
}
