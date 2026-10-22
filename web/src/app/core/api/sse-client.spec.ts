import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { API_BASE_URL } from './api-config';
import { SseClient } from './sse-client';

class FakeEventSource {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSED = 2;

  readyState = FakeEventSource.OPEN;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  closed = false;

  constructor(
    readonly url: string,
    readonly options?: EventSourceInit,
  ) {
    FakeEventSource.instances.push(this);
  }

  close(): void {
    this.closed = true;
    this.readyState = FakeEventSource.CLOSED;
  }

  static instances: FakeEventSource[] = [];
}

describe('SseClient', () => {
  let client: SseClient;

  beforeEach(() => {
    FakeEventSource.instances = [];
    vi.stubGlobal('EventSource', FakeEventSource);
    TestBed.configureTestingModule({
      providers: [{ provide: API_BASE_URL, useValue: 'http://127.0.0.1:3110/' }],
    });
    client = TestBed.inject(SseClient);
  });

  afterEach(() => vi.unstubAllGlobals());

  it('decodes events from the normalized endpoint without credentials', () => {
    const values: string[] = [];
    client
      .connect('api/v1/events', (value) => String((value as { message: string }).message))
      .subscribe((value) => values.push(value));

    const source = FakeEventSource.instances[0];
    expect(source.url).toBe('http://127.0.0.1:3110/api/v1/events');
    expect(source.options).toEqual({ withCredentials: false });
    source.onmessage?.({ data: '{"message":"ready"}' } as MessageEvent<string>);
    expect(values).toEqual(['ready']);
  });

  it('errors and closes when an event cannot be decoded', () => {
    let receivedError: Error | undefined;
    client
      .connect('events', (value) => value)
      .subscribe({ error: (error: Error) => (receivedError = error) });

    const source = FakeEventSource.instances[0];
    source.onmessage?.({ data: 'not-json' } as MessageEvent<string>);

    expect(receivedError?.message).toBe('The event stream returned an invalid event.');
    expect(source.closed).toBe(true);
  });

  it('reports a closed stream and closes the source on unsubscribe', () => {
    let receivedError: Error | undefined;
    const subscription = client
      .connect('events', (value) => value)
      .subscribe({ error: (error: Error) => (receivedError = error) });
    const source = FakeEventSource.instances[0];
    source.readyState = FakeEventSource.CLOSED;
    source.onerror?.(new Event('error'));

    expect(receivedError?.message).toBe('The event stream is unavailable.');
    subscription.unsubscribe();
    expect(source.closed).toBe(true);
  });
});
