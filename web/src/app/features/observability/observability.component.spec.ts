import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { TestBed, ComponentFixture } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { API_BASE_URL } from '../../core/api/api-config';
import { ObservabilityComponent } from './observability.component';

const apiBaseUrl = 'http://127.0.0.1:3110/api/v1';
const projectsUrl = `${apiBaseUrl}/projects?limit=100&offset=0`;
const agentsUrl = `${apiBaseUrl}/agents?limit=100&offset=0`;

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

  function loadProjects(items: ReturnType<typeof project>[]): void {
    http.expectOne(projectsUrl).flush({ items, limit: 100, offset: 0 });
  }

  function flushPage(url: string, items: unknown[]): void {
    http.expectOne(`${apiBaseUrl}/${url}?limit=100&offset=0`).flush({ items, limit: 100, offset: 0 });
  }

  function flushBrain(projectId: string, status: number, body: object | null): void {
    http.expectOne(`${apiBaseUrl}/projects/${projectId}/brain`).flush(body, { status, statusText: status === 200 ? 'OK' : 'Not Found' });
  }

  function loadProject(projectId: string, runs: unknown[], tasks: unknown[], agents: unknown[]): void {
    loadProjects([project('p1'), project('p2')]);
    flushPage(`projects/${projectId}/runs`, runs);
    flushPage(`projects/${projectId}/tasks`, tasks);
    http.expectOne(agentsUrl).flush({ items: agents, limit: 100, offset: 0 });
  }

  function agentsMetric(fixture: ComponentFixture<ObservabilityComponent>): string {
    return (fixture.nativeElement.querySelector('.metrics article .metric') as HTMLElement).textContent?.trim() ?? '';
  }

  it('loads project-scoped operational data and filters runs by status and search', () => {
    const fixture = TestBed.createComponent(ObservabilityComponent);
    fixture.detectChanges();
    loadProjects([project('p1'), project('p2')]);
    loadProject('p1', [run('r1', 'SUCCESS'), run('r2', 'FAILED')], [task('t1')], [agent('a1', ['p1'])]);
    flushBrain('p1', 404, null);
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

  it('counts project member Agents even when no task is assigned to them', () => {
    const fixture = TestBed.createComponent(ObservabilityComponent);
    fixture.detectChanges();
    loadProjects([project('p1'), project('p2')]);
    loadProject('p1', [], [], [agent('member-a', ['p1']), agent('other-b', ['p2'])]);
    flushBrain('p1', 404, null);
    fixture.detectChanges();

    expect(agentsMetric(fixture)).toBe('1');
    expect(fixture.nativeElement.textContent).toContain('member-a');
    expect(fixture.nativeElement.textContent).not.toContain('other-b');
  });

  it('includes the active Brain Agent in the Agents metric without membership duplication', () => {
    const fixture = TestBed.createComponent(ObservabilityComponent);
    fixture.detectChanges();
    loadProjects([project('p1'), project('p2')]);
    loadProject('p1', [], [], [agent('member-a', ['p1'])]);
    flushBrain('p1', 200, brain('brain-agent'));
    http.expectOne(`${apiBaseUrl}/agents/brain-agent`).flush(agent('brain-agent', []), { status: 200, statusText: 'OK' });
    fixture.detectChanges();

    expect(agentsMetric(fixture)).toBe('2');
    expect(fixture.nativeElement.textContent).toContain('member-a');
    expect(fixture.nativeElement.textContent).toContain('brain-agent');
  });

  it('keeps the project selector populated when the empty option is chosen', () => {
    const fixture = TestBed.createComponent(ObservabilityComponent);
    fixture.detectChanges();
    loadProjects([project('p1'), project('p2')]);
    loadProject('p1', [], [], []);
    flushBrain('p1', 404, null);
    fixture.detectChanges();

    let select = fixture.nativeElement.querySelector('select') as HTMLSelectElement;
    select.value = '';
    select.dispatchEvent(new Event('change'));
    fixture.detectChanges();
    loadProjects([project('p1'), project('p2')]);
    fixture.detectChanges();

    select = fixture.nativeElement.querySelector('select') as HTMLSelectElement;
    const options = Array.from(select.querySelectorAll('option')).map((option) => (option as HTMLOptionElement).value);
    expect(options).toContain('p1');
    expect(options).toContain('p2');
    expect(fixture.nativeElement.textContent).toContain('Select a project');

    select.value = 'p2';
    select.dispatchEvent(new Event('change'));
    fixture.detectChanges();
    loadProject('p2', [run('r-p2', 'SUCCESS')], [], []);
    flushBrain('p2', 404, null);
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('Run history');
    expect(fixture.nativeElement.textContent).toContain('run-r-p2');
  });
});

function project(id: string) { return { id, name: id, description: '', status: 'ACTIVE', human_review_required: false }; }
function task(id: string) { return { id, project_id: 'p1', title: id, status: 'BLOCKED', priority: 'NORMAL', updated_at: '2026-10-20T00:00:00Z' }; }
function run(id: string, status: string) { return { id, project_id: 'p1', task_id: `task-${id}`, agent_id: 'a1', model: `model-${id}`, status, started_at: '2026-10-21T00:00:00Z', finished_at: '2026-10-21T00:01:00Z', output: `run-${id}`, audit_events: [] }; }
function agent(id: string, projectIds: string[]) { return { id, name: id, status: 'IDLE', project_ids: projectIds }; }
function brain(agentId: string) { return { id: 'b1', project_id: 'p1', agent_id: agentId, autonomy: 'AUTONOMOUS', active: true }; }
