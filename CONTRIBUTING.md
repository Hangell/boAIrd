# Contributing to boAIrd

Thank you for your interest in contributing to **boAIrd**.

boAIrd is a local-first AI Agent Control Plane designed to coordinate Brain agents, specialized AI workers, external agents, MCP clients, tools, projects, conversations, and Kanban-based workflows.

Contributions are welcome in many forms, including:

- source code;
- bug fixes;
- documentation;
- tests;
- security improvements;
- provider integrations;
- MCP improvements;
- Angular dashboard improvements;
- translations;
- architecture discussions;
- performance improvements;
- developer tooling;
- examples;
- and issue reports.

This document explains how to contribute to the project and the standards expected from contributions.

---

# Table of Contents

- Project Philosophy
- Before Contributing
- Types of Contributions
- Development Stack
- Repository Structure
- Development Environment
- Branching Strategy
- Issues
- Pull Requests
- Commit Messages
- Go Contribution Guidelines
- Angular Contribution Guidelines
- API Guidelines
- MCP Guidelines
- Agent Runtime Guidelines
- Capability and Security Guidelines
- Database Guidelines
- Testing
- Documentation
- Logging and Observability
- AI-Assisted Contributions
- Generated Code
- Security Vulnerabilities
- Breaking Changes
- Dependency Policy
- Code Review
- Contributor Conduct
- Licensing
- Contributor Checklist

---

# Project Philosophy

boAIrd follows several core principles.

Every contribution should respect them.

## Local First

boAIrd must remain easy to run locally.

A contributor should avoid introducing infrastructure dependencies unless they are clearly necessary.

The default development experience should remain close to:

```bash
boaird serve
```

with:

```text
Go
SQLite
Angular
MCP
REST
SSE
```

---

## Modular Monolith First

boAIrd is initially designed as a modular monolith.

Do not introduce microservices simply because a feature could theoretically be separated.

Prefer:

```text
one binary
+
well-defined internal modules
```

over:

```text
many services
+
network complexity
+
distributed infrastructure
```

Distributed components should only be introduced when the project actually requires them.

---

## Model Agnostic

Core boAIrd logic must not depend on a specific AI model.

Avoid code such as:

```go
if agent.Model == "some-specific-model" {
    // special core behavior
}
```

Provider-specific logic belongs inside provider or runtime implementations.

---

## Provider Agnostic

Agents should communicate with providers through common abstractions.

The core should not know whether a request is being handled by:

- OpenAI;
- Anthropic;
- DeepSeek;
- Gemini;
- an OpenAI-compatible provider;
- a local model;
- an MCP client;
- an IDE agent;
- or another future runtime.

---

## Brain Is Mandatory

Every executable project must have one active Brain.

The Brain is responsible for orchestration, validation, guidance, and escalation.

Contributions must not bypass the Brain architecture when implementing features that depend on project coordination.

---

## Human Authority

The administrator always has higher authority than the Brain.

The intended hierarchy is:

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
      ↓
Tools
```

No contribution should allow an agent to escalate above the administrator or project security policy.

---

## Autonomy Is Not Permission

This is a fundamental boAIrd rule:

> Autonomy defines what an agent may decide.
> Capabilities define what an agent may execute.

An autonomous agent does not automatically receive additional permissions.

---

## Deny by Default

Sensitive features should be disabled by default.

Examples include:

```text
shell.execute

git.push

git.merge

github.pr.merge

filesystem.delete

repository.create
```

---

## Audit Important Actions

Sensitive or state-changing actions should be observable and auditable.

If a contribution adds a new sensitive operation, it should normally include:

- authorization;
- capability definition;
- audit event;
- error behavior;
- tests.

---

# Before Contributing

Before starting significant work:

1. Search existing Issues.
2. Search existing Pull Requests.
3. Review the current ROADMAP.
4. Review SECURITY.md.
5. Review project architecture documentation.
6. Open an Issue for significant changes.

Small fixes such as:

- spelling;
- minor documentation corrections;
- small test improvements;
- obvious bug fixes;

may usually be submitted directly.

Large architectural work should be discussed before implementation.

---

# Types of Contributions

## Bug Reports

A useful bug report should include:

```text
boAIrd version

operating system

Go version

Node version

Angular version

steps to reproduce

expected behavior

actual behavior

logs

relevant configuration
```

Never include real:

- API keys;
- access tokens;
- passwords;
- private repository credentials;
- private keys.

---

## Feature Requests

Feature requests should explain:

```text
Problem

Use Case

Proposed Behavior

Alternatives

Security Impact

Architecture Impact
```

Avoid requests that only describe an implementation without explaining the problem being solved.

---

## Documentation

Documentation contributions are welcome.

Examples:

```text
README improvements

architecture documentation

provider setup guides

MCP integration guides

agent examples

security documentation

translations
```

---

## Provider Integrations

New provider integrations are welcome if they follow the shared provider abstraction.

Examples may include:

```text
Qwen

Mistral

Groq

OpenRouter

local OpenAI-compatible servers
```

A provider implementation should not require changes to the core orchestration model unless absolutely necessary.

---

## Agent Runtimes

New runtimes may include:

```text
API Runtime

MCP Runtime

Local Runtime

Remote Runtime

IDE Runtime
```

All runtimes should implement the common runtime abstraction.

---

## Tools

New tools may include:

```text
GitLab

Jira

Linear

Docker

Kubernetes

PlantUML

Mermaid

draw.io

database tools
```

Every tool must define its security requirements explicitly.

---

# Development Stack

## Backend

```text
Go
```

---

## Frontend

```text
Angular
Angular Material
SCSS
```

Avoid introducing additional UI frameworks without architectural justification.

---

## Database

Default local storage:

```text
SQLite
```

Future deployments may support PostgreSQL.

Storage implementations should remain abstracted behind repository interfaces where appropriate.

---

## Communication

The project may use:

```text
REST

SSE

MCP
```

WebSocket should only be introduced where persistent bidirectional communication clearly requires it.

---

# Repository Structure

The repository generally follows this structure:

```text
boaird/

├── cmd/
│   └── boaird/
│
├── internal/
│   ├── project/
│   ├── brain/
│   ├── agent/
│   ├── runtime/
│   ├── provider/
│   ├── credential/
│   ├── capability/
│   ├── task/
│   ├── board/
│   ├── conversation/
│   ├── orchestrator/
│   ├── context/
│   ├── memory/
│   ├── tool/
│   ├── mcp/
│   ├── api/
│   ├── events/
│   ├── run/
│   ├── audit/
│   ├── notification/
│   ├── storage/
│   ├── config/
│   ├── security/
│   └── health/
│
├── pkg/
│
├── web/
│   └── dashboard/
│
├── migrations/
├── configs/
├── docs/
├── tests/
└── scripts/
```

Do not create packages only for organizational appearance.

A package should represent a meaningful domain or technical boundary.

---

# Development Environment

## Requirements

Recommended minimum environment:

```text
Go 1.25+

Node.js LTS or current supported project version

Angular CLI compatible with the current dashboard version

Git

SQLite
```

Always check the repository configuration for the exact supported versions.

---

# Clone the Repository

```bash
git clone <repository-url>
cd boaird
```

---

# Go Dependencies

```bash
go mod download
```

---

# Angular Dependencies

```bash
cd web/dashboard
npm install
```

Prefer:

```bash
npm ci
```

in CI environments.

---

# Running the Backend

```bash
go run ./cmd/boaird serve
```

Default development endpoint:

```text
http://localhost:3110
```

The backend also accepts `BOAIRD_HOST`, `BOAIRD_PORT`, and `BOAIRD_DATABASE_PATH` to override the listen address, listen port, and SQLite file path. Defaults are `127.0.0.1`, `3110`, and `./data/boaird.db`.

---

# Health Check

This liveness check reports whether the HTTP service is running; it does not query SQLite.

```text
GET /health
```

Expected response:

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

The endpoint returns `200 OK` when SQLite is connected:

```json
{
  "status": "ok",
  "database": "connected"
}
```

It returns `503 Service Unavailable` when SQLite cannot be reached:

```json
{
  "status": "unavailable",
  "database": "disconnected"
}
```

---

# Running the Dashboard

From:

```text
web/dashboard
```

run:

```bash
ng serve
```

or the command defined in the project's package scripts.

---

# Branching Strategy

Use short-lived branches.

Recommended formats:

```text
feat/agent-runtime

feat/mcp-heartbeat

fix/task-status-transition

fix/provider-timeout

refactor/capability-engine

docs/security-model

test/orchestrator-recovery
```

Avoid branches such as:

```text
my-branch

changes

test123

new
```

---

# Main Branch

The main branch should always represent the latest stable development state.

Direct pushes to protected branches should be avoided.

Prefer:

```text
branch
↓
commit
↓
push
↓
Pull Request
↓
review
↓
merge
```

---

# Issues

Issues are the preferred place to describe work before implementation.

A good Issue should answer:

```text
What is the problem?

Why does it matter?

What behavior is expected?

Which component is affected?

Are there security implications?

Are there breaking changes?
```

---

# Issue Labels

Recommended categories include:

```text
bug

feature

security

architecture

backend

frontend

mcp

provider

agent

brain

kanban

tool

documentation

testing

good first issue

help wanted
```

---

# Pull Requests

Every significant code change should be submitted through a Pull Request.

A good Pull Request should explain:

```text
What changed?

Why was it necessary?

How was it implemented?

How was it tested?

Does it change security behavior?

Does it change API behavior?

Does it introduce migrations?

Does it introduce breaking changes?
```

---

# Pull Request Size

Prefer focused Pull Requests.

Avoid combining:

```text
provider implementation
+
dashboard redesign
+
database migration
+
security refactor
```

into a single PR unless the changes are inseparable.

Smaller PRs are easier to:

- review;
- test;
- reason about;
- revert;
- secure.

---

# Draft Pull Requests

Use Draft Pull Requests when:

- architecture is still evolving;
- implementation is incomplete;
- early review would help;
- a large feature needs design feedback.

---

# Commit Messages

Use clear, concise commit messages.

Recommended style:

```text
feat: add agent runtime registry

fix: prevent task assignment to offline agents

security: enforce workspace boundary for file tools

refactor: separate provider registry from agent runtime

docs: document MCP external agent flow

test: add brain recovery integration tests
```

---

# Recommended Commit Types

```text
feat

fix

security

refactor

docs

test

build

ci

chore

perf
```

---

# Commit Scope

Optional scopes may be used:

```text
feat(agent): add runtime configuration

fix(mcp): reject expired agent sessions

security(shell): enforce command allowlist

feat(dashboard): add agent status panel
```

---

# Atomic Commits

A commit should ideally represent one logical change.

Avoid commits such as:

```text
stuff

more changes

fix everything

final final
```

---

# Go Contribution Guidelines

Follow standard Go conventions.

Always format Go code with:

```bash
gofmt
```

Before submitting:

```bash
gofmt -w .
```

where appropriate.

---

# Go Validation

Run:

```bash
go vet ./...
```

---

# Go Tests

Run:

```bash
go test ./...
```

For concurrency-sensitive changes:

```bash
go test -race ./...
```

---

# Go Package Design

Prefer small, domain-focused packages.

Avoid:

```text
utils
helpers
common
misc
```

unless the package has a very clear and narrow purpose.

Prefer:

```text
agent

task

provider

security

audit
```

---

# Interfaces

Introduce interfaces where they represent actual architectural boundaries.

Examples:

```go
type Provider interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}
```

or:

```go
type AgentRuntime interface {
    Execute(
        ctx context.Context,
        req ExecutionRequest,
    ) (*ExecutionResult, error)
}
```

Do not create interfaces merely to wrap every struct.

---

# Context

Functions performing:

- network operations;
- database operations;
- provider requests;
- long-running work;
- external tools;

should generally accept:

```go
context.Context
```

---

# Error Handling

Errors should provide useful context.

Prefer:

```go
return fmt.Errorf("load project %s: %w", projectID, err)
```

over:

```go
return err
```

Do not expose secrets through error messages.

---

# Panic Usage

Avoid using panic for normal application errors.

Panic should generally be reserved for unrecoverable programmer errors during startup or initialization.

Runtime errors should be handled gracefully.

---

# Logging

Use structured logging.

Prefer fields such as:

```text
project_id

agent_id

task_id

run_id

provider

runtime
```

Never log:

```text
API keys

Authorization headers

passwords

private keys

raw secrets
```

---

# Concurrency

Go concurrency should remain understandable.

Prefer:

- context cancellation;
- bounded worker pools;
- clear ownership;
- controlled goroutines.

Avoid uncontrolled goroutine creation.

---

# Angular Contribution Guidelines

The dashboard uses:

```text
Angular

Angular Material

SCSS
```

Keep the frontend aligned with this stack.

---

# Angular Structure

Features should normally live under:

```text
src/app/features/
```

Examples:

```text
projects

kanban

brain

agents

providers

runs

logs

settings
```

Reusable components belong under:

```text
src/app/shared/
```

Cross-cutting application services belong under:

```text
src/app/core/
```

---

# Angular Components

Prefer small, focused components.

Avoid very large components responsible for:

```text
API calls
+
state
+
rendering
+
modal management
+
business logic
```

at the same time.

---

# Angular Services

HTTP communication should be centralized through services.

Avoid calling backend APIs directly from unrelated UI components.

---

# Angular Material

Prefer Angular Material components for:

```text
dialogs

buttons

inputs

tables

menus

cards

navigation
```

before introducing custom replacements.

---

# SCSS

Keep styling modular.

Avoid global styles when feature-local styles are sufficient.

Use shared variables and theme definitions where appropriate.

---

# Accessibility

New dashboard features should consider:

- keyboard navigation;
- labels;
- semantic structure;
- accessible dialogs;
- color contrast;
- focus management.

---

# Internationalization

The dashboard is intended to support multiple languages.

Initial expected languages include:

```text
pt-BR

en-US
```

Avoid hardcoding text in ways that prevent future localization.

---

# API Guidelines

The REST API should use versioned paths.

Example:

```text
/api/v1
```

---

# Resource Naming

Prefer plural nouns:

```text
/api/v1/projects

/api/v1/agents

/api/v1/tasks

/api/v1/providers
```

---

# HTTP Methods

Use conventional semantics:

```text
GET
POST
PATCH
DELETE
```

---

# Validation

Never trust frontend input.

Validation belongs in the Go backend.

The Angular dashboard is not a security boundary.

---

# Error Responses

API errors should be structured and predictable.

Avoid returning internal stack traces to clients.

---

# MCP Guidelines

MCP is a major boAIrd interface.

Changes to MCP should preserve:

- authentication boundaries;
- agent identity;
- project boundaries;
- capabilities;
- auditability.

---

# MCP Does Not Bypass Security

Connecting through MCP does not grant authority.

The flow should remain:

```text
MCP Client
    ↓
Identity
    ↓
Agent
    ↓
Project
    ↓
Capability Check
    ↓
Tool
```

---

# External Agent Identity

External clients must not be able to simply declare:

```text
I am the Brain
```

The boAIrd configuration determines which registered agent is the Brain.

---

# MCP Tool Naming

Use predictable names.

Examples:

```text
boaird.project.get

boaird.agent.list

boaird.task.list

boaird.task.claim

boaird.task.complete

boaird.conversation.send
```

Avoid inconsistent naming between tools.

---

# Agent Runtime Guidelines

An Agent is not necessarily an API-backed model.

Always preserve the abstraction:

```text
Agent
  ↓
Runtime
```

Possible runtimes:

```text
API

MCP

Local

Remote
```

Do not put provider-specific logic directly inside the Agent domain model.

---

# Brain Guidelines

The Brain is a specialized Agent.

Do not duplicate all Agent logic into a completely separate implementation unless necessary.

The Brain has additional responsibilities but should reuse common runtime, status, capability, and identity abstractions.

---

# Brain Authority

The Brain may:

```text
create tasks

assign tasks

review work

guide failures

coordinate agents
```

The Brain must not:

```text
grant itself capabilities

change administrative security policies

access secrets directly

disable auditing
```

---

# Capability and Security Guidelines

Security-sensitive changes require special care.

Review:

```text
SECURITY.md
```

before implementing:

- shell;
- filesystem tools;
- Git;
- GitHub;
- credentials;
- external agents;
- MCP;
- authentication;
- remote mode.

---

# Capability First

A new sensitive operation must define a capability.

Example:

New operation:

```text
delete repository
```

should not simply be added as a method.

It should define something like:

```text
github.repository.delete
```

---

# Security Definition of Done

For every sensitive operation, consider:

```text
Capability

Default permission

Authorization

Resource validation

Audit event

Failure behavior

Human review requirement
```

---

# Workspace Boundaries

Filesystem operations must remain inside the configured workspace unless explicit administrative permission exists.

Contributors must protect against:

```text
../

absolute path escapes

symbolic link escapes
```

---

# Shell Security

Do not execute arbitrary LLM output as shell commands.

Never implement patterns such as:

```go
exec.Command("sh", "-c", modelOutput)
```

without strict security controls.

Prefer structured command execution.

---

# Secret Handling

Secrets must never be:

```text
returned through normal APIs

sent to the Angular dashboard

written to logs

stored in plaintext
```

Provider clients should retrieve secrets through the credential service.

---

# Database Guidelines

Migrations should be:

- deterministic;
- reversible where practical;
- tested;
- backward-aware.

---

# SQLite

SQLite is the default MVP database.

Avoid using database-specific features that make future PostgreSQL support unnecessarily difficult unless justified.

---

# Queries

Use parameterized queries.

Never concatenate user or model input into SQL strings.

---

# Testing

New behavior should include tests where practical.

Types of tests include:

```text
unit

integration

end-to-end

security regression
```

---

# Unit Tests

Use unit tests for:

```text
domain rules

status transitions

capability decisions

provider normalization

context selection
```

---

# Integration Tests

Use integration tests for:

```text
database repositories

API handlers

provider adapters

MCP sessions

orchestrator workflows
```

---

# End-to-End Tests

Important flows include:

```text
Create Project
↓
Configure Brain
↓
Create Worker
↓
Create Task
↓
Assign Worker
↓
Execute
↓
Review
↓
Done
```

---

# Security Regression Tests

A security bug fix should include a regression test when practical.

Examples:

```text
workspace traversal blocked

unauthorized git push rejected

expired MCP session rejected

secret redaction verified
```

---

# Frontend Tests

Angular changes should test important behavior where useful.

Especially:

```text
dialogs

forms

Kanban transitions

agent configuration

human review actions
```

---

# Documentation

New major features should include documentation.

Possible locations:

```text
README.md

ROADMAP.md

SECURITY.md

docs/
```

---

# Architecture Documentation

Significant architectural decisions should be documented.

If a change introduces a new core abstraction, explain:

```text
why it exists

what problem it solves

how it interacts with existing components

what alternatives were considered
```

---

# Comments

Code comments should explain:

```text
why
```

more often than:

```text
what
```

Avoid comments that simply repeat obvious code.

---

# Logging and Observability

New workflows should emit useful events.

Examples:

```text
task.created

task.assigned

agent.started

agent.failed

brain.reviewed

tool.executed

security.permission_denied
```

Avoid logging every internal detail if it adds noise without diagnostic value.

---

# Audit Events

Sensitive operations should generate audit events.

Examples:

```text
agent.capability.granted

agent.capability.revoked

brain.changed

shell.executed

git.push

github.pr.merged

credential.created
```

---

# AI-Assisted Contributions

AI-assisted development is allowed.

Contributors may use:

```text
Codex

ChatGPT

Claude

Gemini

DeepSeek

Qwen

local models

other coding agents
```

However:

> The contributor remains responsible for the submitted code.

AI-generated code must be reviewed before submission.

---

# AI Contribution Requirements

Do not submit generated code without understanding it.

Contributors should verify:

```text
correctness

security

licensing

tests

architecture compatibility
```

---

# AI Hallucinations

Generated APIs, packages, functions, or protocol behavior must be verified.

Do not assume generated information is correct merely because a model produced it.

---

# AI and Secrets

Never paste project secrets into third-party AI providers.

This includes:

```text
API keys

production tokens

SSH keys

private repository credentials

passwords
```

---

# AI Authorship

AI tools are not treated as legal contributors or copyright holders.

The person submitting the contribution is responsible for ensuring they have the right to contribute the submitted content.

---

# Generated Code

Generated files should only be committed when they are part of the expected repository workflow.

Avoid committing:

```text
temporary build output

local cache

IDE state

generated secrets

debug dumps
```

---

# Security Vulnerabilities

Do not publicly disclose exploitable security vulnerabilities before maintainers have had a reasonable opportunity to investigate and patch them.

For security reports, include:

```text
affected component

impact

reproduction steps

proof of concept if safe

suggested mitigation
```

Do not include real user secrets.

Refer to:

```text
SECURITY.md
```

for the project's security model.

---

# Breaking Changes

Breaking changes should be clearly identified.

Examples:

```text
REST contract changes

MCP tool changes

database migration incompatibility

configuration changes

provider interface changes

agent runtime changes
```

A Pull Request containing a breaking change should explain the migration path.

---

# Dependency Policy

boAIrd prefers a small dependency surface.

Before adding a dependency, consider:

```text
Is it necessary?

Is it actively maintained?

Is it secure?

Is the license compatible?

Can the standard library solve the problem?

Will it increase runtime size significantly?
```

---

# Go Dependencies

Prefer standard library functionality where practical.

Do not add large frameworks for small tasks.

---

# Angular Dependencies

Avoid unnecessary frontend packages.

Angular Material should cover much of the base UI.

---

# License Compatibility

Contributors must ensure added dependencies are compatible with boAIrd's licensing model.

If uncertain, raise the issue before merging.

---

# Code Review

Reviewers should evaluate:

```text
correctness

architecture

maintainability

security

tests

performance

API compatibility

documentation
```

---

# Security Review

Changes involving these areas deserve additional review:

```text
credentials

shell

filesystem

Git

GitHub

MCP authentication

external agents

capability engine

remote mode

authorization
```

---

# Performance

Avoid premature optimization.

However, contributions should avoid clearly unbounded behavior such as:

```text
unbounded goroutines

unbounded queues

unbounded message history loading

unbounded request payloads
```

---

# Contributor Conduct

Contributors should communicate respectfully and constructively.

Technical disagreement is welcome.

Personal attacks, harassment, discrimination, or hostile behavior are not.

Focus discussions on:

```text
code

architecture

security

behavior

tradeoffs
```

not individuals.

---

# Licensing

boAIrd is distributed under the MIT License. See [LICENSE](LICENSE) for the full terms.

This repository does not specify a separate Contributor License Agreement (CLA) or Developer Certificate of Origin (DCO). If a contribution has licensing restrictions or its compatibility with the MIT License is unclear, raise the question with the maintainers before submitting it.

---

# Commercial Contributions

Companies may contribute code to boAIrd.

The project license is MIT. The project documentation does not add commercial restrictions or require separate commercial authorization.

---

# Recommended Pull Request Workflow

```text
Issue
  ↓
Branch
  ↓
Implementation
  ↓
Tests
  ↓
Local Validation
  ↓
Commit
  ↓
Push
  ↓
Pull Request
  ↓
Review
  ↓
Changes if needed
  ↓
Merge
```

---

# Example Feature Workflow

Suppose you want to implement:

```text
Git Manager Agent
```

Recommended process:

```text
1. Open Issue

2. Describe capabilities

3. Define security model

4. Implement domain changes

5. Implement Git tool

6. Add capabilities

7. Add audit events

8. Add tests

9. Add dashboard support

10. Update documentation

11. Open Pull Request
```

---

# Example Security-Sensitive Contribution

Feature:

```text
github.pr.merge
```

The contribution should define:

```text
Capability:
github.pr.merge

Default:
false

Authorization:
required

Audit:
github.pr.merged

Human Review:
configurable

Tests:
required
```

Simply adding a button that merges a Pull Request is not sufficient.

---

# Example Provider Contribution

A new provider should normally include:

```text
provider implementation

configuration

credential handling

model selection

timeouts

error mapping

usage information where available

tests

documentation
```

The provider must not expose raw credentials to agents.

---

# Example MCP Contribution

A new MCP operation should define:

```text
tool name

input schema

output schema

required identity

required capability

project boundary

audit behavior

error behavior
```

---

# Example Angular Contribution

A new Agent configuration page should:

```text
use Angular Material

follow project routing conventions

use existing API services

respect i18n structure

show validation errors

not expose credentials

not implement authorization only in the UI
```

---

# Contributor Checklist

Before opening a Pull Request, verify:

## General

```text
[ ] The change has a clear purpose.

[ ] Existing Issues and PRs were checked.

[ ] The implementation follows project architecture.

[ ] No unrelated changes are included.
```

## Go

```text
[ ] Code is formatted.

[ ] go vet ./... passes.

[ ] go test ./... passes.

[ ] Race-sensitive changes were tested with -race.
```

## Angular

```text
[ ] Dashboard builds successfully.

[ ] UI follows Angular Material conventions.

[ ] New strings are localization-ready.

[ ] Important UI behavior is tested where appropriate.
```

## Security

```text
[ ] No secrets are logged.

[ ] New sensitive operations have capabilities.

[ ] Permissions are checked in the backend.

[ ] Workspace boundaries are respected.

[ ] Audit events exist where necessary.

[ ] Default behavior is secure.
```

## API / MCP

```text
[ ] Inputs are validated.

[ ] Errors do not leak sensitive information.

[ ] Breaking changes are documented.

[ ] MCP operations do not bypass capability checks.
```

## Documentation

```text
[ ] Documentation was updated if necessary.

[ ] Public behavior changes are documented.

[ ] Configuration changes are documented.
```

---

# Contribution Priority

Contributions aligned with the active ROADMAP are generally easier to review and merge.

Current project priorities should be checked in:

```text
ROADMAP.md
```

---

# Good First Contributions

Good initial contributions may include:

```text
documentation improvements

test coverage

small dashboard components

provider error handling

API validation

logging improvements

translation

health checks

security regression tests
```

---

# Architecture Changes

Major architectural changes should be discussed before implementation.

Examples:

```text
replacing SQLite

introducing microservices

replacing Angular

changing MCP architecture

changing AgentRuntime

changing capability model

changing security hierarchy
```

Please open an Issue or design discussion first.

---

# Final Principle

Every contribution should preserve the fundamental boAIrd execution model:

```text
AI proposes
      ↓
Brain coordinates
      ↓
Policy validates
      ↓
Capability authorizes
      ↓
Tool executes
      ↓
Audit records
      ↓
Human remains in control
```

The purpose of boAIrd is not to give AI unlimited access to a machine.

The purpose is to create a controlled environment where AI agents can collaborate productively while remaining observable, replaceable, permission-bound, and subordinate to the user.

---

# Thank You

Thank you for helping improve boAIrd.

Whether you contribute code, documentation, testing, architecture feedback, bug reports, security findings, translations, or ideas, every useful contribution helps make the project stronger.

Welcome to boAIrd.
