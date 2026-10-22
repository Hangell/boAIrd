import { ErrorHandler, Injectable } from '@angular/core';

@Injectable()
export class DashboardErrorHandler implements ErrorHandler {
  handleError(_error: unknown): void {
    console.error('An unexpected dashboard error occurred.');
  }
}
