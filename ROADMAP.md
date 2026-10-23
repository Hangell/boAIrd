# boAIrd Roadmap

> **Local-first AI Agent Control Plane built with Go + Angular**

boAIrd will be developed incrementally.

The initial priority is to build a functional, local-first, highly controllable MVP before adding distributed features, multi-user support, or complex infrastructure.

Primary stack:

```text
Backend:
Go

Frontend:
Angular
Angular Material
SCSS

Storage inicial:
SQLite

Protocolos:
REST
SSE
MCP
```

---

# Overview

```text
Phase 0   Bootstrap
Phase 1   Core Domain
Phase 2   Projects
Phase 3   Providers
Phase 4   Agents
Phase 5   Brain
Phase 6   Kanban
Phase 7   Conversations
Phase 8   Orchestrator
Phase 9   Angular Dashboard
Phase 10  Capabilities & Security
Phase 11  MCP
Phase 12  External Agents
Phase 13  Context Engine
Phase 14  Memory
Phase 15  Tools
Phase 16  Git & GitHub
Phase 17  Autonomy Modes
Phase 18  Human Review
Phase 19  Remote Mode
Phase 20  Advanced Runtime
```

---

# Phase 0 — Bootstrap

## Objective

Create the minimum Go project foundation.

Expected result:

```bash
boaird serve
```

Serving at:

```text
http://localhost:3110
```

## Backend

Implement:

- Go module;
- initial directory structure;
- CLI;
- HTTP server;
- configuration;
- structured logging;
- graceful shutdown;
- healthcheck;
- SQLite;
- migrations.

Initial structure:

```text
cmd/
internal/
configs/
migrations/
data/
```

## Endpoints

```text
GET /health
GET /api/v1/status
```

## Completion Criteria

```text
boaird serve

↓

API ONLINE
SQLite connected
health = OK
```

---

# Phase 1 — Core Domain

## Objective

Define the core domain before integrating any AI.

Initial entities:

```text
Project
Agent
Brain
Task
Run
Capability
Conversation
```

Enums:

```text
AgentStatus

ONLINE
IDLE
BUSY
WAITING
OFFLINE
STALE
ERROR
```

TaskStatus:

```text
BACKLOG
READY
WORKING
REVIEW
HUMAN_REVIEW
BLOCKED
FAILED
DONE
CANCELLED
```

AutonomyMode:

```text
MANUAL
SEMI_AUTO
AUTONOMOUS
```

## Criteria

Models are defined without a direct dependency on OpenAI, Claude, or another provider.

---

# Phase 2 — Projects

## Objective

Allow users to create and manage projects.

Implement:

```text
Create Project
Get Project
List Projects
Update Project
Archive Project
```

A project should have:

```text
name
description
workspace
repository
autonomy mode
human review defaults
settings
```

## API

```text
POST   /api/v1/projects
GET    /api/v1/projects
GET    /api/v1/projects/:id
PATCH  /api/v1/projects/:id
DELETE /api/v1/projects/:id
```

## Rule

A project may exist without complete configuration.

However:

> A project cannot run agents until it has an active Brain.

---

# Phase 3 — Providers

## Objective

Create a provider-independent abstraction.

Interface:

```go
type Provider interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

    Models(ctx context.Context) ([]Model, error)
}
```

Initial providers:

```text
OpenAI
Anthropic / Claude
DeepSeek
Google Gemini
OpenAI-compatible
```

## Implementation

```text
Provider Registry
Credential Management
Model Configuration
Provider Health
```

## API

```text
GET  /api/v1/providers
POST /api/v1/providers

GET  /api/v1/providers/:id/models
```

## Criteria

A call must work without the rest of the system knowing which SDK is being used.

---

# Phase 4 — Credentials

## Objective

Manage API keys securely.

Implement:

```text
Provider
    ↓
Credential
    ↓
Model
```

Never:

```text
Agent → raw API key
```

## Security

Use:

```text
encrypted secret
+
BOAIRD_MASTER_KEY
```

## Criteria

API keys must never appear in:

```text
logs
API responses
database plaintext
frontend payloads
```

---

# Phase 5 — Agents

## Objective

Allow specialized agents to be created.

Fields:

```text
name
avatar
description
role
runtime
provider
model
preferred language
instructions
capabilities
status
```

Examples:

```text
Backend Go
Frontend Angular
QA
DevOps
Security
Git Manager
Architect
Product Owner
```

## API

```text
POST  /api/v1/agents
GET   /api/v1/agents
GET   /api/v1/agents/:id
PATCH /api/v1/agents/:id
```

## Criteria

It must be possible to create two agents using different providers.

The role is a descriptive label and must not grant capabilities. API runtimes reference a stored credential by ID; an agent record and API response must never contain a raw provider secret. Agent status is runtime-managed and read-only through the configuration API.

Example:

```text
Backend → DeepSeek

QA → Claude
```

---

# Phase 6 — Brain

## Objective

Implement the project's central concept.

Rule:

```text
Every Project Requires One Active Brain
```

The Brain will be an Agent with special responsibilities.

## Responsibilities

The Brain may:

- analyze goals;
- guide agents;
- create tasks;
- assign tasks;
- review deliverables;
- analyze errors;
- request retries;
- act as QA when needed;
- notify the administrator.

## Initial Implementation

Initially:

```text
Brain via API
```

Example:

```text
OpenAI
Claude
Gemini
DeepSeek
```

An MCP-based Brain will come later.

## Criteria

```text
Project
  ↓
Brain
  ↓
Prompt
  ↓
Response
```

Each response must also have a persisted Run.

## Current implementation

- Projects can attach one active Brain to an API or MCP agent with the explicit `brain.execute` capability.
- Active Brain configuration can be replaced only with an authenticated administrator session and a required reason; the replacement is recorded as an immutable audit event.
- An API Brain can request a provider response through its configured provider adapter and credential reference; an MCP Brain uses authenticated, capability-checked MCP tools.
- Each execution persists its project, agent, provider, model, objective, status, and response in `runs`.
- The current runtime returns a proposed plan as text. It does not apply task mutations or grant capabilities.
- SQLite permits active Brains only on the API or MCP runtime and requires a configured active Brain before a project enters READY or RUNNING.

## API

```text
POST /api/v1/projects/{id}/brain
GET  /api/v1/projects/{id}/brain
PUT  /api/v1/projects/{id}/brain (administrator session + audit reason)
POST /api/v1/projects/{id}/brain/runs
```

---

# Phase 7 — Runs

## Objective

Every AI execution must produce a record.

Structure:

```text
Run

agent
task
provider
model
started_at
finished_at
status
input
output
error
tools
```

## Status

```text
PENDING
RUNNING
SUCCESS
FAILED
CANCELLED
```

## Criteria

Every model interaction must be auditable.

## Current implementation

- Runs persist actor agent, project/task references, runtime, provider, model, redacted input/output, tool names, status, timing, and sanitized error text.
- `GET /api/v1/projects/{id}/runs` filters by task or agent; `GET /api/v1/runs/{id}` retrieves a run.
- SQLite creates immutable audit events for Run creation and every status transition. Run responses include those linked events.
- Provider failure and cancellation are stored as terminal states without persisting provider error bodies.

---

# Phase 8 — Kanban

## Objective

Implement boAIrd's primary operational workflow.

Board:

```text
BACKLOG

READY

WORKING

REVIEW

HUMAN_REVIEW

BLOCKED

DONE
```

## Task

Each Task will have:

```text
title
description
status
priority
created_by
assigned_agent
human_review
parent_task
context
```

## Permissions

By default:

```text
Brain:
task.create = true

Worker:
task.create = false
```

Agents such as:

```text
PO
SDM
Project Manager
```

may be granted:

```text
task.create
```

## Criteria

Create a card and move it through the entire workflow.

## Current implementation

- Tasks persist in SQLite with validated workflow states, priority, assignment, parent reference, project human-review policy, and actor/reason metadata.
- The task service checks allowed transitions, active project state, agent capabilities for creation/assignment, and the mandatory human-review gate before completion.
- The local HTTP API supports task create/list/get, assignment, and status transitions. Each mutation requires an audit reason; callers cannot supply actor identity through request JSON.
- SQLite records immutable task creation, assignment, and status-change audit events. Integration tests cover capability denial, the complete review workflow, invalid terminal transitions, and audit attribution.
- Task mutations use persisted optimistic versions; stale assignment or transition writes return a conflict and cannot overwrite newer state or create duplicate audit decisions.
- REST and MCP task results expose the current version and tell callers to reload and retry after a concurrent update conflict.
- REST routes require the local administrator bearer token or HttpOnly session cookie (health/status remain public). Each route declares a capability; project-scoped operations check `project_members`. Task services reject the legacy local actor and require explicit grants. Authorization denials are stored immutably. The MCP adapter is implemented as a separate authenticated endpoint and uses explicit Agent capabilities, project scope, or exact task assignment before invoking the same domain services.

---

# Phase 9 — Conversations

## Objective

Allow communication between agents and humans.

Each Task must have a Conversation.

```text
Task
   ↓
Conversation
   ├── Brain
   ├── Worker
   ├── QA
   └── Human
```

A message must record:

```text
sender
timestamp
content
format
metadata
```

## Current implementation

- SQLite creates one conversation for each task, including tasks that existed when the migration is applied.
- Messages retain opaque content, content type, JSON metadata, sender type/ID, and timestamp; sensitive text and metadata are redacted before persistence.
- `GET` and `POST /api/v1/tasks/{id}/messages` require `BOAIRD_LOCAL_AUTH_TOKEN` and a project membership for the single `local-administrator` principal. The sender identity is set by the server, and callers cannot provide it in JSON.
- Existing projects are backfilled with local administrator membership, and new projects receive that membership automatically. Trusted service calls may add messages from the agent assigned to the task or the active Brain for its project.
- The one configured administrator is the only HTTP principal. Multi-user identity and membership administration remain future work. The dashboard exchanges the root token for a random opaque HttpOnly cookie session ID stored in server memory for 30 minutes; logout revokes it and restart invalidates all sessions. Cookie writes and logout require SameSite and a trusted dashboard Origin.

## Formats

Do not limit communication to human-readable text.

Future formats to support:

```text
text
markdown
json
yaml
binary reference
machine protocol
```

## Criteria

The full history must be accessible from the card.

---

# Phase 10 — Orchestrator

## Objective

Create the mechanism that coordinates the Brain, agents, and Tasks.

Flow:

```text
Brain
  ↓
Task
  ↓
Agent
  ↓
Run
  ↓
Review
  ↓
Brain
```

## Responsibilities

```text
task assignment
agent availability
execution
status transition
review flow
failure routing
```

## Failure Handling

Do not rely on automatic retries alone.

Flow:

```text
Worker fails
     ↓
Brain analyzes
     ↓
Brain adds instructions
     ↓
Worker retries
```

If the Brain is uncertain:

```text
Brain
  ↓
Administrator Notification
```

## Current implementation

- `internal/orchestrator` schedules only READY tasks that already have an assigned API Agent with `task.transition`. Every claim and transition goes through `task.Service` using the configured project Brain actor, so capabilities, autonomy, and Human Review are enforced rather than reimplemented. Dispatch and review first verify the Brain through `Brain.AuthorizeAction`: without `brain.execute` no provider call is made; a missing task grant skips the task instead of bypassing policy. Worker attempts are persisted as worker-kind Runs, so Brain failure-analysis Runs linked to the same task do not consume the attempt budget.
- External Agents are skipped because they claim work through authenticated MCP tools; the orchestrator does not push work to them.
- Each attempt creates a linked Run (`task_id`, `agent_id`, `runtime`, `provider`, `model`, input, output, and terminal status). Successful work moves to REVIEW as the assigned Agent.
- A failed or cancelled attempt is recorded as BLOCKED. The Brain analyzes the failure through `AnalyzeTaskFailure` (a Run linked to the same task), records retry instructions in the task conversation, and schedules a retry. Attempts are bounded by `BOAIRD_ORCHESTRATOR_MAX_ATTEMPTS`; when exhausted, the task stays BLOCKED and an `orchestrator.escalated` audit event is written for administrator follow-up.
- `POST /api/v1/tasks/{id}/review` applies the Brain verdict. APPROVE completes the task only when the Brain autonomy and project Human Review policy allow it; otherwise it moves the task to HUMAN_REVIEW. REWORK records the instructions and executes one more attempt within the same limit.
- `POST /api/v1/projects/{id}/orchestration/dispatch` returns one bounded outcome per scheduled task. Concurrent claims are detected by task versions, so a task cannot be executed twice.

---

# Phase 11 — Angular Bootstrap

## Objective

Create the web dashboard.

Stack:

```text
Angular
Angular Material
SCSS
```

## Structure

```text
Overview

Projects

Agents

Providers

Logs

Settings
```

Layout:

```text
Sidebar
Header
Main Workspace
```

## Implementation

```text
routing
Angular Material theme
API client
global error handler
SSE service
i18n foundation
```

## Current implementation

- The `web/` standalone Angular workspace builds with strict TypeScript and SCSS, Angular Material, feature routes, and a responsive shell.
- `API_BASE_URL` defaults to the loopback API in development; production builds use `/api/v1`. The browser projects a view of backend state and does not own task status.
- The project list and project detail view use the current project, task, Brain, agent, and run endpoints to show operational status, blocked tasks, and recent failed runs, with explicit loading, empty, and retryable error states.
- The Kanban board groups backend tasks by API status, submits status changes with a required audit reason, and refreshes from the server. Task detail pages expose audit history, text conversation messages, linked runs, and human-review state.
- Shared infrastructure includes a typed HTTP client, an SSE client with caller-provided event decoding, and a global error handler that avoids logging error payloads.
- The shell includes semantic landmarks, keyboard skip navigation, visible focus, reduced-motion styling, and Angular i18n markers with English as the source locale.

---

# Phase 12 — Project Dashboard

## Objective

Create an operational view for each project.

Screen:

```text
Project

├── Overview
├── Kanban
├── Brain
├── Agents
├── Conversations
├── Runs
├── Logs
└── Settings
```

## Overview

Show:

```text
Brain status

Agents online
Agents working

Tasks backlog
Tasks working
Tasks review

Errors
Recent activity
```

---

# Phase 13 — Kanban Angular

## Objective

Turn Tasks into a visual experience.

Interface:

```text
BACKLOG   READY   WORKING   REVIEW   DONE
```

Support:

```text
drag & drop
filters
agent avatar
priority
status
human review
```

On click:

```text
Task Modal
```

Show:

```text
Description
Agent
Conversation
Runs
Context
Logs
Review
```

---

# Phase 14 — Agent Dashboard

## Objective

Allow agents to be fully configured.

Screen:

```text
Avatar
Name
Role
Runtime
Provider
Model
Instructions
Language
Capabilities
Tools
Status
Current Task
Logs
```

---

# Phase 15 — Brain Dashboard

## Objective

Allow the Brain to be configured and monitored.

Configuration:

```text
Brain Agent
Model
Runtime
Instructions
Capabilities
Autonomy Mode
```

Show:

```text
status
current activity
current reasoning request
tasks managed
failures
notifications
```

The first dashboard iteration uses project selection, the active Brain and agent status, recent Brain Runs as proposals, and tasks waiting in `HUMAN_REVIEW`. Replacing an active Brain requires an authenticated administrator session and records a reasoned audit event.

---

# Phase 16 — Capabilities

## Objective

Create a security layer independent of role.

Examples:

```text
filesystem.read
filesystem.write

shell.execute

task.read
task.create
task.assign
task.review

git.read
git.commit
git.branch
git.push
git.merge

github.issue.read
github.issue.write

github.pr.read
github.pr.create
github.pr.review
github.pr.merge
```

Rule:

```text
Role != Permission
```

## Criteria

- Sensitive REST operations declare required capabilities and are checked in Go.
- Authorization denies by default, independent of role labels or UI state.
- Project-scoped reads and writes require local administrator project membership.
- Denials fail closed and create immutable audit records.
- The capability package is used by REST and MCP entry points; tool-level tests cover MCP authentication, session identity binding, and scoped operations.
- Tests cover explicit grants, missing grants, anonymous REST access, and cross-project denial.

## Current implementation

- The local bearer token authenticates every `/api/v1` route except status; the browser exchanges it for a SameSite=Strict HttpOnly cookie.
- Route policies map project, credential, agent, Brain, run, task, and conversation operations to explicit capabilities. Project-scoped routes validate the single administrator's membership before invoking managers.
- Task services independently require explicit capabilities and project scope for administrator actors, check active Brain/agent grants, reject the legacy unprivileged local actor, and audit denied operations.
- The API and MCP adapters record authorization denials in an immutable SQLite table. Agent project memberships are persisted in `agent_projects`; actual grants and revocations are audited in `audit_events`.
- MCP project-wide operations require explicit persisted Agent scope. Assignment grants only task-specific actions after checking the exact assignee. An active Brain derives scope only from its Brain-to-project relation.

A Backend agent may have:

```text
filesystem.write
```

while lacking:

```text
git.push
```

---

# Phase 17 — Autonomy Modes

Autonomy is enforced by the Brain domain before its actions reach task services.
Every action still needs its declared capability, active project scope, and
project policy. Changing a mode requires the local administrator and emits an
immutable `brain.autonomy.changed` audit event.

## Manual Mode

The Brain may produce plans and request administrator help. Task creation,
assignment, review, and completion require human intervention.

```text
MANUAL
```

---

## Semi-Auto Mode

The Brain may create and assign tasks, track work, and review results when it
has the matching capabilities. Task completion always requires human approval.

```text
SEMI_AUTO
```

---

## Autonomous Mode

```text
AUTONOMOUS
```

The Brain may:

```text
create tasks
assign agents
review
retry
reprioritize
finish tasks
```

Completion is allowed only when the Brain has `task.complete` and the project
does not require human review. It must never exceed its capabilities or bypass
project review policy. This phase gates Brain actions; provider output remains
proposed text and does not itself execute tools.

---

# Phase 18 — Human Review

## Objective

Enforce optional human approval for task completion, with a hard backend gate
and an immutable decision record containing reviewer identity, timestamp,
decision, and rationale. A transition from `HUMAN_REVIEW` to `DONE` records
approval; a transition back to `WORKING` records rejection and returns the task
to execution. Brain and worker actors cannot resolve a pending review.

When:

```text
human_review = false
```

Flow:

```text
Worker
  ↓
QA / Brain
  ↓
DONE
```

When:

```text
human_review = true
```

Flow:

```text
Worker
  ↓
QA / Brain
  ↓
HUMAN_REVIEW
  ↓
Administrator
  ↓
DONE
```

---

# Phase 19 — MCP Server

## Objective

Turn boAIrd into an MCP server.

Initial tools:

```text
boaird.project.get

boaird.agent.list
boaird.agent.get
boaird.agent.status

boaird.task.list
boaird.task.get
boaird.task.claim
boaird.task.update
boaird.task.complete
boaird.task.fail

boaird.conversation.read
boaird.conversation.send

boaird.context.get
boaird.brain.request
boaird.run.get
```

## Criteria

An MCP client must be able to:

```text
connect
↓
list tasks
↓
claim task
↓
send update
↓
complete task
```

## Current implementation

- `/mcp` uses authenticated Streamable HTTP and checks expiring persisted token verifiers on every request.
- The authenticated token determines Agent identity; stateful MCP sessions reject requests authenticated as a different Agent.
- Tools use explicit capabilities and project scope. MCP Workers may atomically self-assign an unassigned READY task only with persisted project membership and both `task.assign` and `task.transition`; version checks reject competing claims. The authenticated active MCP Brain delegates through `boaird.task.assign` with `expected_version`, and task mutations enter the task service as `ActorBrain` so autonomy, workflow, and Human Review policy remain enforced.
- Agent project memberships are stored in `agent_projects` and managed by the authenticated administrator at `PUT`/`DELETE /api/v1/projects/{projectID}/agents/{agentID}`. The Agent response exposes read-only `project_ids`; changes are written to immutable audit history.
- Project reads, Brain Runs, and other project-wide MCP operations require explicit membership. Assignment does not add project scope; active Brain scope is derived from the Brain-project relation. MCP scope denials are recorded.
- Task transitions, Human Review checks, conversation access, and Brain autonomy rules remain enforced by domain services.
- MCP Agent tokens are issued, rotated, revoked, and listed through administrator-only REST operations. Only SHA-256 verifiers are persisted; plaintext is returned once. Expiry is explicit and capped at 365 days; legacy environment tokens are imported once with a 90-day expiry. Lifecycle events are immutable and contain no secret material. Revocation blocks the next request on an existing MCP session; idle sessions expire after 30 minutes.
- Integration tests cover authentication, session binding, task-completion policy, and assigned-Agent conversation scope.

---

# Phase 20 — External Agents

## Objective

Allow agents to run outside the main process.

Examples:

```text
Codex
VS Code Agent
Claude
Cursor
custom agent
local CLI
```

Initial model:

```text
Pull
```

Flow:

```text
Agent
  ↓
connect
  ↓
request task
  ↓
boAIrd
```

---

# Phase 21 — Heartbeat

## Objective

Monitor external agents.

States:

```text
ONLINE
IDLE
BUSY
WAITING
STALE
OFFLINE
ERROR
```

Flow:

```text
heartbeat
heartbeat
heartbeat

↓

timeout

↓

STALE

↓

OFFLINE
```

## Implementation

```text
authenticated MCP activity
      ↓
agent_sessions (agent_id, token_id, last_seen_at)
      ↓
derived status  (IDLE / ONLINE / BUSY / WAITING / ERROR / STALE / OFFLINE)
      ↓
agent_status_events (immutable) + persisted agents.status
```

- Status is derived on read and reconciled in the background; clients never
  submit a status.
- BUSY and WAITING come from assigned tasks in `WORKING` and `REVIEW`; ERROR
  comes from a recent `FAILED` task.
- Thresholds are configurable and the clock is injectable, so timeout
  boundaries are covered by tests.
- `GET /api/v1/agents/{id}/status-events` exposes the transition history.

---

# Phase 22 — External Brain

## Objective

Allow the Brain to run externally as well.

Examples:

```text
Codex in VS Code

Claude app

GPT client

Gemini

local AI
```

boAIrd must handle the following in the same way:

```text
BrainRuntime
```

regardless of where it runs.

The first external implementation is an authenticated MCP Agent configured as
the project's active Brain. MCP tools remain the execution contract and enforce
the Agent's explicit capabilities and project policies. Provider-run endpoints
remain API-runtime-only and reject MCP Brains without creating a provider run.

---

# Phase 23 — Agent Runtime Abstraction

Implement:

```go
type AgentRuntime interface {

    Execute(
        ctx context.Context,
        request ExecutionRequest,
    ) (*ExecutionResult, error)

    Status(
        ctx context.Context,
    ) AgentStatus
}
```

Implementations:

```text
API Runtime
MCP Runtime
Local Runtime
Remote Runtime
```

## Current implementation

- `internal/runtime` defines the provider-independent `AgentRuntime` contract:
  `Runtime()`, `RequiredCapabilities()`, `Execute`, and `Status`. Domain services
  depend only on this contract.
- `APIExecutor` executes API-runtime Agents through their configured provider and
  declares `task.transition` as its required capability.
- MCP, local, and remote adapters fail closed: their `Execute` returns
  `ErrUnsupportedRuntime` because MCP Agents pull work through authenticated
  tools and local/remote execution are not yet implemented, while `Status` still
  reports the persisted Agent state.
- `Registry` resolves adapters by `agent.Runtime`, and `Enforcer` applies the
  shared policy to every adapter: Agent resolution, declared capability checks,
  a bounded deadline, and uniform cancellation/timeout/denial/failure error
  mapping.
- The orchestrator consumes the abstraction through the `Executor` interface; it
  still schedules only push-executable Agents and owns the per-execution
  deadline. Contract tests run against every adapter and cover success, denial,
  cancellation, timeout, and failure.

---

# Phase 24 — Context Engine

## Objective

Select context intelligently.

Initial implementation: bounded project/task context composition for API Agent Runs with source/trust provenance, deterministic selection, project memory, authorized conversation and same-task Run history, and explicit omission reporting. Files, Git, issues, PRs, and documentation require scoped readers and remain unavailable until those are implemented.

Final context:

```text
Brain selection
+
Agent request
+
boAIrd Context Engine
```

Sources:

```text
Project
Task
Files
Conversation
Memory
Runs
Git
Issues
PRs
Documentation
```

---

# Phase 25 — Memory Engine

## Objective

Reduce cost and excess context.

Flow:

```text
Raw History
    ↓
Brain Summary
    ↓
Optimized Memory
```

Categories:

```text
Architecture Decisions

Project Decisions

Important Context

Known Problems

Current State

Task History
```

## Rule

Raw history remains available.

However, it does not need to be sent to models on every run.

---

## Implemented

- `internal/memory` — provider-independent `Summary` domain and `Service`
  (`Record`, `List`, `Decisions`, `Select`, `Prune`).
- `internal/storage/memory.go` + migration `0019_project_memory.sql` — versioned
  summaries keyed by `(project_id, category, version)`, with source provenance.
- App wiring connects the Memory Service to task context selection. Work without
  Human Review is summarized after Brain approval; with Human Review, summaries
  are recorded only after the task transitions from `HUMAN_REVIEW` to `DONE`.
- Version numbers are allocated atomically by SQLite; archived projects reject
  both recording and pruning.
- Secrets are redacted before persistence; redaction is fail-closed.
- `Architecture Decision` and `Project Decision` summaries are canonical and are
  never pruned. Ephemeral categories retain the newest
  `MaxSummariesPerCategory` summaries per category.
- `Select` returns a bounded, reproducible selection and reports omitted items,
  so raw history is never sent to a model in full.
- Raw conversations and runs are never deleted; only summaries are pruned.

---

# Phase 26 — Tools

## Objective

Create a generic tool system.

Structure:

```text
Tool Registry
```

Initial tools:

```text
Filesystem
Shell
Git
GitHub
```

Future tools:

```text
Mermaid
PlantUML
draw.io
Docker
Database
Browser
Kubernetes
```

---

# Phase 27 — Shell

## Objective

Controlled command execution.

Do not grant unrestricted shell access by default.

Capabilities:

```text
shell.execute
shell.go
shell.node
shell.angular
shell.nest
```

Example:

Backend Go:

```text
go test
go build
go vet
go fmt
```

Frontend Angular:

```text
ng
npm
npx
```

---

# Phase 28 — Git

## Objective

Allow controlled Git operations.

```text
git.read
git.diff
git.branch
git.commit
git.push
git.merge
```

Also support:

```text
Git Manager Agent
```

---

# Phase 29 — GitHub

Implement:

```text
repositories
issues
pull requests
reviews
```

Capabilities:

```text
github.issue.create

github.pr.create
github.pr.review
github.pr.merge
```

---

# Phase 30 — Specialized Agents

Add support for more advanced profiles.

Examples:

```text
Software Architect

Security Engineer

QA

Git Manager

DevOps

PO

SDM
```

Each profile may have exclusive tools.

Example:

```text
Software Architect

├── Mermaid
├── PlantUML
└── draw.io
```

---

## Implemented

- `internal/profile` — built-in profile catalog (`qa`, `security`, `devops`,
  `git-manager`, `architect`, `product-owner`, `documentation`, `sdm`) with
  responsibility descriptions, instructions, default runtime, suggested tools,
  and suggested capabilities. Profiles do not define task-bound authorization scope.
- Profile names never grant authority: a profile's effective capabilities and
  tools come only from an administrator-configured, project-scoped override.
- `internal/storage/profile.go` + migration `0020_project_profiles.sql` persist
  per-project overrides and append immutable `profile.created` / `profile.updated`
  events in the same SQLite transaction.
- API: `GET /api/v1/profiles` lists the catalog;
  `GET /api/v1/projects/{id}/profiles` and `PUT /api/v1/projects/{id}/profiles/{key}`
  resolve and customize the effective profile for one project. Suggested tools
  are the only tools an override may expose.

---

# Phase 31 — Observability

## Objective

Understand everything that happens.

Dashboard:

```text
Runs
Logs
Errors
Agent activity
Task history
```

Categories:

```text
system
brain
agent
task
provider
mcp
tool
security
git
api
```

---

# Phase 32 — Audit

Record critical actions.

```text
Actor

Action

Resource

Timestamp

Result
```

Especially:

```text
shell
filesystem
git
issues
PR
permissions
credentials
```

---

# Phase 33 — Notifications

Create an internal notification system.

Examples:

```text
Brain needs help

Agent failed

Human review required

External agent disconnected

Provider unavailable
```

---

# Phase 34 — Internationalization

Initial dashboard languages:

```text
pt-BR
en-US
```

Designed to support additional languages.

Agents may also have a preferred language.

---

# Phase 35 — Remote Mode

## Objective

Allow secure operation outside localhost.

Implement:

```text
authentication
TLS
API tokens
remote dashboard
remote MCP
```

---

# Phase 36 — PostgreSQL

SQLite remains the default for local use.

PostgreSQL can be added for larger environments.

```text
SQLite
   ↓
PostgreSQL
```

Repositories should prevent a strong dependency on the database from the start.

---

# Phase 37 — Distributed Workers

Only when truly necessary, and only after a maintainer-approved decision.

This phase is a **decision gate**, recorded in
[ADR 0001](docs/adr/0001-gate-distributed-workers.md). The gate stays closed
while the single-process design (Go core, one SQLite writer, pull-based MCP
claims) carries the target workload.

It opens only when measured evidence shows at least one of:

```text
sustained task-state writes > 25/s for 10 minutes, or write p95 > 250 ms
board list reads > 30/s with p95 > 500 ms
an availability target a process restart cannot meet
execution on hosts that cannot share one authoritative board
delivery guarantees the pull model cannot express
```

When the gate trips, the response order is fixed: measure again, optimize the
single node, Phase 36 (PostgreSQL), and only then consider a broker.

Until approval:

```text
no broker
no queue service
no distributed worker runtime
```

Baseline evidence: [in-process measurements](docs/measurements/2026-10-22-in-process-baseline.md).

Possible technologies, if the gate ever opens:

```text
NATS
Valkey
Redis Streams
RabbitMQ
```

Do not implement this until the gate opens and the ADR is approved.

---

# Phase 38 — Multi-User

Future.

Implement:

```text
Users
Organizations
Roles
Permissions
Project membership
```

---

# Phase 39 — Agent Templates

Allow reusable agents.

```text
Global Agent Template
        ↓
Project Agent
```

Examples:

```text
Go Senior

Angular Senior

Security Reviewer

Git Manager
```

---

# Phase 40 — Tool Marketplace / Extensions

Future.

Allow tools to be added without changing the core.

```text
boAIrd Tool SDK
```

Possible integrations:

```text
GitHub
GitLab
Jira
Linear
Docker
Kubernetes
PlantUML
draw.io
Databases
Cloud providers
```

---

# MVP Definition

The MVP will be considered functional when this workflow is in place:

```text
User starts boAIrd
      ↓
opens Angular Dashboard
      ↓
creates Project
      ↓
configures Provider
      ↓
creates Brain
      ↓
creates Worker
      ↓
creates Task
      ↓
Brain analyzes Task
      ↓
Brain assigns Worker
      ↓
Worker executes
      ↓
Task → REVIEW
      ↓
Brain reviews
      ↓
Task → DONE
```

The dashboard must provide access to the full history.

---

# MVP 1

Backend:

```text
Go
SQLite
REST
SSE
```

Frontend:

```text
Angular
Angular Material
SCSS
```

AI:

```text
OpenAI
DeepSeek
```

Initial workflow:

```text
Brain API
+
Worker API
```

---

# MVP 2

Add:

```text
Claude
Gemini

Kanban completo

Conversations

Capabilities

Human Review
```

---

# MVP 3

Add:

```text
MCP Server

External Agents

Codex / VS Code integration

heartbeat
```

---

# MVP 4

Add:

```text
External Brain

Context Engine

Memory optimization
```

---

# Beta

The project may enter Beta once it has:

```text
Projects

Brain

Agents

Providers

Kanban

Conversations

Capabilities

Manual mode

Semi-auto mode

Autonomous mode

Human Review

MCP

External Agents

Context Engine

Memory

Logs

Audit

Git integration
```

---

# Post-Beta

```text
GitHub advanced integration

specialized tools

remote deployment

PostgreSQL

multi-user

distributed workers

tool extensions
```

---

# Priorities

## P0 — Required

```text
Go Core
Projects
Providers
Agents
Brain
Tasks
Kanban
Runs
Angular Dashboard
```

---

## P1 — High Priority

```text
Conversations
Capabilities
Autonomy
Human Review
MCP
External Agents
```

---

## P2 — Further Development

```text
Context Engine
Memory
Tools
Git
GitHub
Observability
```

---

## P3 — Scale

```text
Remote Mode
PostgreSQL
Multi-user
Distributed workers
Extensions
```

---

# Architectural Rules

## Rule 1

The core does not depend on a specific model.

```text
Model Agnostic
```

---

## Rule 2

An Agent is not an API.

```text
Agent
  ↓
Runtime
```

A Runtime can be:

```text
API
MCP
Local
Remote
```

---

## Rule 3

A Brain must always exist.

```text
Project
   ↓
Brain
```

---

## Rule 4

The administrator has the highest authority.

```text
Administrator
      ↓
Policy
      ↓
Brain
      ↓
Agents
```

---

## Rule 5

Autonomy does not grant permission.

```text
Autonomy != Capability
```

---

## Rule 6

Kanban belongs to the domain.

Not just to Angular.

---

## Rule 7

Every relevant execution must be auditable.

---

## Rule 8

Security must use:

```text
deny by default
+
least privilege
```

---

## Rule 9

Do not add distributed infrastructure too early.

The MVP will be:

```text
Modular Monolith
```

---

# Initial Architecture

```text
                   Angular Dashboard
                          │
                    REST / SSE
                          │
                          ▼
                 ┌────────────────┐
                 │     boAIrd     │
                 │     Go Core    │
                 └───────┬────────┘
                         │
        ┌────────────────┼────────────────┐
        │                │                │
      Brain          Orchestrator        MCP
        │                │                │
        │             Tasks              │
        │                │                │
        └─────────── Agents ──────────────┘
                         │
                    AgentRuntime
                         │
          ┌──────────────┼──────────────┐
          │              │              │
         API            MCP           Local
          │
          ▼
       Providers
          │
    ┌─────┼─────┬──────┐
    │     │     │      │
 OpenAI Claude DeepSeek Gemini
```

---

# First Technical Milestone

The first development milestone should be:

```bash
go run ./cmd/boaird serve
```

Result:

```text
boAIrd v0.0.1-dev

API          READY
SQLite       READY
Event Bus    READY

Listening:
http://localhost:3110
```

---

# First Functional Milestone

Next:

```text
Create Project
      ↓
Create Brain
      ↓
Create Worker
      ↓
Create Task
      ↓
Brain assigns
      ↓
Worker executes
      ↓
Brain reviews
      ↓
DONE
```

This is the point at which boAIrd can be considered a functional **AI Agent Control Plane**.

---

# Recommended Development Order

Recommended order:

```text
Go Core

↓

Domain

↓

Projects

↓

Providers

↓

Agents

↓

Brain

↓

Tasks

↓

Kanban

↓

Orchestrator

↓

Angular

↓

Capabilities

↓

MCP

↓

External Agents

↓

Context

↓

Memory

↓

Tools
```

Avoid starting MCP, Angular, and multiple agent integrations at the same time.

First, build a small workflow that works end to end.

Expand after that.

---

# Current Target

```text
Version:
0.1.0

Target:
Local MVP

Architecture:
Modular Monolith

Backend:
Go

Dashboard:
Angular

Database:
SQLite

Primary Goal:
Brain + Worker + Kanban working end-to-end
```
