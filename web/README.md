# boAIrd Dashboard

Standalone Angular dashboard foundation for the boAIrd local API. It uses strict TypeScript, Angular Material, SCSS, standalone routes, and OnPush change detection. The dashboard is a view of backend-owned project and task state.

## Requirements

- Node.js 22.22.3+ or 24.15.0+
- npm 10 or newer

## Development

```bash
npm install
npm start
```

The development environment uses `http://127.0.0.1:3110/api/v1`. Override it in `src/environments/environment.ts` when connecting to another local API. Production builds replace it with `/api/v1`.

## Commands

```bash
npm run build
npm test
npm run extract-i18n
```

Routes and feature pages live under `src/app`. Shared API transport is provided through `ApiClient` and the `API_BASE_URL` injection token. Do not cache task status as a second source of truth in the browser.

Template strings use Angular `i18n` markers and TypeScript strings use `$localize` IDs. English is the source locale. The shell provides semantic landmarks, a keyboard skip link, visible focus indicators, and reduced-motion styling.
