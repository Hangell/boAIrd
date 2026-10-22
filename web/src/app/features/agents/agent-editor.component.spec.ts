import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { of } from 'rxjs';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiClient } from '../../core/api/api-client';
import { API_BASE_URL } from '../../core/api/api-config';
import { Agent, AgentConfiguration } from '../../core/api/api-models';
import { AgentEditorComponent } from './agent-editor.component';

const createdAgent: Agent = {
  id: 'agent-1', name: 'Planner', avatar: '', description: '', role: 'planner', runtime: 'API',
  provider: 'openai', model: 'example-model', credential_id: 'credential-1', instructions: '',
  language_preferences: ['en'], capabilities: ['brain.execute'], status: 'OFFLINE',
};

describe('AgentEditorComponent', () => {
  afterEach(() => TestBed.resetTestingModule());

  it('uses stored credential metadata and submits explicit capabilities without a secret', () => {
    const api = {
      get: vi.fn((path: string) => path === 'credentials'
        ? of({ items: [{ id: 'credential-1', name: 'OpenAI key', provider: 'openai', secret: 'must-not-render' }] })
        : of(createdAgent)),
      post: vi.fn(() => of(createdAgent)),
      patch: vi.fn(() => of(createdAgent)),
    };

    TestBed.configureTestingModule({
      imports: [AgentEditorComponent],
      providers: [
        provideHttpClient(), provideHttpClientTesting(), provideRouter([]),
        { provide: ApiClient, useValue: api },
        { provide: API_BASE_URL, useValue: 'http://127.0.0.1:3110/api/v1' },
        { provide: ActivatedRoute, useValue: { paramMap: of(convertToParamMap({ agentId: 'new' })) } },
      ],
    });

    const fixture = TestBed.createComponent(AgentEditorComponent);
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('OpenAI key (openai)');
    expect(fixture.nativeElement.textContent).not.toContain('must-not-render');

    const component = fixture.componentInstance as unknown as {
      form: AgentConfiguration;
      languagesText: string;
      capabilitiesText: string;
      save: () => void;
    };
    Object.assign(component.form, {
      name: 'Planner', role: 'planner', runtime: 'API', provider: 'openai', model: 'example-model',
      credential_id: 'credential-1',
    });
    component.languagesText = 'en';
    component.capabilitiesText = 'brain.execute\ntasks.read';
    component.save();

    expect(api.post).toHaveBeenCalledWith('agents', expect.objectContaining({
      role: 'planner', capabilities: ['brain.execute', 'tasks.read'], credential_id: 'credential-1',
    }));
    expect(JSON.stringify(api.post.mock.calls)).not.toContain('must-not-render');
  });

  it('rejects malformed capabilities before calling the API', () => {
    const api = { get: vi.fn(() => of({ items: [] })), post: vi.fn(), patch: vi.fn() };
    TestBed.configureTestingModule({
      imports: [AgentEditorComponent],
      providers: [
        provideHttpClient(), provideHttpClientTesting(), provideRouter([]),
        { provide: ApiClient, useValue: api },
        { provide: API_BASE_URL, useValue: 'http://127.0.0.1:3110/api/v1' },
        { provide: ActivatedRoute, useValue: { paramMap: of(convertToParamMap({ agentId: 'new' })) } },
      ],
    });

    const fixture = TestBed.createComponent(AgentEditorComponent);
    fixture.detectChanges();
    const component = fixture.componentInstance as unknown as {
      form: AgentConfiguration;
      capabilitiesText: string;
      save: () => void;
      validationError: () => string;
    };
    Object.assign(component.form, { name: 'Planner', role: 'planner', runtime: 'LOCAL' });
    component.capabilitiesText = 'Brain.Execute';
    component.save();

    expect(component.validationError()).toContain('capability identifiers');
    expect(api.post).not.toHaveBeenCalled();
  });
});
