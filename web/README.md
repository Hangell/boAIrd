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

The project list opens a project scoped overview using the existing project, task, Brain, agent, and run endpoints. The view reports blocked tasks and recent failed runs, and provides loading, empty, and retryable error states.

The Agents route lists agents and provides create/detail configuration forms using the versioned agent and credential metadata endpoints. Agent status is read-only. API agents reference an existing credential by ID; the dashboard never requests or displays raw secrets. Role is a descriptive label, while capabilities remain an explicit configuration list. API failures use generic messages so internal server responses are not shown in the UI.

The Brain route selects a project and shows the active Brain agent status, autonomy mode, recent Brain Run proposals, and tasks blocked or waiting for human review. Replacing an active Brain requires the local administrator bearer token and a reason. The dashboard sends the token only with that request and does not persist it.

The Kanban board groups tasks into the statuses defined by the API, submits each move with an audit reason, and reloads state from the server after a successful transition. Task details show audit history, text messages, linked runs, and the human review requirement.

Template strings use Angular `i18n` markers and TypeScript strings use `$localize` IDs. English is the source locale. The shell provides semantic landmarks, a keyboard skip link, visible focus indicators, and reduced-motion styling.
