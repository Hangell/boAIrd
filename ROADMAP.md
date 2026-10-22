# boAIrd Roadmap

> **Local-first AI Agent Control Plane built with Go + Angular**

O desenvolvimento do boAIrd será incremental.

A prioridade inicial é construir um MVP funcional, local-first e altamente controlável antes de adicionar recursos distribuídos, multiusuário ou infraestrutura complexa.

Stack principal:

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

# Visão Geral

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
Phase 17  Observability
Phase 18  Autonomous Mode
Phase 19  Remote Mode
Phase 20  Advanced Runtime
```

---

# Phase 0 — Bootstrap

## Objetivo

Criar a base mínima do projeto Go.

Resultado esperado:

```bash
boaird serve
```

Subindo:

```text
http://localhost:3110
```

## Backend

Implementar:

- Go module;
- estrutura inicial de pastas;
- CLI;
- servidor HTTP;
- configuração;
- logs estruturados;
- graceful shutdown;
- healthcheck;
- SQLite;
- migrations.

Estrutura inicial:

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

## Critério de conclusão

```text
boaird serve

↓

API ONLINE
SQLite connected
health = OK
```

---

# Phase 1 — Core Domain

## Objetivo

Definir o domínio principal antes de integrar qualquer IA.

Entidades iniciais:

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

## Critério

Modelos definidos sem dependência direta de OpenAI, Claude ou outro provider.

---

# Phase 2 — Projects

## Objetivo

Permitir criar e gerenciar projetos.

Implementar:

```text
Create Project
Get Project
List Projects
Update Project
Archive Project
```

Projeto deverá possuir:

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

## Regra

Um projeto ainda pode existir sem configuração completa.

Porém:

> um projeto não poderá executar agentes até possuir um Brain ativo.

---

# Phase 3 — Providers

## Objetivo

Criar abstração independente de providers.

Interface:

```go
type Provider interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

    Models(ctx context.Context) ([]Model, error)
}
```

Providers iniciais:

```text
OpenAI
Anthropic / Claude
DeepSeek
Google Gemini
OpenAI-compatible
```

## Implementar

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

## Critério

Uma chamada deve funcionar sem que o restante do sistema saiba qual SDK está sendo utilizado.

---

# Phase 4 — Credentials

## Objetivo

Gerenciar chaves de API com segurança.

Implementar:

```text
Provider
    ↓
Credential
    ↓
Model
```

Nunca:

```text
Agent → raw API key
```

## Segurança

Utilizar:

```text
encrypted secret
+
BOAIRD_MASTER_KEY
```

## Critério

API keys nunca devem aparecer em:

```text
logs
API responses
database plaintext
frontend payloads
```

---

# Phase 5 — Agents

## Objetivo

Permitir criação de agentes especializados.

Campos:

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

Exemplos:

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

## Critério

Deve ser possível criar dois agentes utilizando providers diferentes.

Exemplo:

```text
Backend → DeepSeek

QA → Claude
```

---

# Phase 6 — Brain

## Objetivo

Implementar o conceito central do projeto.

Regra:

```text
Every Project Requires One Active Brain
```

O Brain será um Agent com responsabilidades especiais.

## Funções

Brain poderá:

- analisar objetivo;
- orientar agentes;
- criar tarefas;
- atribuir tarefas;
- revisar entregas;
- analisar erros;
- solicitar nova tentativa;
- substituir QA quando necessário;
- notificar administrador.

## Primeira implementação

Inicialmente:

```text
Brain via API
```

Exemplo:

```text
OpenAI
Claude
Gemini
DeepSeek
```

MCP Brain virá posteriormente.

## Critério

```text
Project
  ↓
Brain
  ↓
Prompt
  ↓
Response
```

com Run persistido.

---

# Phase 7 — Runs

## Objetivo

Toda execução de IA deverá gerar um registro.

Estrutura:

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

## Critério

Toda interação com modelo deve ser auditável.

---

# Phase 8 — Kanban

## Objetivo

Implementar o principal fluxo operacional do boAIrd.

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

Cada Task possuirá:

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

## Permissões

Por padrão:

```text
Brain:
task.create = true

Worker:
task.create = false
```

Agentes como:

```text
PO
SDM
Project Manager
```

podem receber:

```text
task.create
```

## Critério

Criar card e movê-lo por todo o fluxo.

---

# Phase 9 — Conversations

## Objetivo

Permitir comunicação entre agentes e humanos.

Cada Task deverá possuir uma Conversation.

```text
Task
   ↓
Conversation
   ├── Brain
   ├── Worker
   ├── QA
   └── Human
```

Mensagem deverá registrar:

```text
sender
timestamp
content
format
metadata
```

## Formatos

Não limitar comunicação a texto humano.

Suportar futuramente:

```text
text
markdown
json
yaml
binary reference
machine protocol
```

## Critério

Histórico completo deve poder ser aberto através do card.

---

# Phase 10 — Orchestrator

## Objetivo

Criar o mecanismo que coordena Brain, agentes e Tasks.

Fluxo:

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

## Responsabilidades

```text
task assignment
agent availability
execution
status transition
review flow
failure routing
```

## Falha

Não realizar apenas retry automático.

Fluxo:

```text
Worker fails
     ↓
Brain analyzes
     ↓
Brain adds instructions
     ↓
Worker retries
```

Caso o Brain tenha dúvida:

```text
Brain
  ↓
Administrator Notification
```

---

# Phase 11 — Angular Bootstrap

## Objetivo

Criar o dashboard web.

Stack:

```text
Angular
Angular Material
SCSS
```

## Estrutura

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

## Implementar

```text
routing
Angular Material theme
API client
global error handler
SSE service
i18n foundation
```

---

# Phase 12 — Project Dashboard

## Objetivo

Criar visão operacional por projeto.

Tela:

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

Mostrar:

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

## Objetivo

Transformar Tasks em experiência visual.

Interface:

```text
BACKLOG   READY   WORKING   REVIEW   DONE
```

Suportar:

```text
drag & drop
filters
agent avatar
priority
status
human review
```

Ao clicar:

```text
Task Modal
```

com:

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

## Objetivo

Permitir configuração completa de agentes.

Tela:

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

## Objetivo

Permitir configurar e observar o Brain.

Configurações:

```text
Brain Agent
Model
Runtime
Instructions
Capabilities
Autonomy Mode
```

Mostrar:

```text
status
current activity
current reasoning request
tasks managed
failures
notifications
```

---

# Phase 16 — Capabilities

## Objetivo

Criar camada de segurança independente de role.

Exemplos:

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

Regra:

```text
Role != Permission
```

## Critério

Backend pode ter:

```text
filesystem.write
```

sem possuir:

```text
git.push
```

---

# Phase 17 — Autonomy Modes

## Manual

Brain orienta, mas ações importantes dependem do usuário.

```text
MANUAL
```

---

## Semi-Auto

Brain distribui e acompanha trabalho.

```text
SEMI_AUTO
```

Ações críticas ainda podem exigir aprovação.

---

## Autonomous

```text
AUTONOMOUS
```

Brain pode:

```text
create tasks
assign agents
review
retry
reprioritize
finish tasks
```

Mas nunca ultrapassar capabilities.

---

# Phase 18 — Human Review

## Objetivo

Implementar aprovação humana opcional.

Quando:

```text
human_review = false
```

Fluxo:

```text
Worker
  ↓
QA / Brain
  ↓
DONE
```

Quando:

```text
human_review = true
```

Fluxo:

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

## Objetivo

Transformar boAIrd em servidor MCP.

Primeiras ferramentas:

```text
boaird.project.get

boaird.agent.list
boaird.agent.get

boaird.task.list
boaird.task.get
boaird.task.claim
boaird.task.update
boaird.task.complete
boaird.task.fail

boaird.conversation.read
boaird.conversation.send

boaird.context.get
```

## Critério

Um MCP Client deve conseguir:

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

---

# Phase 20 — External Agents

## Objetivo

Permitir agentes fora do processo principal.

Exemplos:

```text
Codex
VS Code Agent
Claude
Cursor
custom agent
local CLI
```

Primeiro modelo:

```text
Pull
```

Fluxo:

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

## Objetivo

Monitorar agentes externos.

Estados:

```text
ONLINE
BUSY
STALE
OFFLINE
```

Fluxo:

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

---

# Phase 22 — External Brain

## Objetivo

Permitir que o Brain também seja externo.

Exemplos:

```text
Codex in VS Code

Claude app

GPT client

Gemini

local AI
```

boAIrd deve tratar da mesma forma:

```text
BrainRuntime
```

independentemente de onde ele está sendo executado.

---

# Phase 23 — Agent Runtime Abstraction

Implementar:

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

Implementações:

```text
API Runtime
MCP Runtime
Local Runtime
Remote Runtime
```

---

# Phase 24 — Context Engine

## Objetivo

Selecionar contexto de forma inteligente.

Contexto final:

```text
Brain selection
+
Agent request
+
boAIrd Context Engine
```

Fontes:

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

## Objetivo

Reduzir custo e excesso de contexto.

Fluxo:

```text
Raw History
    ↓
Brain Summary
    ↓
Optimized Memory
```

Categorias:

```text
Architecture Decisions

Project Decisions

Important Context

Known Problems

Current State

Task History
```

## Regra

Histórico bruto continua disponível.

Mas não precisa ser enviado para modelos em toda execução.

---

# Phase 26 — Tools

## Objetivo

Criar sistema genérico de ferramentas.

Estrutura:

```text
Tool Registry
```

Primeiras tools:

```text
Filesystem
Shell
Git
GitHub
```

Futuras:

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

## Objetivo

Execução controlada de comandos.

Não conceder shell irrestrito por padrão.

Capabilities:

```text
shell.execute
shell.go
shell.node
shell.angular
shell.nest
```

Exemplo:

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

## Objetivo

Permitir operações Git controladas.

```text
git.read
git.diff
git.branch
git.commit
git.push
git.merge
```

Também suportar:

```text
Git Manager Agent
```

---

# Phase 29 — GitHub

Implementar:

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

Criar suporte para perfis mais avançados.

Exemplos:

```text
Software Architect

Security Engineer

QA

Git Manager

DevOps

PO

SDM
```

Cada um podendo possuir ferramentas exclusivas.

Exemplo:

```text
Software Architect

├── Mermaid
├── PlantUML
└── draw.io
```

---

# Phase 31 — Observability

## Objetivo

Entender tudo que acontece.

Dashboard:

```text
Runs
Logs
Errors
Agent activity
Task history
```

Categorias:

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

Registrar ações críticas.

```text
Actor

Action

Resource

Timestamp

Result
```

Especialmente:

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

Criar sistema interno de notificações.

Exemplos:

```text
Brain needs help

Agent failed

Human review required

External agent disconnected

Provider unavailable
```

---

# Phase 34 — Internationalization

Dashboard inicialmente:

```text
pt-BR
en-US
```

Preparado para novos idiomas.

Agentes também podem possuir idioma preferencial.

---

# Phase 35 — Remote Mode

## Objetivo

Permitir execução segura fora de localhost.

Implementar:

```text
authentication
TLS
API tokens
remote dashboard
remote MCP
```

---

# Phase 36 — PostgreSQL

SQLite permanece padrão local.

PostgreSQL entra para ambientes maiores.

```text
SQLite
   ↓
PostgreSQL
```

Repositories devem impedir dependência forte do banco desde o início.

---

# Phase 37 — Distributed Workers

Somente quando realmente necessário.

Possíveis tecnologias:

```text
NATS
Valkey
Redis Streams
RabbitMQ
```

Não implementar antes de existir necessidade real.

---

# Phase 38 — Multi-User

Futuro.

Implementar:

```text
Users
Organizations
Roles
Permissions
Project membership
```

---

# Phase 39 — Agent Templates

Permitir agentes reutilizáveis.

```text
Global Agent Template
        ↓
Project Agent
```

Exemplos:

```text
Go Senior

Angular Senior

Security Reviewer

Git Manager
```

---

# Phase 40 — Tool Marketplace / Extensions

Futuro.

Permitir adicionar ferramentas sem alterar o core.

```text
boAIrd Tool SDK
```

Possíveis integrações:

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

O MVP será considerado funcional quando este fluxo existir:

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

Com histórico disponível no dashboard.

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

Primeiro fluxo:

```text
Brain API
+
Worker API
```

---

# MVP 2

Adicionar:

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

Adicionar:

```text
MCP Server

External Agents

Codex / VS Code integration

heartbeat
```

---

# MVP 4

Adicionar:

```text
External Brain

Context Engine

Memory optimization
```

---

# Beta

O projeto poderá entrar em Beta quando possuir:

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

# Pós-Beta

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

# Prioridades

## P0 — Obrigatório

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

## P1 — Muito importante

```text
Conversations
Capabilities
Autonomy
Human Review
MCP
External Agents
```

---

## P2 — Evolução

```text
Context Engine
Memory
Tools
Git
GitHub
Observability
```

---

## P3 — Escala

```text
Remote Mode
PostgreSQL
Multi-user
Distributed workers
Extensions
```

---

# Regras Arquiteturais

## Regra 1

O core não depende de um modelo específico.

```text
Model Agnostic
```

---

## Regra 2

Agent não significa API.

```text
Agent
  ↓
Runtime
```

Runtime pode ser:

```text
API
MCP
Local
Remote
```

---

## Regra 3

Brain sempre existe.

```text
Project
   ↓
Brain
```

---

## Regra 4

Administrador possui autoridade máxima.

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

## Regra 5

Autonomia não concede permissão.

```text
Autonomy != Capability
```

---

## Regra 6

Kanban pertence ao domínio.

Não apenas ao Angular.

---

## Regra 7

Toda execução relevante deve ser auditável.

---

## Regra 8

Segurança deve utilizar:

```text
deny by default
+
least privilege
```

---

## Regra 9

Não adicionar infraestrutura distribuída cedo demais.

O MVP será:

```text
Modular Monolith
```

---

# Arquitetura Inicial

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

# Primeiro marco técnico

O primeiro marco de desenvolvimento deverá ser:

```bash
go run ./cmd/boaird serve
```

Resultado:

```text
boAIrd v0.0.1-dev

API          READY
SQLite       READY
Event Bus    READY

Listening:
http://localhost:3110
```

---

# Primeiro marco funcional

Depois:

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

Esse será o primeiro momento em que o boAIrd poderá ser considerado um **AI Agent Control Plane funcional**.

---

# Desenvolvimento recomendado

A ordem recomendada é:

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

Evitar começar simultaneamente pelo MCP, Angular e integração com vários agentes.

Primeiro construir um fluxo pequeno que funcione completamente.

Depois expandir.

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
