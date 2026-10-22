import { HttpClient } from '@angular/common/http';
import { inject, Injectable } from '@angular/core';
import { EMPTY, expand, map, Observable, reduce } from 'rxjs';
import { API_BASE_URL } from './api-config';
import { ApiPage } from './api-models';

@Injectable({ providedIn: 'root' })
export class ApiClient {
  private readonly http = inject(HttpClient);
  private readonly baseUrl = inject(API_BASE_URL).replace(/\/+$/, '');

  get<TResponse>(path: string): Observable<TResponse> {
    return this.http.get<TResponse>(this.endpoint(path));
  }

  post<TRequest, TResponse>(path: string, body: TRequest): Observable<TResponse> {
    return this.http.post<TResponse>(this.endpoint(path), body);
  }

  put<TRequest, TResponse>(path: string, body: TRequest): Observable<TResponse> {
    return this.http.put<TResponse>(this.endpoint(path), body);
  }

  patch<TRequest, TResponse>(path: string, body: TRequest): Observable<TResponse> {
    return this.http.patch<TResponse>(this.endpoint(path), body);
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
