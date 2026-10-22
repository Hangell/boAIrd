import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { API_BASE_URL } from '../../core/api/api-config';
import { ObservabilityComponent } from './observability.component';

const apiBaseUrl = 'http://127.0.0.1:3110/api/v1';

describe('ObservabilityComponent', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [ObservabilityComponent],
      providers: [provideHttpClient(), provideHttpClientTesting(), provideRouter([]), { provide: API_BASE_URL, useValue: apiBaseUrl }],
    });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('loads project-scoped operational data and filters runs by status and search', () => {
    const fixture = TestBed.createComponent(ObservabilityComponent);
    fixture.detectChanges();
    http.expectOne(`${apiBaseUrl}/projects?limit=100&offset=0`).flush({ items: [project('p1'), project('p2')], limit: 100, offset: 0 });
    http.expectOne(`${apiBaseUrl}/projects/p1/runs?limit=100&offset=0`).flush({ items: [run('r1', 'SUCCESS'), run('r2', 'FAILED')], limit: 100, offset: 0 });
    http.expectOne(`${apiBaseUrl}/projects/p1/tasks?limit=100&offset=0`).flush({ items: [task('t1')], limit: 100, offset: 0 });
    http.expectOne(`${apiBaseUrl}/projects?limit=100&offset=0`).flush({ items: [project('p1'), project('p2')], limit: 100, offset: 0 });
    fixture.detectChanges();

    const text = fixture.nativeElement.textContent as string;
    expect(text).toContain('p1');
    expect(text).toContain('SUCCESS');
    expect(text).toContain('FAILED');
    expect(http.match((request) => request.url.includes('/projects/p2/')).length).toBe(0);

    const status = fixture.nativeElement.querySelectorAll('select')[1] as HTMLSelectElement;
    status.value = 'FAILED';
    status.dispatchEvent(new Event('change'));
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('run-r2');
    expect(fixture.nativeElement.textContent).not.toContain('run-r1');

    const search = fixture.nativeElement.querySelector('input[type="search"]') as HTMLInputElement;
    search.value = 'not-found';
    search.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('No runs match these filters.');
  });
});

function project(id: string) { return { id, name: id, description: '', status: 'ACTIVE', human_review_required: false }; }
function task(id: string) { return { id, project_id: 'p1', title: id, status: 'BLOCKED', priority: 'NORMAL', updated_at: '2026-10-20T00:00:00Z' }; }
function run(id: string, status: string) { return { id, project_id: 'p1', task_id: `task-${id}`, agent_id: 'a1', model: `model-${id}`, status, started_at: '2026-10-21T00:00:00Z', finished_at: '2026-10-21T00:01:00Z', output: `run-${id}`, audit_events: [] }; }
