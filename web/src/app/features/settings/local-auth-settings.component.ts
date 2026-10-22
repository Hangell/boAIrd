import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { LocalAuthService } from '../../core/auth/local-auth.service';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule],
  selector: 'app-local-auth-settings',
  template: `
    <header class="page-header">
      <p class="eyebrow" i18n="@@localAuth.eyebrow">Dashboard security</p>
      <h1 i18n="@@localAuth.title">Local API authentication</h1>
      <p class="description" i18n="@@localAuth.description">Set the local administrator token to read task conversations and authorize protected dashboard actions.</p>
    </header>
    <section class="panel" aria-labelledby="token-heading">
      <h2 id="token-heading" i18n="@@localAuth.tokenHeading">Administrator token</h2>
      <p class="secondary" i18n="@@localAuth.memoryNotice">The token stays in memory for this browser session only. Reloading the dashboard clears it.</p>
      <form (ngSubmit)="saveToken()">
        <label for="local-api-token" i18n="@@localAuth.tokenLabel">32-byte local API token (64 hexadecimal characters)</label>
        <input id="local-api-token" name="token" type="password" autocomplete="off" spellcheck="false" [(ngModel)]="token" required>
        @if (error()) { <p class="notice notice-error" role="alert" i18n="@@localAuth.invalidToken">Enter exactly 64 hexadecimal characters.</p> }
        @if (saved()) { <p class="notice notice-success" role="status" i18n="@@localAuth.saved">Token is active in memory.</p> }
        <div class="actions">
          <button class="button" type="submit" i18n="@@localAuth.save">Use token</button>
          @if (auth.configured()) { <button class="button button-secondary" type="button" (click)="clearToken()" i18n="@@localAuth.clear">Clear token</button> }
        </div>
      </form>
    </section>
  `,
  styles: `
    :host { display:block; max-width:60rem; margin-inline:auto; }
    .page-header { margin-block:1.5rem 2rem; }
    .eyebrow { color:var(--boaird-accent); font-size:.75rem; font-weight:700; letter-spacing:.12em; margin:0 0 .75rem; text-transform:uppercase; }
    h1 { color:var(--boaird-ink); font-size:clamp(2rem,4vw,3rem); letter-spacing:-.04em; margin:0; }
    h2 { color:var(--boaird-ink); font-size:1.2rem; margin:0; }
    .description,.secondary { color:var(--boaird-muted); line-height:1.55; }
    .panel { background:var(--boaird-surface); border:1px solid var(--boaird-border); border-radius:1rem; display:grid; gap:1rem; padding:1.25rem; }
    form { display:grid; gap:.75rem; max-width:38rem; }
    label { color:var(--boaird-ink); font-weight:700; }
    input { background:var(--boaird-surface); border:1px solid var(--boaird-border); border-radius:.5rem; color:var(--boaird-ink); font:inherit; padding:.7rem; }
    input:focus-visible,.button:focus-visible { outline:3px solid var(--boaird-accent); outline-offset:2px; }
    .actions { display:flex; flex-wrap:wrap; gap:.75rem; }
    .button { background:var(--boaird-ink); border:0; border-radius:.5rem; color:var(--boaird-on-ink); cursor:pointer; font:inherit; font-weight:700; padding:.7rem 1rem; }
    .button-secondary { background:var(--boaird-surface-muted); color:var(--boaird-ink); }
    .notice { border:1px solid var(--boaird-border); border-radius:.75rem; margin:0; padding:.75rem; }
    .notice-error { border-color:var(--boaird-danger); }
    .notice-success { border-color:var(--boaird-accent); }
  `,
})
export class LocalAuthSettingsComponent {
  protected readonly auth = inject(LocalAuthService);
  protected readonly error = signal(false);
  protected readonly saved = signal(false);
  protected token = '';

  protected saveToken(): void {
    this.error.set(false);
    this.saved.set(false);
    if (!this.auth.setToken(this.token)) {
      this.error.set(true);
      return;
    }
    this.token = '';
    this.saved.set(true);
  }

  protected clearToken(): void {
    this.auth.clearToken();
    this.token = '';
    this.saved.set(false);
    this.error.set(false);
  }
}
