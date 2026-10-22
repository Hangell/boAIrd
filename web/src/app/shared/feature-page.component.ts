import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { AsyncPipe } from '@angular/common';
import { ActivatedRoute } from '@angular/router';
import { map } from 'rxjs';
import { MatCardModule } from '@angular/material/card';

interface PageContent {
  title: string;
  description: string;
}

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [AsyncPipe, MatCardModule],
  selector: 'app-feature-page',
  template: `
    @if (content$ | async; as content) {
      <header class="page-header">
        <p class="eyebrow" i18n="@@app.workspaceLabel">boAIrd workspace</p>
        <h1>{{ content.title }}</h1>
        <p class="page-description">{{ content.description }}</p>
      </header>
      <mat-card appearance="outlined" class="feature-card">
        <mat-card-content>
          <p i18n="@@app.featurePlaceholder">This area is ready for its feature view. Project and task state will come from the boAIrd API.</p>
        </mat-card-content>
      </mat-card>
    }
  `,
  styles: `
    :host { display: block; max-width: 78rem; margin-inline: auto; }
    .page-header { margin-block: 1.5rem 2rem; }
    .eyebrow { color: var(--boaird-accent); font-size: .75rem; font-weight: 700; letter-spacing: .12em; margin: 0 0 .75rem; text-transform: uppercase; }
    h1 { color: var(--boaird-ink); font-size: clamp(2rem, 4vw, 3rem); letter-spacing: -.04em; line-height: 1.1; margin: 0; }
    .page-description { color: var(--boaird-muted); font-size: 1rem; line-height: 1.6; margin: .75rem 0 0; max-width: 48rem; }
    .feature-card { background: var(--boaird-surface); border-color: var(--boaird-border); border-radius: 1rem; color: var(--boaird-muted); }
  `,
})
export class FeaturePageComponent {
  private readonly route = inject(ActivatedRoute);

  protected readonly content$ = this.route.data.pipe(
    map((data): PageContent => ({
      title: typeof data['title'] === 'string' ? data['title'] : 'boAIrd',
      description: typeof data['description'] === 'string' ? data['description'] : '',
    })),
  );
}
