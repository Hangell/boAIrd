import { inject, Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { API_BASE_URL } from './api-config';

@Injectable({ providedIn: 'root' })
export class SseClient {
  private readonly baseUrl = inject(API_BASE_URL).replace(/\/+$/, '');

  connect<TEvent>(path: string, decode: (value: unknown) => TEvent): Observable<TEvent> {
    return new Observable<TEvent>((subscriber) => {
      const endpoint = `${this.baseUrl}/${path.replace(/^\/+/, '')}`;
      const source = new EventSource(endpoint, { withCredentials: true });

      source.onmessage = (event: MessageEvent<string>) => {
        try {
          const value: unknown = JSON.parse(event.data);
          subscriber.next(decode(value));
        } catch {
          subscriber.error(new Error('The event stream returned an invalid event.'));
          source.close();
        }
      };

      source.onerror = () => {
        if (source.readyState === EventSource.CLOSED) {
          subscriber.error(new Error('The event stream is unavailable.'));
        }
      };

      return () => source.close();
    });
  }
}
