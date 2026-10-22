import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { of } from 'rxjs';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { API_BASE_URL } from '../../core/api/api-config';
import { TaskDetailComponent } from './task-detail.component';

const apiBaseUrl = 'http://127.0.0.1:3110/api/v1';

describe('TaskDetailComponent conversation history', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [TaskDetailComponent],
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        provideRouter([]),
        { provide: API_BASE_URL, useValue: apiBaseUrl },
        {
          provide: ActivatedRoute,
          useValue: {
            paramMap: of(convertToParamMap({ taskId: 'task-1' })),
            queryParamMap: of(convertToParamMap({ projectId: 'project-1' })),
          },
        },
      ],
    });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('loads older conversation pages using the cursor and keeps messages chronological', () => {
    const fixture = TestBed.createComponent(TaskDetailComponent);
    fixture.detectChanges();

    http.expectOne(`${apiBaseUrl}/tasks/task-1`).flush({
      id: 'task-1', project_id: 'project-1', title: 'Task', status: 'READY', priority: 'NORMAL',
      description: '', human_review_required: false, created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z', audit_events: [],
    });
    http.expectOne(`${apiBaseUrl}/tasks/task-1/messages?limit=50`).flush({
      id: 'conversation-1', project_id: 'project-1', task_id: 'task-1', next_before: 'm3',
      messages: [message('m3', 'newer one'), message('m4', 'newer two')],
    });
    http.expectOne(`${apiBaseUrl}/projects/project-1/runs?task_id=task-1&limit=10&offset=0`).flush({ items: [], limit: 10, offset: 0 });
    fixture.detectChanges();

    const loadOlder = fixture.nativeElement.querySelector('.older-button') as HTMLButtonElement;
    loadOlder.click();
    fixture.detectChanges();

    http.expectOne(`${apiBaseUrl}/tasks/task-1/messages?limit=50&before=m3`).flush({
      id: 'conversation-1', project_id: 'project-1', task_id: 'task-1', next_before: 'm1',
      messages: [message('m1', 'older one'), message('m2', 'older two')],
    });
    fixture.detectChanges();

    const contents = Array.from(fixture.nativeElement.querySelectorAll('.message-content'), (element: Element) => element.textContent?.trim());
    expect(contents).toEqual(['older one', 'older two', 'newer one', 'newer two']);

    (fixture.nativeElement.querySelector('.older-button') as HTMLButtonElement).click();
    fixture.detectChanges();
    http.expectOne(`${apiBaseUrl}/tasks/task-1/messages?limit=50&before=m1`).flush({
      id: 'conversation-1', project_id: 'project-1', task_id: 'task-1', messages: [],
    });
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.older-button')).toBeNull();
  });

  it('keeps task details available when conversation authentication fails', () => {
    const fixture = TestBed.createComponent(TaskDetailComponent);
    fixture.detectChanges();

    http.expectOne(`${apiBaseUrl}/tasks/task-1`).flush({
      id: 'task-1', project_id: 'project-1', title: 'Still visible', status: 'READY', priority: 'NORMAL',
      description: '', human_review_required: false, created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z', audit_events: [],
    });
    http.expectOne(`${apiBaseUrl}/tasks/task-1/messages?limit=50`).flush({}, { status: 401, statusText: 'Unauthorized' });
    http.expectOne(`${apiBaseUrl}/projects/project-1/runs?task_id=task-1&limit=10&offset=0`).flush({ items: [], limit: 10, offset: 0 });
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('Still visible');
    expect(fixture.nativeElement.querySelector('a[href="/settings"]')).not.toBeNull();
  });
});

function message(id: string, content: string) {
  return {
    id,
    conversation_id: 'conversation-1',
    sender_type: 'AGENT',
    sender_id: 'agent-1',
    content,
    content_type: 'text/plain',
    created_at: '2026-01-01T00:00:00Z',
  };
}
