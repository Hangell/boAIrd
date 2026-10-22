import type {} from '@angular/localize';
import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { BreakpointObserver } from '@angular/cdk/layout';
import { toSignal } from '@angular/core/rxjs-interop';
import { map } from 'rxjs';
import { MatButtonModule } from '@angular/material/button';
import { MatListModule } from '@angular/material/list';
import { MatSidenavModule } from '@angular/material/sidenav';
import { MatToolbarModule } from '@angular/material/toolbar';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatListModule, MatSidenavModule, MatToolbarModule, RouterLink, RouterLinkActive, RouterOutlet],
  selector: 'app-root',
  styleUrl: './app.scss',
  templateUrl: './app.html',
})
export class App {
  private readonly breakpointObserver = inject(BreakpointObserver);

  protected readonly isHandset = toSignal(
    this.breakpointObserver.observe('(max-width: 720px)').pipe(map((state) => state.matches)),
    { initialValue: false },
  );

  protected readonly navigation = [
    { label: $localize`:@@navigation.overview:Overview`, path: '/overview' },
    { label: $localize`:@@navigation.projects:Projects`, path: '/projects' },
    { label: $localize`:@@navigation.kanban:Kanban`, path: '/tasks' },
    { label: $localize`:@@navigation.conversations:Conversations`, path: '/conversations' },
    { label: $localize`:@@navigation.agents:Agents`, path: '/agents' },
    { label: $localize`:@@navigation.brain:Brain`, path: '/brain' },
    { label: $localize`:@@navigation.providers:Providers`, path: '/providers' },
    { label: $localize`:@@navigation.runs:Runs`, path: '/runs' },
    { label: $localize`:@@navigation.logs:Logs`, path: '/logs' },
  ];
}
