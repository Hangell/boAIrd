import { HttpClient, provideHttpClient, withInterceptors } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { API_BASE_URL } from '../api/api-config';
import { LocalAuthService } from './local-auth.service';
import { localAuthInterceptor } from './local-auth.interceptor';

describe('LocalAuthService and localAuthInterceptor', () => {
  let http: HttpTestingController;
  let auth: LocalAuthService;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        provideHttpClient(withInterceptors([localAuthInterceptor])),
        provideHttpClientTesting(),
        { provide: API_BASE_URL, useValue: 'http://127.0.0.1:3110/api/v1' },
      ],
    });
    http = TestBed.inject(HttpTestingController);
    auth = TestBed.inject(LocalAuthService);
  });

  afterEach(() => http.verify());

  it('attaches the in-memory token to requests under the configured API base path', () => {
    expect(auth.setToken('a'.repeat(64))).toBe(true);
    TestBed.inject(HttpClient).get('http://127.0.0.1:3110/api/v1/tasks/t1/messages').subscribe();

    const request = http.expectOne('http://127.0.0.1:3110/api/v1/tasks/t1/messages');
    expect(request.request.headers.get('Authorization')).toBe(`Bearer ${'a'.repeat(64)}`);
    request.flush({});
  });

  it('does not send the token to another origin', () => {
    expect(auth.setToken('b'.repeat(64))).toBe(true);
    TestBed.inject(HttpClient).get('https://third-party.example/api/v1/data').subscribe();

    const request = http.expectOne('https://third-party.example/api/v1/data');
    expect(request.request.headers.has('Authorization')).toBe(false);
    request.flush({});
  });

  it('keeps the token only in memory and clears it on request', () => {
    expect(auth.setToken('not-a-valid-token')).toBe(false);
    expect(auth.configured()).toBe(false);
    expect(auth.setToken('c'.repeat(64))).toBe(true);
    expect(auth.authorizationHeader()).toBe(`Bearer ${'c'.repeat(64)}`);

    auth.clearToken();

    expect(auth.configured()).toBe(false);
    expect(auth.authorizationHeader()).toBeNull();
  });
});
