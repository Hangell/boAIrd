import { HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { API_BASE_URL } from '../api/api-config';
import { LocalAuthService } from './local-auth.service';

export const localAuthInterceptor: HttpInterceptorFn = (request, next) => {
  const authorization = inject(LocalAuthService).authorizationHeader();
  if (!authorization) return next(request);

  const apiBaseUrl = new URL(inject(API_BASE_URL));
  const requestUrl = new URL(request.url, apiBaseUrl);
  const basePath = apiBaseUrl.pathname.replace(/\/+$/, '');
  const isConfiguredAPI = requestUrl.origin === apiBaseUrl.origin
    && (requestUrl.pathname === basePath || requestUrl.pathname.startsWith(`${basePath}/`));
  if (!isConfiguredAPI) return next(request);

  return next(request.clone({ setHeaders: { Authorization: authorization } }));
};
