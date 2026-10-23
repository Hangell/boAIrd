# boAIrd

> **Local-first AI Agent Control Plane and Orchestration Runtime**

**boAIrd** is a local-first platform for creating, coordinating, supervising, and running AI agents.

The project grew from the need for a **highly controllable** environment where multiple agents can work together using APIs from different providers, external agents, IDE extensions, local models, or other runtimes.

boAIrd is not intended to be just a client for AI APIs.

It works as an **Agent Control Plane**, offering:

- a required Brain;
- specialized agents;
- a shared Kanban board;
- communication between agents;
- permission controls;
- autonomy levels;
- MCP integration;
- IDE integration;
- external APIs;
- local agents;
- project management;
- optimized memory;
- auditing;
- dashboard web;
- local or remote execution.

---

# Vision

The core idea behind boAIrd is simple:

```text
                    Administrator
                          │
                          ▼
                   ┌─────────────┐
                   │   boAIrd    │
                   │ ControlPlane│
                   └──────┬──────┘
                          │
                       Project
                          │
                    ┌─────┴─────┐
                    │           │
                  Brain       Kanban
                    │           │
          ┌─────────┼───────────┼─────────┐
          │         │           │         │
          ▼         ▼           ▼         ▼
      Backend    Frontend      QA      Git Manager
       Agent      Agent       Agent       Agent
```

Every project must have a **Brain**.

The Brain is the agent responsible for:

- understanding the project goal;
- guiding the other agents;
- creating tasks;
- distributing work;
- reviewing results;
- validating implementations;
- investigating failures;
- requesting corrections;
- performing tasks directly when needed;
- validating issues;
- reviewing Pull Requests;
- using the shell;
- running code;
- requesting human intervention when needed.

The recommendation is for the Brain to use the **most capable model or agent available to the user**.

---

# Objective

boAIrd aims to let users configure a team of agents that works like a real team:

```text
                        Brain
                          │
             ┌────────────┼─────────────┐
             │            │             │
             ▼            ▼             ▼
          Backend      Frontend         QA
             │            │             │
             └────────────┼─────────────┘
                          │
                         Git
                          │
                          ▼
                      Repository
```

Each agent can have:

- name;
- avatar;
- function;
- description;
- preferred language;
- model;
- provider;
- runtime;
- permissions;
- tools;
- instructions;
- memory;
- context;
- specialization.

---

# Principles

## Local First

boAIrd must run entirely on the user’s machine.

Example:

```bash
boaird serve
```

The server should be available at:

```text
http://localhost:3110
```

---

## Remote Ready

The same application may later run on:

- a home server;
- VPS;
- corporate server;
- homelab;
- private cloud.

Example:

```text
https://boaird.example.com
```

---

## Model Agnostic

The core must not depend on a specific model.

The Brain can be:

- OpenAI;
- Codex;
- Claude;
- DeepSeek;
- Gemini;
- Qwen;
- a VS Code agent;
- a local application;
- an MCP agent;
- a local model;
- a remote service.

---

## Provider Agnostic

boAIrd must support multiple providers.

Initially:

```text
OpenAI
Anthropic / Claude
DeepSeek
Google Gemini
OpenAI-compatible APIs
```

In the future:

```text
Qwen
Ollama
LM Studio
vLLM
OpenRouter
Groq
Mistral
custom providers
```

---

# Brain

Every boAIrd project must have an **active Brain**.

The Brain is the agents’ primary supervisor.

```text
Administrator
      │
      ▼
Project Policy
      │
      ▼
Brain
      │
      ▼
Agents
```

The Brain can:

- creating tasks;
- editing tasks;
- assigning tasks;
- guiding agents;
- monitoring execution;
- requesting corrections;
- validating results;
- moving cards;
- reviewing code;
- running code;
- running the shell;
- working with Git;
- working with Issues;
- working with Pull Requests;
- acting as QA when no QA agent exists;
- communicating with agents;
- summarizing memory;
- selecting context;
- informing the administrator.

---

# Authority

The administrator always has authority over the Brain.

```text
Administrator
      ↓
Project Policy
      ↓
Brain
      ↓
Specialized Agents
      ↓
Workers
```

No agent may exceed the permissions set by the administrator.

Even in fully autonomous mode:

```text
Autonomy != Permission
```

Fundamental rule:

> **Autonomy defines how much an agent can decide.
> Capability defines what it can execute.**

---

# Brain Runtime

The Brain does not have to run through an API.

It can use different runtimes:

```text
Brain
 │
 ├── API
 │
 ├── MCP
 │
 ├── IDE Agent
 │
 ├── Local Agent
 │
 └── Remote Agent
```

Example:

```yaml
brain:
  name: Codex Brain
  runtime: mcp
```

Or:

```yaml
brain:
  name: GPT Brain
  runtime: api
  provider: openai
  model: gpt
```

---

# Agent Runtime

The architecture must abstract how an agent is executed.

```go
type AgentRuntime interface {
    Execute(
        ctx context.Context,
        request ExecutionRequest,
    ) (*ExecutionResult, error)

    Status(
        ctx context.Context,
    ) AgentStatus

    Capabilities(
        ctx context.Context,
    ) []Capability
}
```

Possible implementations:

```text
AgentRuntime

├── APIAgent
│   ├── OpenAI
│   ├── Claude
│   ├── DeepSeek
│   └── Gemini
│
├── MCPAgent
│   ├── Codex
│   ├── VS Code
│   ├── Claude
│   └── external MCP client
│
├── LocalAgent
│
└── RemoteAgent
```

---

# Agents

Agents represent specialized members of a team.

They can be defined by:

- language;
- framework;
- responsibility;
- function;
- specialty;

Examples:

```text
Backend Go
Backend NestJS
Frontend Angular
Flutter Developer
Security
QA
DevOps
Git Manager
Software Architect
Documentation
Product Owner
Scrum Master
SDM
Database Engineer
```

---

# Agent Definition

Example:

```yaml
agent:

  name: Backend Go

  role: backend

  avatar: backend-go.png

  runtime: api

  provider: deepseek

  model: deepseek-chat

  language:
    - pt-BR
    - en-US
```

---

# Agent Communication

Agents can communicate freely.

Communication does not have to use natural language.

Agents can use:

- Portuguese;
- English;
- JSON structures;
- YAML;
- custom protocols;
- compressed data;
- binary representations;
- formats optimized for machine-to-machine communication.

The runtime must be able to store and display this communication.

---

# Conversation

Each task can have an associated conversation.

```text
Task #142
   │
   └── Conversation
          │
          ├── Brain
          ├── Backend
          ├── QA
          ├── Security
          └── Human
```

The dashboard must let users follow these conversations.

---

# Kanban

Kanban will be one of boAIrd’s core elements.

It will be more than a visual component.

It will be part of the application domain.

Initial workflow:

```text
BACKLOG
   ↓
READY
   ↓
WORKING
   ↓
REVIEW
   ↓
DONE
```

Additional states:

```text
BLOCKED
FAILED
HUMAN_REVIEW
CANCELLED
```

---

# Workflow

Example:

```text
User Objective
      │
      ▼
     Brain
      │
      ▼
Creates Tasks
      │
      ▼
    READY
      │
      ▼
Agent Claims Task
      │
      ▼
   WORKING
      │
      ▼
   REVIEW
      │
      ├── QA exists
      │      ↓
      │      QA
      │
      └── No QA
             ↓
           Brain
              │
              ▼
             DONE
```

---

# Creating Tasks

By default, regular agents cannot create tasks.

The following can create tasks:

```text
Brain
Administrator
```

Other agents may create tasks if the administrator allows it.

Example:

```text
Product Owner
SDM
Project Manager
Architect
```

Capability:

```text
task.create
```

---

# Task

Conceptual model:

```go
type Task struct {
    ID          string
    ProjectID   string

    Title       string
    Description string

    Status TaskStatus

    CreatedBy ActorID

    AssignedTo *AgentID

    HumanReview bool

    ParentID *string

    CreatedAt time.Time
    UpdatedAt time.Time
}
```

---

# Subtasks

Cards may have subtasks in the future.

```text
Task #42
 │
 ├── #42.1 Backend
 ├── #42.2 Frontend
 ├── #42.3 Tests
 └── #42.4 Documentation
```

---

# Human Review

Human review is optional.

Example:

```yaml
human_review: false
```

Workflow:

```text
Agent
  ↓
QA / Brain
  ↓
DONE
```

When:

```yaml
human_review: true
```

Workflow:

```text
Agent
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

# Autonomy Levels

Initially, boAIrd will have three levels.

## Manual

```text
manual
```

The user controls most actions.

The Brain provides guidance but frequently asks for approval.

---

## Semi-Auto

```text
semi_auto
```

The Brain can:

- distributing tasks;
- guiding agents;
- moving cards;
- request corrections.

Critical actions may require human approval.

---

## Autonomous

```text
autonomous
```

The Brain can:

- interpreting the goal;
- creating tasks;
- assigning agents;
- monitoring execution;
- requesting corrections;
- reorganizing priorities;
- reviewing results;
- completing tasks.

Always respecting capabilities and policies.

---

# Capabilities

Permissions will be implemented through capabilities.

Do not create separate flags for every feature.

Avoid:

```text
can_git
can_shell
can_issue
can_pr
can_mermaid
```

Prefer:

```go
type Capability string
```

Examples:

```text
filesystem.read
filesystem.write

shell.execute

git.read
git.commit
git.branch
git.merge
git.push
git.create_repository

github.issue.read
github.issue.write

github.pr.read
github.pr.create
github.pr.review
github.pr.merge

task.read
task.create
task.assign
task.review

diagram.mermaid
diagram.plantuml
diagram.drawio
```

---

# Capability Policies

Example:

```yaml
agent:

  name: Backend Go

  capabilities:

    - filesystem.read
    - filesystem.write
    - shell.go
    - git.read
    - git.commit
```

Another agent:

```yaml
agent:

  name: Git Manager

  capabilities:

    - git.read
    - git.branch
    - git.commit
    - git.push
    - github.issue.read
    - github.issue.write
    - github.pr.read
    - github.pr.create
```

---

# Default Permissions

A development agent may initially have:

```text
filesystem.read     true
filesystem.write    true
```

By default:

```text
shell.execute       false
github.issue.write  false
github.pr.create    false
git.push            false
git.merge           false
```

---

# Framework CLI

Agents can be granted controlled access to the CLI related to their work.

Example:

Backend NestJS:

```text
npm
npx
nest
node
```

Backend Go:

```text
go test
go build
go vet
go fmt
```

Angular:

```text
ng
npm
npx
```

Access may be granted without granting completely unrestricted shell access.

---

# Tool Agents

Some agents may have special roles.

Example:

```text
Environment Manager
```

Responsible for:

- installing tools;
- validating dependencies;
- configuring the environment;
- prepare the CLI;
- install SDKs;
- configuring the required tools.

---

# Git

Git access must be controlled by a capability.

Examples:

```text
git.read
git.branch
git.commit
git.push
git.merge
git.create_repository
```

An agent may have full access if the administrator chooses.

There may also be a specialized agent:

```text
Git Manager
```

Responsible exclusively for Git and GitHub.

---

# Agent Acting on the User’s Behalf

If authorized, an agent may interact with Git and other tools directly on the user’s behalf.

This must be explicitly configured.

This must never be the default behavior.

---

# GitHub

Possible capabilities:

```text
github.repository.read
github.repository.create

github.issue.read
github.issue.create
github.issue.update
github.issue.close

github.pr.read
github.pr.create
github.pr.review
github.pr.merge
```

---

# Pull Requests

The Brain may have the capability to:

- analyze PRs;
- review changes;
- comment on PRs;
- validate implementations;
- request corrections.

Merge may have a separate capability:

```text
github.pr.merge
```

---

# Failures

When an agent fails, responsibility returns to the Brain.

Workflow:

```text
Worker
   │
   │ failure
   ▼
Brain
   │
   ├── analyzes error
   ├── provides new instructions
   └── retries worker
```

Do not implement blind retries alone.

The Brain must interpret the problem.

---

# Brain Uncertainty

If the Brain does not know how to proceed:

```text
Brain
  ↓
Administrator Notification
```

The administrator can:

- reply;
- add context;
- change the task;
- change the agent;
- switch provider;
- take control.

---

# Agent Status

Possible states:

```text
ONLINE
IDLE
BUSY
WAITING
OFFLINE
STALE
ERROR
```

Status is **server-owned**. Clients authenticate; they never submit a status.
The server derives the value from the most recent authenticated session plus the
work the board shows for that Agent:

```text
no session ever                 IDLE
silent >= offline threshold     OFFLINE
silent >= stale threshold       STALE
assigned task in WORKING        BUSY
assigned task in REVIEW         WAITING
recent FAILED task              ERROR
fresh session without work      ONLINE
```

Liveness outranks work: an Agent that stops reporting becomes `STALE` and then
`OFFLINE` even while it still holds a `WORKING` task.

---

# Heartbeat

A heartbeat is authenticated activity, not a client-declared state.

```text
VS Code Agent
     │ authenticated MCP request (bearer token)
     ├── heartbeat
     ├── heartbeat
     └── heartbeat
```

- Any authenticated MCP request refreshes liveness for the verified Agent, and
  the `boaird.agent.heartbeat` tool reports it explicitly.
- A session is bound to the Agent **and** the credential that opened it. An
  Agent name is never an identity, and a heartbeat that names another Agent is
  rejected and recorded as a denied authorization.
- Silence for `BOAIRD_AGENT_STALE_AFTER` marks the Agent `STALE`; silence for
  `BOAIRD_AGENT_OFFLINE_AFTER` expires the session and marks it `OFFLINE`.
- Reconnecting with the same credential opens a new session for the same
  identity (`session.resumed`).
- Every observed transition is stored as an immutable status event and exposed
  at `GET /api/v1/agents/{id}/status-events`.

If heartbeats stop:

```text
ONLINE
  ↓
STALE
  ↓
OFFLINE
```

---

# External Agents

boAIrd must allow external agents to participate in a project.

Examples:

```text
Codex
Claude
VS Code Agent
Cursor
Custom Agent
Local CLI Agent
```

Preferred communication:

```text
MCP
```

---

# Pull Model

Initially, external agents can use the Pull model.

```text
Agent
  │
  ├── connect
  │
  ├── ask for task
  │
  ▼
boAIrd
  │
  ▼
Task
```

This makes it easier to integrate with IDEs that may connect and disconnect.

---

# Push Model

In the future:

```text
boAIrd
  │
  └── sends task
          │
          ▼
       Agent
```

---

# MCP

MCP is an authenticated integration endpoint at `/mcp`; requests fail until an
active token is issued for an MCP Agent. Each token maps to one Agent and the
MCP session remains bound to that identity. Administrators create tokens with
`POST /api/v1/mcp/tokens`, rotate them with `POST /api/v1/mcp/tokens/{id}/rotate`,
and revoke them with `DELETE /api/v1/mcp/tokens/{id}`. Each token expires within
365 days. The plaintext is returned once; SQLite stores only a SHA-256 verifier.
Revocation blocks the next MCP HTTP request, including on an existing session.
The server exposes project, Agent, task, conversation, context, Brain-request,
and run tools listed below; every operation applies the Agent's explicit
capabilities and project or assignment scope.

Architecture:

```text
                  boAIrd Core
                      │
         ┌────────────┼────────────┐
         │            │            │
        MCP          REST         SSE
         │            │            │
       Agents      Dashboard      Events
```

---

# MCP Tools

Initial examples:

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

boaird.extension.execute (for active project-scoped extensions)

boaird.shell.execute (only when explicitly enabled)
```

Tool inputs and required capabilities:

| Tool | Inputs | Capability and scope |
| --- | --- | --- |
| `boaird.project.get` | `project_id` | `project.read`; persisted project membership or the authenticated Agent's active Brain relationship for that project |
| `boaird.agent.list`, `.get`, `.status` | optional `agent_id` for get/status | `agent.read`; returns only the authenticated Agent |
| `boaird.task.list` | optional `project_id`, `status`, `limit` (1–100), `offset` (0–10,000) | `task.read`; returns all tasks for projects in persisted membership or the authenticated Agent's active Brain scope, and otherwise only tasks assigned to that Agent |
| `boaird.task.get` | `task_id` | `task.read`; exact assignment or persisted/active-Brain project scope |
| `boaird.task.assign` | `task_id`, `agent_id`, `expected_version`, `reason` | `task.assign`; active Brain project scope or authorized Agent project scope; assignee must have project scope or be the caller; stale versions fail with a conflict |
| `boaird.task.claim` | `task_id`, `reason` | Worker self-claim only: `task.assign` + `task.transition` for an unassigned READY task in a project where the Agent has persisted membership; `task.transition` for a task already assigned to that Agent; active Brains must delegate with `task.assign` |
| `boaird.task.update` | `task_id`, `status` (`WORKING`, `REVIEW`, `HUMAN_REVIEW`, `BLOCKED`), `reason` | Worker: `task.transition` and exact assignment; active Brain: matching Brain capability, project scope, autonomy policy, and task workflow; `HUMAN_REVIEW` is allowed only when project policy requires it |
| `boaird.task.complete` | `task_id`, `reason` | Worker: `task.complete` and exact assignment; active Brain: `task.complete`, project scope, autonomy policy, domain workflow, and Human Review policy |
| `boaird.task.fail` | `task_id`, `reason` | Worker: `task.transition` and exact assignment; active Brain: `task.review`, project scope, autonomy policy, and task workflow |
| `boaird.conversation.read` | `task_id`, optional `before`, `limit` (1–100; default 50) | Workers require `task.read` and `conversation.read` with exact assignment; the active Brain requires both capabilities and scope to its own project |
| `boaird.conversation.send` | `task_id`, `content`, optional `content_type` | Workers require `conversation.write` and exact assignment; the active Brain requires `task.read`, `conversation.write`, and scope to its own project; persisted sender identity is `BRAIN` |
| `boaird.context.get` | `task_id` | Workers require `task.read` and `conversation.read` with exact assignment; the active Brain requires both capabilities and scope to its own project; at most 20 messages; project metadata is included only with effective project scope and `project.read` |
| `boaird.brain.request` | `project_id`, `objective`, optional `max_tokens` (1–4096; default 2048) | `brain.execute`; API-backed active Brain only. An MCP Brain uses its authenticated MCP tools directly |
| `boaird.run.get` | `run_id` | `run.read`; persisted project membership or the authenticated Agent's active Brain relationship for the run's project; run ownership alone does not grant access |
| `boaird.extension.execute` | `project_id`, `tool_name`, object-valued `input`, optional `task_id` and `run_id` | The activated extension's declared capability; active extension status and project membership or active-Brain scope are checked before the adapter runs; supplied task and Run references must match the same project |
| `boaird.shell.execute` | `project_id`, `executable`, optional structured `arguments`, optional `working_directory` | `shell.execute` and persisted project membership or active-Brain scope; the executable must be in the administrator allowlist |

An active Brain's scope is derived from the Brain-to-project relationship and is
limited to that one project; it does not create duplicate Agent project
membership. Every project-scoped operation still requires its named
capability. Assignment grants access only to that exact task and does not grant
project metadata or project-wide task visibility.

An MCP Worker may self-assign an unassigned READY task only when it has persisted
membership in that project and the explicit `task.assign` capability. The claim
is recorded through the task service before the task transitions to WORKING;
concurrent claims fail through task version checks. Active MCP Brains use
`boaird.task.assign` with the observed task version to delegate work, then use
Brain task tools under the same capability, autonomy, workflow, and Human Review
checks as other Brain execution paths. Brains cannot use the Worker self-claim
flow. Agents cannot claim tasks assigned to another Agent or tasks in any other
status.

Missing or invalid bearer tokens receive HTTP 401. A service-level denied or
invalid operation returns an MCP tool error with a generic message; malformed
protocol requests use the SDK protocol errors. Internal service errors are not
returned to the client. The MCP client initializes a stateful
session, then lists and calls tools on that session. The SDK binds that session
to the Agent authenticated at initialization; switching Agent tokens is
rejected. Sessions expire after 30 minutes idle.

New tokens should be issued through the authenticated administrator API and
stored in the MCP client's secret store. For migration only,
`BOAIRD_MCP_AGENT_TOKENS` is imported on first startup against a database, and
each imported token expires after 90 days. Remove this environment variable
after initial startup; later configuration changes do not create or restore
tokens.

```sh
curl --request POST http://127.0.0.1:3110/api/v1/mcp/tokens \
  --header "Authorization: Bearer ${BOAIRD_LOCAL_AUTH_TOKEN}" \
  --header 'Content-Type: application/json' \
  --data '{"agent_id":"agent-id","expires_in_seconds":7776000}'
```

Token lifecycle operations are recorded in immutable audit events without
token material. MCP sessions expire after 30 minutes of inactivity.
`boaird.task.claim` atomically assigns and starts an unassigned READY task only
when the authenticated Agent has persisted project membership and both
`task.assign` and `task.transition`. The task version observed by the MCP
handler is passed through the service compare-and-swap, so concurrent claims
cannot overwrite an assignment or both succeed. A task already assigned to the
Agent requires `task.transition`; claims of another Agent's task are denied.

---

# Providers

The initial providers should include:

```text
OpenAI
Anthropic
DeepSeek
Google Gemini
```

Support should also include:

```text
OpenAI-compatible API
```

---

# Provider Interface

```go
type Provider interface {

    Chat(
        ctx context.Context,
        request ChatRequest,
    ) (*ChatResponse, error)

    Models(
        ctx context.Context,
    ) ([]Model, error)
}
```

---

# Provider Structure

```text
internal/provider/

├── provider.go
├── registry.go
│
├── openai/
│   └── provider.go
│
├── anthropic/
│   └── provider.go
│
├── deepseek/
│   └── provider.go
│
├── gemini/
│   └── provider.go
│
└── openai_compatible/
    └── provider.go
```

---

# Credential Architecture

Credentials are stored independently from agents; an Agent references a credential ID and never receives its plaintext secret.

Relationship:

```text
Provider
   │
Credential
   │
Model
   │
Agent
```

This allows:

```text
OpenAI

├── Personal
├── Company
└── Backup
```

At execution time, provider resolution checks the credential ID against the
server-derived Agent and project. The Agent must be linked to the credential,
the provider must match, and the Agent must have administrator-granted project
membership or be that project's active Brain. Revoked or missing scope is
denied before decryption; scope changes are recorded in audit history.

---

# Secrets

API keys must never be stored in plaintext.

Initially:

```text
encrypted secret
+
master key
```

Example:

```bash
BOAIRD_MASTER_KEY=...
```

---

# Project

A project represents an isolated unit of work.

```text
Project
 │
 ├── Brain
 ├── Agents
 ├── Board
 ├── Conversations
 ├── Context
 ├── Memory
 ├── Tools
 ├── Policies
 ├── Runs
 └── Logs
```

---

# Projects

The dashboard must allow users to:

```text
Create Project
Edit Project
Archive Project
Open Project
Configure Brain
Configure Agents
Configure Capabilities
Configure Autonomy
Configure Repository
```

---

# Global Agents

Agents may be reusable in the future.

Structure:

```text
Global Agent Template
          │
          ▼
     Project Agent
```

Example:

```text
Backend Go Senior
```

It can be used in:

```text
Cryptox
boAIrd
LinkNauta
Network Core
```

Each project can override settings.

---

# Context

The context used by agents will combine three elements:

```text
Brain
+
Agent
+
boAIrd Context Engine
```

---

# Context Engine

The worker orchestrator builds a bounded, project/task-scoped JSON context for each API Agent execution. Entries keep their source and trust label; the serialized notice treats entry content as data that cannot grant authority or override policy. The dispatch outcome reports selected and omitted entries, including budget and unavailable-source reasons. Limits default to 16 KiB and 4096 estimated tokens and can be configured with `BOAIRD_CONTEXT_MAX_BYTES` and `BOAIRD_CONTEXT_MAX_TOKENS`.

Currently available inputs are project and task metadata, Agent/Brain instructions, authorized task conversation, same-task/same-Agent Runs, and bounded project memory. Files and repository history remain unavailable until dedicated scoped readers exist; the engine reports those omissions and does not access workspace paths or repository URLs. Work without Human Review is summarized after Brain approval; when Human Review is enabled, memory is recorded only after a human moves the task from `HUMAN_REVIEW` to `DONE`. Rejected or cancelled work does not become durable memory. Summaries are sanitized, versioned, and retain source provenance; selected memory is serialized as untrusted context data.

The context model includes these planned sources; those without a scoped reader are reported unavailable:

- files;
- documentation;
- tasks;
- history;
- decisions;
- conversations;
- commits;
- issues;
- PRs;
- logs;
- project information.

---

# Memory

The Brain should primarily optimize memory.

Objective:

```text
Less Context
↓
Less Tokens
↓
Lower Cost
```

The Brain can generate progressive summaries.

---

# Memory Architecture

```text
Raw History
    │
    ▼
Brain Summarization
    │
    ▼
Project Memory
```

Possible categories:

```text
Project Decisions
Architecture Decisions
Important Context
Task History
Agent Notes
Known Problems
Current State
```

---

# Raw Conversation

Conversations can remain stored for auditing.

However, new calls do not need the entire history.

```text
Raw Conversation
        │
        ├── available for audit
        │
        └── summarized for execution
```

---

# Dashboard

Frontend:

```text
Angular
Angular Material
SCSS
```

No additional UI framework initially.

---

# Dashboard Sections

Initial structure:

```text
Overview

Projects

Project
├── Overview
├── Kanban
├── Brain
├── Agents
├── Conversations
├── Runs
├── Logs
└── Settings

Providers

Agents

Logs

Settings
```

---

# Overview

The overview can show:

```text
Projects
Active Agents
Connected Agents
Running Tasks
Waiting Review
Human Review
Errors
Recent Activity
```

---

# Project Dashboard

Example:

```text
Cryptox

Brain:
Codex

Mode:
Autonomous

Agents:
5 online
2 working
3 idle

Tasks:
12 backlog
3 working
2 review
18 done
```

---

# Kanban UI

Example:

```text
BACKLOG      READY       WORKING       REVIEW       DONE

#120         #121        #123          #119         #110

Protocol     Tests       Backend       Security     README
```

---

# Task Modal

When clicking a card:

```text
Task #123

Title
Description

Assigned Agent

Status

Priority

Created By

Context

Conversation

Execution History

Files Changed

Logs

Reviews

Human Review
```

---

# Agent UI

Each agent will have:

```text
Avatar
Name
Role
Status
Runtime
Provider
Model
Capabilities
Tools
Instructions
Projects
Current Task
Conversation
Logs
```

---

# Agent Avatar

The user can choose:

- upload;
- icon;
- image;
- generate an avatar later.

---

# Languages

The interface must support internationalization.

Initially:

```text
pt-BR
en-US
```

Structure prepared for other languages.

---

# Agent Language

Each agent may have a preferred language.

Example:

```yaml
languages:
  preferred: en-US

  accepted:
    - en-US
    - pt-BR
```

---

# Tool System

Tools must be decoupled from agents.

Conceptual Structure:

```text
Agent
  │
Capabilities
  │
Tools
```

---

# Future Specialized Tools

Examples:

```text
Mermaid
PlantUML
draw.io
Browser
Database
Docker
Kubernetes
GitHub
GitLab
Jira
Terminal
Filesystem
```

---

# Software Engineer Agent

Future example:

```text
Software Engineer

Tools:

├── PlantUML
├── Mermaid
├── draw.io
├── Git
├── Filesystem
└── Shell
```

---

# Runs

Every execution must create a Run.

```text
Run
 │
 ├── Agent
 ├── Task
 ├── Provider
 ├── Model
 ├── Input
 ├── Output
 ├── Tools
 ├── Duration
 ├── Result
 └── Error
```

---

# Run Example

```text
Run ID:
run_01...

Project:
boAIrd

Task:
Implement Agent API

Agent:
Backend Go

Provider:
DeepSeek

Model:
deepseek-chat

Status:
SUCCESS

Duration:
12.4s
```

---

# Logs

Logs will be essential for auditing.

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

# Storage

For the MVP:

```text
SQLite
```

Motivos:

- simple;
- no additional infrastructure;
- ideal for local-first use;
- easy to back up;
- portable.

---

# Optional PostgreSQL Storage

```text
SQLite: local default
PostgreSQL: optional backend
```

PostgreSQL is implemented as an optional backend. Remote PostgreSQL targets,
including every configured fallback host, require `sslmode=verify-full` so the
certificate chain and server hostname are verified. Protect the connection URL
as a deployment secret and restrict database network access. The service uses
the same database credentials for normal operations and startup migrations, so
that role must currently have the required migration privileges. PostgreSQL-native backups must be encrypted and access-controlled; the database operator remains trusted with stored project data and audit history.

---

# Queue

For the MVP:

```text
Go channels
+
internal scheduler
```

Do not use initially:

```text
Kafka
RabbitMQ
Redis Streams
NATS
```

---

# Future Queue

When needed:

```text
NATS
Valkey
Redis Streams
RabbitMQ
```

---

# Backend

Primary backend:

```text
Go
```

Responsibilities:

```text
MCP
REST API
Agent Runtime
Provider Integration
Orchestrator
Task Engine
Context Engine
Memory
Security
Storage
Events
```

---

# Frontend

```text
Angular
Angular Material
SCSS
```

---

# Real-Time

The dashboard can initially use:

```text
SSE
```

for:

```text
Agent status
Task updates
Conversation
Runs
Logs
```

WebSocket can be added when persistent two-way communication is needed.

---

# REST API

Initial structure:

```text
/api/v1
```

Examples:

```text
GET    /api/v1/projects
POST   /api/v1/projects

GET    /api/v1/projects/:id

GET    /api/v1/agents
POST   /api/v1/agents

GET    /api/v1/tasks
POST   /api/v1/tasks

GET    /api/v1/providers

GET    /api/v1/runs

GET    /api/v1/logs
```

---

# Healthcheck

This liveness check reports whether the HTTP service is running; it does not query the configured database.

```text
GET /health
```

Response:

```json
{
  "status": "ok"
}
```

---

# Service Status

```text
GET /api/v1/status
```

When the service and configured database are available, the endpoint returns `200 OK`:

```json
{
  "status": "ok",
  "database": "connected"
}
```

When the configured database is unavailable, it returns `503 Service Unavailable`:

```json
{
  "status": "unavailable",
  "database": "disconnected"
}
```

---

# CLI

Binary:

```bash
boaird
```

---

# Serve

```bash
boaird serve
```

The service reads these optional environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `BOAIRD_HOST` | `127.0.0.1` | HTTP listen address |
| `BOAIRD_PORT` | `3110` | HTTP listen port |
| `BOAIRD_DATABASE_DRIVER` | `sqlite` | Storage backend: `sqlite` or `postgres`. SQLite remains the default. |
| `BOAIRD_DATABASE_PATH` | `./data/boaird.db` | SQLite database file |
| `BOAIRD_DATABASE_URL` | Empty | PostgreSQL connection URL; required when the driver is `postgres`. Keep credentials in deployment secrets; every remote target and fallback requires `sslmode=verify-full`. |
| `BOAIRD_MASTER_KEY` | Required | 32-byte encryption key encoded as 64 hexadecimal characters; never store it in SQLite or commit it. |
| `BOAIRD_LOCAL_AUTH_TOKEN` | Required | 32-byte bearer token encoded as 64 hexadecimal characters; authenticates all versioned API routes except status and can be exchanged for a browser session. |
| `BOAIRD_MCP_AGENT_TOKENS` | Empty | Legacy one-time bootstrap only: imported from the environment on first database startup with a 90-day expiry. Manage tokens through the authenticated MCP token API afterward. |
| `BOAIRD_PROVIDER_ALLOWED_HOSTS` | Empty | Comma-separated exact hostnames or IPs approved for custom provider URLs. Built-in vendor defaults and explicit loopback development URLs remain available. |
| `BOAIRD_PROVIDER_ALLOWED_IP_RANGES` | Empty | Comma-separated CIDR ranges that custom provider hosts may resolve to when private or special-use networks are intentionally required. |
| `BOAIRD_AGENT_STALE_AFTER` | `90s` | Silence after which an Agent is reported `STALE`. Must be at least `1s` and shorter than the offline threshold. |
| `BOAIRD_AGENT_OFFLINE_AFTER` | `5m` | Silence after which an Agent session expires and the Agent is reported `OFFLINE`. Maximum `24h`. |
| `BOAIRD_AGENT_STATUS_INTERVAL` | `30s` | How often silent sessions are expired and status transitions are recorded. Between `1s` and `1h`. |
| `BOAIRD_ORCHESTRATOR_MAX_ATTEMPTS` | `3` | Maximum Runs the orchestrator creates for one task before leaving it BLOCKED and escalating to the administrator. |
| `BOAIRD_ORCHESTRATOR_TIMEOUT` | `5m` | Maximum duration of one worker execution. Timeouts are recorded as cancelled Runs and handled through the bounded retry flow. |
| `BOAIRD_CONTEXT_MAX_BYTES` | `16384` | Maximum serialized worker context size in bytes; valid range is 512 bytes to 1 MiB. |
| `BOAIRD_CONTEXT_MAX_TOKENS` | `4096` | Maximum estimated worker context tokens; valid range is 128 to 100,000. |
| `BOAIRD_SHELL_ENABLED` | `false` | Enables the controlled `boaird.shell.execute` MCP tool. Requires a non-empty executable allowlist. |
| `BOAIRD_SHELL_ALLOWED_EXECUTABLES` | Empty | Comma-separated executable names or paths trusted by the administrator. The tool passes structured arguments directly and does not provide an OS filesystem sandbox. |

Generate each secret independently with `openssl rand -hex 32` and provide it through the deployment environment. Losing the master key makes stored provider credentials unrecoverable. The local auth token identifies the single `local-administrator` principal; do not reuse the master key as the token. Credential API responses contain metadata only; the secret is accepted only when creating a credential.

SQLite is the default backend. To select PostgreSQL, set `BOAIRD_DATABASE_DRIVER=postgres` and `BOAIRD_DATABASE_URL` to a `postgres://` or `postgresql://` URL containing a host and database name. Validation uses pgx's effective parsed connection settings, including hosts supplied through `host=` and fallback hosts. Every effective remote host requires `sslmode=verify-full`, which verifies both the trusted certificate chain and the hostname; loopback PostgreSQL may use local TLS settings. Selecting a different backend does not copy data from the existing database. PostgreSQL schema migrations are versioned independently from SQLite migrations and run at service startup. The service uses the same database credentials for application operations and startup migrations, so the configured role needs the required schema-migration privileges; separate runtime/migration credentials are not currently supported. Back up PostgreSQL with PostgreSQL-native tooling such as `pg_dump --format=custom`, and restore with `pg_restore`; provide credentials through a protected PostgreSQL service or password file rather than command-line arguments. Protect the URL as a deployment secret and restrict network ingress. The database operator can read or change application data and audit history. Do not copy a live database directory. Before upgrading, take and verify a backup and stop older application versions from writing during schema changes. The application applies each migration transactionally; PostgreSQL transactions use the configured server's standard `READ COMMITTED` isolation.

Custom HTTPS provider URLs are rejected unless their exact host is listed in `BOAIRD_PROVIDER_ALLOWED_HOSTS`. Every DNS answer is checked when a connection is opened and the selected validated IP is dialed directly. Loopback is allowed for explicit local development URLs. Private, link-local, metadata, and special-purpose IP ranges recognized by the endpoint policy are blocked unless they fall within `BOAIRD_PROVIDER_ALLOWED_IP_RANGES`. For IPv6, this includes the active IANA special-purpose prefixes and translation/transition ranges such as NAT64, Teredo, and 6to4; review the registry when updating this policy. Only add ranges needed by a trusted provider, because credentials are sent to the configured endpoint.

---

# Status

```bash
boaird status
```

---

# Projects

```bash
boaird projects list
```

---

# Agents

```bash
boaird agents list
```

The local API supports `POST /api/v1/agents`, `GET /api/v1/agents`, `GET /api/v1/agents/{id}`, and `PATCH /api/v1/agents/{id}`. API-backed agents identify a provider, model, and stored credential by ID; raw provider keys are not part of agent configuration. MCP agents authenticate with their issued, revocable MCP token. `role` is descriptive and does not grant capabilities. Agent status is managed by the runtime and is read-only through these endpoints.

All `/api/v1` operations except status require a currently registered user identity. The configured bearer token identifies the bootstrap administrator; an administrator can issue an opaque 30-minute browser session to a registered user. Each request reloads the user's current role, so deleted users and revoked sessions are rejected. Logout, user deletion, membership revocation, and server restart invalidate in-memory sessions. Deleted user IDs cannot be reused, preventing stale membership from transferring to a recreated identity. User, membership, and successful session issuance events are immutable and user or membership changes commit atomically with their audit record. Cookie-authenticated mutations require a trusted dashboard Origin. The API evaluates an explicit capability for each operation and checks `project_members` for project-scoped resources, including when the caller has the administrator role. Role grants operation authority but never bypasses project scope. The bootstrap administrator is seeded into every project; other administrators must be granted membership before using project-specific routes. Administrators can list all projects and Agents and read any Agent details/status history. Member list filtering happens before pagination, and member Agent details/status history are limited to Agents associated with a current member project. Members have read-only access to their projects. The global user-authority audit route is administrator-only; project audit remains project-scoped. Denials are stored in the immutable `authorization_denials` table. Projects can attach one active API or MCP Brain with `POST /api/v1/projects/{id}/brain`; the selected Agent must explicitly have the `brain.execute` capability. An MCP Brain must use its authenticated MCP session and existing capability-checked tools. Replacing an active Brain uses `PUT /api/v1/projects/{id}/brain` with a required audit reason; the server attributes the event to `local-administrator`. `POST /api/v1/projects/{id}/brain/runs` sends an objective to an API Brain's configured provider and persists a run; for an MCP Brain it returns HTTP 409 and does not create a run. API Brain output is proposed text and does not itself apply task or project mutations. Autonomy mode does not grant permissions, and project human review remains in force.

Runs can be listed with `GET /api/v1/projects/{id}/runs`, filtered by `task_id` or `agent_id`, or fetched individually with `GET /api/v1/runs/{id}`. Responses include immutable audit events for run creation and status transitions. Common credential patterns are redacted from run input, output, tool names, and errors before persistence and display.

Tasks can be created and listed with `POST` and `GET /api/v1/projects/{id}/tasks`, retrieved with `GET /api/v1/tasks/{id}`, assigned with `PUT /api/v1/tasks/{id}/assignment`, and moved through validated workflow transitions with `POST /api/v1/tasks/{id}/transitions`. Members can read tasks only in projects where they currently hold membership; task mutations require administrator capabilities. Mutations require a reason and create immutable audit events. Task responses include a monotonically increasing `version`; assignment and transition writes use optimistic concurrency, and return HTTP 409 if the task changed since it was read. Reload the task before retrying. Project human-review policy requires a human-controlled transition through `HUMAN_REVIEW` before `DONE`. The API checks the authenticated user's operation capability and project membership in Go and attributes mutations to the server-owned principal.

`POST /api/v1/projects/{id}/orchestration/dispatch` schedules READY tasks already assigned to an API Agent and creates one linked Run per attempt. The project Brain is verified through `Brain.AuthorizeAction` before any provider call: a Brain without `brain.execute` cannot dispatch, and the task capabilities used by each transition (typically `task.assign`, `task.review`, and `task.complete`) are enforced per transition, with missing grants skipped rather than bypassed. Failures return to the Brain for retry instructions and are bounded by `BOAIRD_ORCHESTRATOR_MAX_ATTEMPTS`; only worker executions count toward the limit, not the Brain analysis Runs linked to the same task. `POST /api/v1/tasks/{id}/review` applies `APPROVE` or `REWORK` as the Brain: approval completes the task only when autonomy and human-review policy allow it, otherwise it moves to `HUMAN_REVIEW`; rework records the instructions and executes one more bounded attempt. Externally hosted Agents are not dispatched because they claim work through MCP.

Each task has a persisted conversation. `GET` and `POST /api/v1/tasks/{id}/messages` require a registered user session and verify current project membership. The server derives human sender identity from the authenticated user; caller-supplied sender identity is rejected. New and existing projects receive membership for the bootstrap administrator. Message history is paginated with `limit` (default 50, maximum 100) and the `before` cursor returned as `next_before`. Agent messages are accepted only from the agent assigned to that task, and Brain messages only from the project's active Brain. Common secret patterns and sensitive metadata fields are redacted before storage and return.

## Dashboard foundation

The standalone Angular dashboard lives in [`web/`](web/README.md). It uses Angular Material, SCSS, strict TypeScript, feature routes, a server-owned state model, and a local API base URL of `http://localhost:3110/api/v1` during development. The login screen sends the administrator token only to exchange it for an HttpOnly browser session; later dashboard requests use the session cookie and never attach the root token. The project list opens a project scoped overview of its Brain, assigned agents, task status, blocked work, and recent runs. The Agents screen lists, creates, and edits agent configuration through the versioned API; it displays only stored credential metadata, treats status as read-only, and keeps capabilities explicit rather than deriving them from role labels. The Kanban view groups tasks by backend status, submits reasoned transitions to the API, and links to task history, conversations, and runs. Conversation history loads older pages through the API cursor. The Brain dashboard selects a project Brain, shows its recent proposals and blocked or human-review tasks, and requires an administrator session plus a reason to replace an active Brain. Run `npm install`, `npm start`, and `npm run build` from `web/`.

---

# Tasks

```bash
boaird tasks list
```

---

# Providers

```bash
boaird providers list
```

---

# Architecture

```text
                         Browser
                            │
                            ▼
                  ┌──────────────────┐
                  │ Angular Dashboard│
                  └────────┬─────────┘
                           │
                       REST / SSE
                           │
                           ▼
               ┌────────────────────────┐
               │        boAIrd          │
               │       Go Core          │
               └────────────┬───────────┘
                            │
      ┌─────────────────────┼──────────────────────┐
      │                     │                      │
      ▼                     ▼                      ▼
  MCP Server            REST API              Event Bus
      │
      ▼
 Agent Runtime
      │
 ┌────┼────────────┬─────────────┐
 │    │            │             │
 ▼    ▼            ▼             ▼
API   MCP         Local         Remote
 │
 ▼
Providers
 │
 ├── OpenAI
 ├── Claude
 ├── DeepSeek
 └── Gemini
```

---

# Internal Architecture

```text
                    Project
                       │
                       ▼
                     Brain
                       │
                       ▼
                 Orchestrator
                       │
          ┌────────────┼────────────┐
          │            │            │
          ▼            ▼            ▼
        Tasks        Context       Memory
          │
          ▼
        Agents
          │
          ▼
       Runtime
          │
     ┌────┼─────┐
     │    │     │
     ▼    ▼     ▼
    API  MCP  Local
```

---

# Recommended Project Structure

```text
boaird/
│
├── cmd/
│   └── boaird/
│       └── main.go
│
├── internal/
│
│   ├── project/
│   │   ├── project.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── policy.go
│   │
│   ├── brain/
│   │   ├── brain.go
│   │   ├── service.go
│   │   └── supervisor.go
│   │
│   ├── agent/
│   │   ├── agent.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   ├── status.go
│   │   └── capability.go
│   │
│   ├── runtime/
│   │   ├── runtime.go
│   │   ├── registry.go
│   │   │
│   │   ├── api/
│   │   ├── mcp/
│   │   ├── local/
│   │   └── remote/
│   │
│   ├── provider/
│   │   ├── provider.go
│   │   ├── registry.go
│   │   │
│   │   ├── openai/
│   │   │   └── provider.go
│   │   │
│   │   ├── anthropic/
│   │   │   └── provider.go
│   │   │
│   │   ├── deepseek/
│   │   │   └── provider.go
│   │   │
│   │   ├── gemini/
│   │   │   └── provider.go
│   │   │
│   │   └── openai_compatible/
│   │       └── provider.go
│   │
│   ├── credential/
│   │   ├── credential.go
│   │   ├── service.go
│   │   ├── encryption.go
│   │   └── repository.go
│   │
│   ├── capability/
│   │   ├── capability.go
│   │   ├── registry.go
│   │   └── policy.go
│   │
│   ├── task/
│   │   ├── task.go
│   │   ├── status.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── assignment.go
│   │
│   ├── board/
│   │   ├── board.go
│   │   └── service.go
│   │
│   ├── conversation/
│   │   ├── conversation.go
│   │   ├── message.go
│   │   ├── service.go
│   │   └── repository.go
│   │
│   ├── orchestrator/
│   │   ├── orchestrator.go
│   │   ├── scheduler.go
│   │   ├── dispatcher.go
│   │   └── recovery.go
│   │
│   ├── context/
│   │   ├── context.go
│   │   ├── engine.go
│   │   └── selector.go
│   │
│   ├── memory/
│   │   ├── memory.go
│   │   ├── service.go
│   │   └── summarizer.go
│   │
│   ├── tool/
│   │   ├── tool.go
│   │   ├── registry.go
│   │   │
│   │   ├── filesystem/
│   │   ├── shell/
│   │   ├── git/
│   │   ├── github/
│   │   └── diagram/
│   │
│   ├── mcp/
│   │   ├── server.go
│   │   ├── session.go
│   │   ├── tools.go
│   │   ├── resources.go
│   │   └── transport.go
│   │
│   ├── api/
│   │   ├── server.go
│   │   ├── router.go
│   │   │
│   │   ├── handler/
│   │   ├── middleware/
│   │   └── dto/
│   │
│   ├── events/
│   │   ├── event.go
│   │   ├── bus.go
│   │   └── subscriber.go
│   │
│   ├── run/
│   │   ├── run.go
│   │   ├── service.go
│   │   └── repository.go
│   │
│   ├── audit/
│   │   ├── audit.go
│   │   └── service.go
│   │
│   ├── notification/
│   │   ├── notification.go
│   │   └── service.go
│   │
│   ├── storage/
│   │   ├── storage.go
│   │   │
│   │   ├── sqlite/
│   │   │   ├── database.go
│   │   │   └── migrations.go
│   │   │
│   │   └── postgres/
│   │
│   ├── config/
│   │   ├── config.go
│   │   └── loader.go
│   │
│   ├── security/
│   │   ├── crypto.go
│   │   ├── permissions.go
│   │   └── secrets.go
│   │
│   └── health/
│       └── health.go
│
├── pkg/
│   └── sdk/
│
├── web/
│   └── dashboard/
│
├── migrations/
│
├── configs/
│
├── data/
│
├── docs/
│   ├── adr/
│   ├── measurements/
│   ├── architecture/
│   ├── mcp/
│   ├── providers/
│   └── agents/
│
├── scripts/
│
├── tests/
│   ├── integration/
│   └── e2e/
│
├── docker/
│
├── .env.example
├── .gitignore
├── docker-compose.yml
├── Makefile
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```

---

# Angular Structure

Suggested Structure:

```text
web/dashboard/

src/
│
├── app/
│
│   ├── core/
│   │   ├── api/
│   │   ├── auth/
│   │   ├── events/
│   │   ├── services/
│   │   └── models/
│   │
│   ├── shared/
│   │   ├── components/
│   │   ├── directives/
│   │   ├── pipes/
│   │   └── material/
│   │
│   ├── features/
│   │
│   │   ├── overview/
│   │   ├── projects/
│   │   ├── project/
│   │   ├── kanban/
│   │   ├── tasks/
│   │   ├── brain/
│   │   ├── agents/
│   │   ├── conversations/
│   │   ├── providers/
│   │   ├── runs/
│   │   ├── logs/
│   │   └── settings/
│   │
│   ├── layout/
│   │   ├── sidebar/
│   │   ├── header/
│   │   └── shell/
│   │
│   ├── app.routes.ts
│   ├── app.config.ts
│   └── app.component.ts
│
├── assets/
│
├── styles/
│   ├── _variables.scss
│   ├── _theme.scss
│   └── styles.scss
│
└── environments/
```

---

# MVP

The first MVP must prove this workflow:

```text
1. User starts boAIrd

2. User opens the dashboard

3. User creates a project

4. User configures the Brain

5. The Brain can be:
   API
   MCP
   VS Code Agent
   Local Agent

6. User creates agents

7. User defines capabilities

8. User sets the autonomy level

9. User creates a goal or task

10. The Brain interprets it

11. The Brain creates and assigns tasks

12. The worker executes the task

13. The task moves to REVIEW

14. QA or the Brain reviews it

15. If needed:
    the worker receives guidance

16. If human_review is enabled:
    the administrator approves

17. The task moves to DONE

18. All history remains available
```

---

# MVP Stack

```text
Go
SQLite
Angular
Angular Material
SCSS

REST
SSE
MCP

OpenAI
Claude
DeepSeek
Gemini
```

---

# Do Not Use Initially

Avoid unnecessary complexity in the MVP:

```text
Kubernetes
Kafka
RabbitMQ
Redis Cluster
Vector Database
Microservices
Service Mesh
Mandatory local LLM
```

boAIrd will initially be a **modular monolith**.

---

# Modular Monolith

Structure:

```text
One Binary
   │
   ├── API
   ├── MCP
   ├── Brain
   ├── Agents
   ├── Tasks
   ├── Providers
   ├── Memory
   └── Storage
```

This keeps:

- development fast;
- deployment simple;
- infrastructure minimal;
- debugging easy.

---

# Bootstrap Roadmap

## Phase 0 — Project Bootstrap

Objective:

```text
boaird serve
```

Implement:

- Go module;
- CLI;
- config;
- logging;
- HTTP server;
- healthcheck;
- graceful shutdown;
- SQLite.

---

# Phase 1 — Projects

Implement:

- create project;
- list project;
- update project;
- delete/archive project;
- project settings;
- autonomy mode.

---

# Phase 2 — Providers

Implement:

```text
OpenAI
Anthropic
DeepSeek
Gemini
```

With:

- credentials;
- model configuration;
- provider registry;
- chat execution.

---

# Phase 3 — Agents

Implement:

- agent entity;
- avatar;
- role;
- description;
- capabilities;
- provider;
- model;
- runtime;
- status.

---

# Phase 4 — Brain

Implement the rule:

```text
Project requires Brain
```

The Brain can run through an API provider or as an authenticated external MCP
Agent. Local and remote runtime adapters remain future work.

---

# Phase 5 — Kanban

Implement:

```text
BACKLOG
READY
WORKING
REVIEW
HUMAN_REVIEW
BLOCKED
DONE
```

---

# Phase 6 — Conversations

Implement:

- task conversation;
- agent conversation;
- brain messages;
- human messages;
- history.

---

# Phase 7 — Orchestrator

Implement:

```text
Brain
↓
Task
↓
Agent
↓
Review
↓
Brain
```

---

# Phase 8 — MCP

Implement:

- MCP server;
- agent connection;
- heartbeat;
- task discovery;
- task claim;
- conversation;
- task completion.

---

# Phase 9 — Angular Dashboard

Implement:

```text
Overview
Projects
Project
Kanban
Brain
Agents
Conversations
Providers
Runs
Logs
Settings
```

---

# Phase 10 — Capabilities

Implement security policies.

```text
filesystem
shell
git
github
tasks
tools
```

---

# Phase 11 — Context

Implement the Context Engine.

Sources:

```text
Project
Task
Files
Memory
Conversation
Brain
Agent
```

---

# Phase 12 — Memory

Implement:

```text
Raw History
↓
Brain Summary
↓
Optimized Memory
```

---

# Phase 13 — Specialized Tools

Implement the plugin/tool system.

Examples:

```text
Mermaid
PlantUML
draw.io
GitHub
Filesystem
Shell
```

---

# Phase 14 — Remote Mode

Implement:

```text
Authentication
TLS
Remote MCP
Remote Dashboard
Multi-user
```

---

# Security Principles

## Deny by Default

Dangerous actions must be disabled by default.

```text
shell.execute       false
git.push            false
git.merge           false
github.pr.merge     false
```

---

## Least Privilege

Each agent must have only the capabilities it needs.

---

## Audit Everything

Important actions must be audited.

```text
Actor
Action
Resource
Timestamp
Result
```

---

## Human Override

The administrator can interrupt any workflow.

---

# Long-Term Vision

In the long term, boAIrd should let users build their own virtual team.

```text
                         Brain
                           │
           ┌───────────────┼────────────────┐
           │               │                │
       Architect        Developer          QA
           │               │                │
           ├───────────────┼────────────────┤
                           │
                       DevOps
                           │
                       Git Manager
```

Each component can use a different model.

```text
Brain:
Codex

Backend:
DeepSeek

Frontend:
Gemini

QA:
Claude

Git:
Local Agent
```

---

# boAIrd Is Not

boAIrd should not be just:

```text
LLM Chat UI
API Wrapper
Prompt Manager
MCP Proxy
Kanban Board
```

It combines these concepts to create an:

> **AI Agent Control Plane**

---

# Main Concept

```text
Models
   │
   ▼
Agents
   │
   ▼
Brain
   │
   ▼
Tasks
   │
   ▼
Kanban
   │
   ▼
Tools
   │
   ▼
Projects
   │
   ▼
Human
```

---

# Philosophy

The human defines:

```text
goal
permissions
autonomy
Brain
agents
tools
limits
```

The Brain decides:

```text
how to organize
who executes
when to review
when to correct
when to escalate
```

Agents execute.

boAIrd controls.

---

# Current Status

```text
Project:
boAIrd

Status:
Experimental

Stage:
Architecture / Bootstrap

Version:
0.0.1-dev
```

---

# Initial Development Goal

First technical goal:

```text
go run ./cmd/boaird serve
```

Expected result:

```text
boAIrd starting...

API:
http://localhost:3110

Health:
http://localhost:3110/health

Storage:
SQLite

Status:
READY
```

---

# First Milestone

```text
Create Project
      ↓
Configure Brain
      ↓
Configure Worker
      ↓
Create Task
      ↓
Brain Assigns
      ↓
Worker Executes
      ↓
Brain Reviews
      ↓
DONE
```

When this workflow works end to end through the dashboard, we will have the first **functional boAIrd MVP**.

---

# License

boAIrd is distributed under the MIT License. See [LICENSE](LICENSE) for the full terms.

---

# Name

```text
boAIrd
```

The name represents the combination of:

```text
Board
+
AI
```

The board is where work happens.

AI coordinates and executes the work.

**boAIrd** is the environment where humans and agents work together.
