import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { TestBed, ComponentFixture } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { BehaviorSubject, Observable, of, Subject } from 'rxjs';
import { API_BASE_URL } from '../../core/api/api-config';
import { ApiClient } from '../../core/api/api-client';
import { SseClient } from '../../core/api/sse-client';
import { ProjectDashboardComponent } from './project-dashboard.component';

describe('ProjectDashboardComponent', () => {
  let routeParams: BehaviorSubject<ReturnType<typeof convertToParamMap>>;
  let fixture: ComponentFixture<ProjectDashboardComponent>;
  let streams: Map<string, Subject<{ id: string }>>;
  let closed: string[];
  let get: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    routeParams = new BehaviorSubject(convertToParamMap({ projectId: 'p1' }));
    streams = new Map();
    closed = [];
    get = vi.fn((path: string) => {
      if (path.endsWith('/tasks')) return of([]);
      if (path.includes('/runs?')) return of({ items: [] });
      if (path.includes('/notifications?')) return of({ items: [] });
      if (path.endsWith('/brain')) return of({ agent_id: 'brain', active: true });
      if (path.startsWith('agents/')) return of({ id: 'brain', name: 'Brain', status: 'ONLINE' });
      return of({ id: path.split('/').at(-1), name: 'Project', description: '', status: 'READY' });
    });
    TestBed.configureTestingModule({
      imports: [ProjectDashboardComponent],
      providers: [
        { provide: ActivatedRoute, useValue: { paramMap: routeParams.asObservable() } },
        { provide: ApiClient, useValue: { get, getAllPages: vi.fn(() => of([])), post: vi.fn(() => of(undefined)) } },
        { provide: SseClient, useValue: { connect: (path: string) => new Observable<{ id: string }>((subscriber) => {
          const projectId = path.split('/')[1];
          const stream = new Subject<{ id: string }>();
          streams.set(projectId, stream);
          const subscription = stream.subscribe(subscriber);
          return () => { closed.push(projectId); subscription.unsubscribe(); };
        }) } },
        { provide: API_BASE_URL, useValue: 'http://localhost/api/v1' },
      ],
    });
    fixture = TestBed.createComponent(ProjectDashboardComponent);
    fixture.detectChanges();
  });

  afterEach(() => fixture.destroy());

  it('closes the prior project stream and ignores its late events after a route change', () => {
    const oldStream = streams.get('p1');
    expect(oldStream).toBeDefined();

    routeParams.next(convertToParamMap({ projectId: 'p2' }));
    fixture.detectChanges();

    expect(closed).toContain('p1');
    get.mockClear();
    oldStream?.next({ id: 'notice-from-p1' });
    expect(get.mock.calls.some(([path]) => path.includes('/p1/notifications'))).toBe(false);
  });
});
