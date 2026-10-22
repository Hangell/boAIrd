import { HttpClient } from '@angular/common/http';
import { inject, Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { API_BASE_URL } from './api-config';

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

  patch<TRequest, TResponse>(path: string, body: TRequest): Observable<TResponse> {
    return this.http.patch<TResponse>(this.endpoint(path), body);
  }

  private endpoint(path: string): string {
    return `${this.baseUrl}/${path.replace(/^\/+/, '')}`;
  }
}
