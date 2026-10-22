import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { ApiClient } from './api-client';
import { API_BASE_URL } from './api-config';

describe('ApiClient', () => {
  let client: ApiClient;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: API_BASE_URL, useValue: 'http://127.0.0.1:3110/' },
      ],
    });
    client = TestBed.inject(ApiClient);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('joins the configured base URL and a slash-prefixed path', () => {
    let response: { id: string } | undefined;

    client.get<{ id: string }>('/api/v1/projects/p1').subscribe((value) => (response = value));

    const request = http.expectOne('http://127.0.0.1:3110/api/v1/projects/p1');
    expect(request.request.method).toBe('GET');
    request.flush({ id: 'p1' });
    expect(response).toEqual({ id: 'p1' });
  });

  it('posts typed payloads to paths without a leading slash', () => {
    let response: { id: string } | undefined;

    client
      .post<{ name: string }, { id: string }>('api/v1/projects', { name: 'Example' })
      .subscribe((value) => (response = value));

    const request = http.expectOne('http://127.0.0.1:3110/api/v1/projects');
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({ name: 'Example' });
    request.flush({ id: 'p1' });
    expect(response).toEqual({ id: 'p1' });
  });

  it('patches typed configuration to a resource path', () => {
    let response: { id: string } | undefined;

    client.patch<{ name: string }, { id: string }>('agents/a1', { name: 'Updated' }).subscribe((value) => (response = value));

    const request = http.expectOne('http://127.0.0.1:3110/agents/a1');
    expect(request.request.method).toBe('PATCH');
    expect(request.request.body).toEqual({ name: 'Updated' });
    request.flush({ id: 'a1' });
    expect(response).toEqual({ id: 'a1' });
  });

  it('sends bearer authorization only with the requested PUT', () => {
    client.putWithBearer<{ reason: string }, { id: string }>('api/v1/projects/p1/brain', { reason: 'replace' }, 'admin-token').subscribe();

    const request = http.expectOne('http://127.0.0.1:3110/api/v1/projects/p1/brain');
    expect(request.request.method).toBe('PUT');
    expect(request.request.headers.get('Authorization')).toBe('Bearer admin-token');
    request.flush({ id: 'brain-1' });
  });

  it('appends pagination parameters after existing filters', () => {
    client.getAllPages<{ id: string }>('projects/p1/tasks?status=HUMAN_REVIEW').subscribe();

    const request = http.expectOne('http://127.0.0.1:3110/projects/p1/tasks?status=HUMAN_REVIEW&limit=100&offset=0');
    request.flush({ items: [], limit: 100, offset: 0 });
  });
});
