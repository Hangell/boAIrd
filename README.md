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

---

# Heartbeat

External agents may send heartbeats.

Example:

```text
VS Code Agent
     │
     ├── heartbeat
     ├── heartbeat
     └── heartbeat
```

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
```

Tool inputs and required capabilities:

| Tool | Inputs | Capability and scope |
| --- | --- | --- |
| `boaird.project.get` | `project_id` | `project.read`; persisted project membership or the authenticated Agent's active Brain relationship for that project |
| `boaird.agent.list`, `.get`, `.status` | optional `agent_id` for get/status | `agent.read`; returns only the authenticated Agent |
| `boaird.task.list` | optional `project_id`, `status`, `limit` (1–100), `offset` (0–10,000) | `task.read`; returns all tasks for projects in persisted membership or the authenticated Agent's active Brain scope, and otherwise only tasks assigned to that Agent |
| `boaird.task.get` | `task_id` | `task.read`; exact assignment or persisted/active-Brain project scope |
| `boaird.task.claim` | `task_id`, `reason` | `task.transition`; starts an already assigned READY task |
| `boaird.task.update` | `task_id`, `status` (`WORKING`, `REVIEW`, `BLOCKED`), `reason` | `task.transition`; exact assignment |
| `boaird.task.complete` | `task_id`, `reason` | `task.complete`; exact assignment, domain workflow and Human Review policy |
| `boaird.task.fail` | `task_id`, `reason` | `task.transition`; exact assignment |
| `boaird.conversation.read` | `task_id`, optional `before`, `limit` (1–100; default 50) | `task.read` and `conversation.read`; exact assignment |
| `boaird.conversation.send` | `task_id`, `content`, optional `content_type` | `conversation.write`; exact assignment |
| `boaird.context.get` | `task_id` | `task.read` and `conversation.read`; exact assignment or persisted/active-Brain project scope, at most 20 messages; project metadata is included only with effective project scope and `project.read` |
| `boaird.brain.request` | `project_id`, `objective`, optional `max_tokens` (1–4096; default 2048) | `brain.execute`; authenticated Agent must be the project's active Brain |
| `boaird.run.get` | `run_id` | `run.read`; persisted project membership or the authenticated Agent's active Brain relationship for the run's project; run ownership alone does not grant access |

An active Brain's scope is derived from the Brain-to-project relationship and is
limited to that one project; it does not create duplicate Agent project
membership. Every project-scoped operation still requires its named
capability. Assignment grants access only to that exact task and does not grant
project metadata or project-wide task visibility.

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
`boaird.task.claim` starts work only on
a task already assigned to that Agent; assignment remains an administrator or
authorized Brain operation.

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

boAIrd can select:

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

# Future Storage

```text
SQLite
   ↓
PostgreSQL
```

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

This liveness check reports whether the HTTP service is running; it does not query SQLite.

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

When the service and SQLite are available, the endpoint returns `200 OK`:

```json
{
  "status": "ok",
  "database": "connected"
}
```

When SQLite is unavailable, it returns `503 Service Unavailable`:

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
| `BOAIRD_DATABASE_PATH` | `./data/boaird.db` | SQLite database file |
| `BOAIRD_MASTER_KEY` | Required | 32-byte encryption key encoded as 64 hexadecimal characters; never store it in SQLite or commit it. |
| `BOAIRD_LOCAL_AUTH_TOKEN` | Required | 32-byte bearer token encoded as 64 hexadecimal characters; authenticates all versioned API routes except status and can be exchanged for a browser session. |
| `BOAIRD_MCP_AGENT_TOKENS` | Empty | Legacy one-time bootstrap only: imported from the environment on first database startup with a 90-day expiry. Manage tokens through the authenticated MCP token API afterward. |
| `BOAIRD_PROVIDER_ALLOWED_HOSTS` | Empty | Comma-separated exact hostnames or IPs approved for custom provider URLs. Built-in vendor defaults and explicit loopback development URLs remain available. |
| `BOAIRD_PROVIDER_ALLOWED_IP_RANGES` | Empty | Comma-separated CIDR ranges that custom provider hosts may resolve to when private or special-use networks are intentionally required. |

Generate each secret independently with `openssl rand -hex 32` and provide it through the deployment environment. Losing the master key makes stored provider credentials unrecoverable. The local auth token identifies the single `local-administrator` principal; do not reuse the master key as the token. Credential API responses contain metadata only; the secret is accepted only when creating a credential.

Custom HTTPS provider URLs are rejected unless their exact host is listed in `BOAIRD_PROVIDER_ALLOWED_HOSTS`. Every DNS answer is checked when a connection is opened and the selected validated IP is dialed directly. Loopback is allowed for explicit local development URLs. Private, link-local, metadata, and listed special-purpose IP ranges are blocked unless they fall within `BOAIRD_PROVIDER_ALLOWED_IP_RANGES`; only add ranges needed by a trusted provider, because credentials are sent to the configured endpoint.

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

The local API supports `POST /api/v1/agents`, `GET /api/v1/agents`, `GET /api/v1/agents/{id}`, and `PATCH /api/v1/agents/{id}`. API-backed agents identify a provider, model, and stored credential by ID; raw provider keys are not part of agent configuration. `role` is descriptive and does not grant capabilities. Agent status is managed by the runtime and is read-only through these endpoints.

All `/api/v1` operations except status require the local administrator bearer token or a valid browser session. `POST /api/v1/session` exchanges the bearer token for a 30-minute HttpOnly, SameSite=Strict cookie containing a random opaque session ID; the server stores session state in memory and revokes it at logout or restart. The dashboard keeps the root token only in the login form until the exchange completes. Cookie-authenticated mutations require a trusted dashboard Origin. The API evaluates an explicit capability for each operation and verifies `project_members` for project-scoped resources. Denials are stored in the immutable `authorization_denials` table. Projects can attach one active API Brain with `POST /api/v1/projects/{id}/brain`; the selected agent must explicitly have the `brain.execute` capability. Replacing an active Brain uses `PUT /api/v1/projects/{id}/brain` with a required audit reason; the server attributes the event to `local-administrator`. `POST /api/v1/projects/{id}/brain/runs` sends an objective to the configured provider and persists a run. The Brain returns a proposed plan as text; this runtime does not apply task or project mutations. Autonomy mode does not grant permissions, and project human review remains in force.

Runs can be listed with `GET /api/v1/projects/{id}/runs`, filtered by `task_id` or `agent_id`, or fetched individually with `GET /api/v1/runs/{id}`. Responses include immutable audit events for run creation and status transitions. Common credential patterns are redacted from run input, output, tool names, and errors before persistence and display.

Tasks can be created and listed with `POST` and `GET /api/v1/projects/{id}/tasks`, retrieved with `GET /api/v1/tasks/{id}`, assigned with `PUT /api/v1/tasks/{id}/assignment`, and moved through validated workflow transitions with `POST /api/v1/tasks/{id}/transitions`. Mutations require a reason and create immutable audit events. Task responses include a monotonically increasing `version`; assignment and transition writes use optimistic concurrency, and return HTTP 409 if the task changed since it was read. Reload the task before retrying. Project human-review policy requires a human-controlled transition through `HUMAN_REVIEW` before `DONE`. The API requires the local administrator token/session, checks the operation capability and project membership in Go, and attributes mutations to the server-owned administrator principal.

Each task has a persisted conversation. `GET` and `POST /api/v1/tasks/{id}/messages` require the authenticated local administrator session and verify project membership. The server derives human sender identity as `local-administrator`; caller-supplied sender identity is rejected. New and existing projects receive membership for this single local principal. Message history is paginated with `limit` (default 50, maximum 100) and the `before` cursor returned as `next_before`. Agent messages are accepted only from the agent assigned to that task, and Brain messages only from the project's active Brain. Common secret patterns and sensitive metadata fields are redacted before storage and return. This initial identity model has one local administrator; multi-user accounts and membership management are future work.

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

Initially, the Brain runs through an API.

Later:

```text
MCP Brain
External Brain
```

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
