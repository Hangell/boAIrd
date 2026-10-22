import { HttpErrorResponse } from '@angular/common/http';
import { Component, ElementRef, inject, ViewChild } from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import { ApiClient } from '../../core/api/api-client';

@Component({
  selector: 'app-local-login',
  standalone: true,
  imports: [],
  template: `
    <main class="login-shell">
      <section class="login-card" aria-labelledby="login-title">
        <h1 id="login-title">Local administrator</h1>
        <p>Enter the token configured in <code>BOAIRD_LOCAL_AUTH_TOKEN</code>. It is exchanged for a temporary, HttpOnly browser session and is not saved by the dashboard.</p>
        <form (ngSubmit)="connect()">
          <label for="local-token">Administrator token</label>
          <input #tokenInput id="local-token" name="token" type="password" autocomplete="current-password" required />
          @if (errorMessage) {
            <p class="error" role="alert">{{ errorMessage }}</p>
          }
          <button type="submit" [disabled]="busy">{{ busy ? 'Connecting…' : 'Connect' }}</button>
        </form>
        <button class="clear-session" type="button" (click)="clearSession()">Clear browser session</button>
      </section>
    </main>
  `,
  styles: `
    :host { display: block; min-height: 100%; }
    .login-shell { display: grid; min-height: 100vh; place-items: center; padding: 24px; }
    .login-card { width: min(100%, 460px); display: grid; gap: 16px; padding: 28px; border: 1px solid var(--border-color, #d8dce3); border-radius: 16px; background: var(--surface-color, #fff); }
    h1, p { margin: 0; }
    form { display: grid; gap: 12px; }
    input { min-height: 42px; padding: 8px 12px; border: 1px solid var(--border-color, #aab0bc); border-radius: 8px; }
    button { min-height: 42px; border: 0; border-radius: 8px; cursor: pointer; }
    button[type='submit'] { color: #fff; background: #3347a9; }
    button:disabled { cursor: wait; opacity: .6; }
    .clear-session { background: transparent; color: inherit; }
    .error { color: #a52222; }
  `,
})
export class LocalLoginComponent {
  private readonly api = inject(ApiClient);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  @ViewChild('tokenInput') private tokenInput!: ElementRef<HTMLInputElement>;
  busy = false;
  errorMessage = '';

  connect(): void {
    const token = this.tokenInput.nativeElement.value;
    if (!token || this.busy) return;
    this.busy = true;
    this.errorMessage = '';
    this.tokenInput.nativeElement.value = '';
    this.api.createLocalSession(token).subscribe({
      next: () => {
        this.busy = false;
        const returnUrl = this.route.snapshot.queryParamMap.get('returnUrl') ?? '/overview';
        void this.router.navigateByUrl(returnUrl.startsWith('/') && !returnUrl.startsWith('//') ? returnUrl : '/overview');
      },
      error: (error: unknown) => {
        this.busy = false;
        this.errorMessage = error instanceof HttpErrorResponse && error.status === 401
          ? 'The administrator token is invalid.'
          : 'The local API could not establish a session.';
      },
    });
  }

  clearSession(): void {
    this.api.clearLocalSession().subscribe({
      next: () => { this.errorMessage = 'The browser session was cleared.'; },
      error: () => { this.errorMessage = 'The local API could not clear the session.'; },
    });
  }
}
