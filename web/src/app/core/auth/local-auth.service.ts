import { Injectable, signal } from '@angular/core';

@Injectable({ providedIn: 'root' })
export class LocalAuthService {
  private token = '';
  readonly configured = signal(false);

  setToken(token: string): boolean {
    const normalizedToken = token.trim();
    if (!/^[\da-fA-F]{64}$/.test(normalizedToken)) return false;

    this.token = normalizedToken;
    this.configured.set(true);
    return true;
  }

  clearToken(): void {
    this.token = '';
    this.configured.set(false);
  }

  authorizationHeader(): string | null {
    return this.token ? `Bearer ${this.token}` : null;
  }
}
