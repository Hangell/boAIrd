import { CanActivateFn, Router } from '@angular/router';
import { inject } from '@angular/core';
import { map } from 'rxjs';
import { ApiClient } from '../api/api-client';

export const localSessionGuard: CanActivateFn = (_route, state) => {
  const api = inject(ApiClient);
  const router = inject(Router);
  return api.hasLocalSession().pipe(
    map((authenticated) =>
      authenticated ? true : router.createUrlTree(['/login'], { queryParams: { returnUrl: state.url } }),
    ),
  );
};
