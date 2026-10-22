import { HttpClient } from '@angular/common/http';
import { inject, Injectable } from '@angular/core';
import { EMPTY, expand, map, Observable, reduce, catchError, of } from 'rxjs';
import { API_BASE_URL } from './api-config';
import { ApiPage } from './api-models';

@Injectable({ providedIn: 'root' })
export class ApiClient {
  private readonly http = inject(HttpClient);
  private readonly baseUrl = inject(API_BASE_URL).replace(/\/+$/, '');

  get<TResponse>(path: string): Observable<TResponse> {
    return this.http.get<TResponse>(this.endpoint(path), { withCredentials: true });
  }

  post<TRequest, TResponse>(path: string, body: TRequest): Observable<TResponse> {
    return this.http.post<TResponse>(this.endpoint(path), body, { withCredentials: true });
  }

  patch<TRequest, TResponse>(path: string, body: TRequest): Observable<TResponse> {
    return this.http.patch<TResponse>(this.endpoint(path), body, { withCredentials: true });
  }

  put<TRequest, TResponse>(path: string, body: TRequest): Observable<TResponse> {
    return this.http.put<TResponse>(this.endpoint(path), body, { withCredentials: true });
  }

  putWithBearer<TRequest, TResponse>(path: string, body: TRequest, token: string): Observable<TResponse> {
    return this.http.put<TResponse>(this.endpoint(path), body, {
      headers: { Authorization: `Bearer ${token}` },
      withCredentials: true,
    });
  }

  createLocalSession(token: string): Observable<void> {
    return this.http.post<void>(this.endpoint('/session'), {}, {
      headers: { Authorization: `Bearer ${token}` },
      withCredentials: true,
    });
  }

  hasLocalSession(): Observable<boolean> {
    return this.http.get<void>(this.endpoint('/session'), { withCredentials: true }).pipe(
      map(() => true),
      catchError(() => of(false)),
    );
  }

  clearLocalSession(): Observable<void> {
    return this.http.delete<void>(this.endpoint('/session'), { withCredentials: true });
  }

  getAllPages<TItem>(path: string): Observable<TItem[]> {
    const separator = path.includes('?') ? '&' : '?';
    const firstPage = (offset: number): Observable<ApiPage<TItem>> =>
      this.get<ApiPage<TItem>>(`${path}${separator}limit=100&offset=${offset}`);

    return firstPage(0).pipe(
      expand((page) => page.items.length === 100 ? firstPage(page.offset + page.limit) : EMPTY),
      map((page) => page.items),
      reduce((items, nextPage) => items.concat(nextPage), [] as TItem[]),
    );
  }

  private endpoint(path: string): string {
    return `${this.baseUrl}/${path.replace(/^\/+/, '')}`;
  }
}
