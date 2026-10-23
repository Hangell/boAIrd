# boAIrd Extension SDK

This document defines the versioned compatibility contract for boAIrd tool
extensions. Extensions add governed tools without changing the core, and are
subject to administrator approval, explicit registration, and the same
capability and resource boundaries as built-in tools.

## Contract version

The only accepted `sdk_version` is **`1`**. Manifests with any other version are
rejected. The contract may evolve only with a new integer version; older
manifests are never silently reinterpreted.

## Manifest

An extension manifest is a JSON object:

```json
{
  "sdk_version": 1,
  "name": "example-tools",
  "version": "1.0.0",
  "tools": [
    {
      "name": "example.search",
      "description": "Search an allowlisted API",
      "kind": "http",
      "endpoint": "https://api.example.com/search",
      "input_schema": { "type": "object" },
      "output_schema": { "type": "object" },
      "required_capabilities": ["example.search"],
      "data_access": "network:api.example.com"
    }
  ]
}
```

### Fields

- `sdk_version` — required, must be `1`.
- `name` — required, matches `^[a-z][a-z0-9._-]*$`, at most 64 characters.
- `version` — required, bounded to 64 characters.
- `tools` — required, 1 to 16 declarations.

### Tool declaration

- `name` — required, matches `^[a-z][a-z0-9._-]*$`.
- `description` — required.
- `kind` — `declarative` (contract only, not executable) or `http`.
- `endpoint` — required for `http`; an absolute `https://` URL with no embedded
  credentials, query, or fragment; it must use port 443 and its exact public DNS
  host must be configured in `BOAIRD_PROVIDER_ALLOWED_HOSTS`.
- `input_schema` / `output_schema` — required object-root JSON schemas from the
  bounded SDK v1 subset: `type`, `properties`, `required`,
  `additionalProperties`, `items`, `enum`, `minLength`, `maxLength`, `minItems`,
  `maxItems`, `minimum`, `maximum`, and `pattern`. Unsupported keywords,
  schemas over 32 KiB, and nesting deeper than 16 are rejected. The declared
  schemas validate each input and the redacted output before the call succeeds.
- `required_capabilities` — required; at least one lowercase dotted capability
  identifier, enforced by the tool Enforcer on every invocation.
- `data_access` — `none` (default) or `network:<host>`. For `http`, the host
  must exactly match the endpoint hostname. Filesystem, secret,
  credential, and shell/exec access are rejected.

## Boundaries

Extensions are declarative manifests, never executable code. They cannot read
arbitrary files or secrets and cannot execute arbitrary programs. An `http` tool
performs one bounded, redacted `POST` of its validated input to its declared
HTTPS endpoint; the shared provider endpoint policy checks the exact hostname,
pins DNS resolution, blocks private and special-purpose addresses, disables
proxies and redirects, and requires TLS 1.2 or later. Provider CIDR exceptions
do not widen extension destinations. Responses are size-limited
and secret-redacted. `declarative`
tools expose a reviewed contract but have no execution until a core-reviewed
adapter is added.

## Lifecycle

```text
install (pending)  ->  activate (active, tools registered)  ->  deactivate (removed, tools unregistered)
```

- Installation validates the manifest, reserves every tool name, and stores it
  as `pending` in one transaction with the immutable audit event.
- Activation is an administrator-only (`extension.manage`) operation. It first
  atomically registers all executable tools, then commits active status and its
  audit event. Runtime calls also check persisted active status before any
  adapter runs, so a pending or deactivated extension cannot execute during a
  registration or removal race.
- Deactivation removes active state and records its audit event in one
  transaction, then unregisters the tools. Persisted active-state checks deny
  calls immediately after the transaction commits.
- Every lifecycle change is recorded in the immutable `extension_audit` stream
  (`extension.installed`, `extension.activated`, `extension.deactivated`).
- Tool names are globally unique across active and pending extensions. The
  SQLite migration is `0026_extensions.sql`; PostgreSQL uses
  `0006_extensions.sql`.

## API

```text
GET    /api/v1/extensions
POST   /api/v1/extensions
GET    /api/v1/extensions/{name}
PUT    /api/v1/extensions/{name}/activate
DELETE /api/v1/extensions/{name}
```

All routes require the `extension.manage` capability.

Activated executable tools are invoked through the authenticated MCP gateway
`boaird.extension.execute` with `project_id`, `tool_name`, optional `task_id`
and `run_id`, and an object-valued `input`. The authenticated Agent identity is
derived from its MCP token. The shared Enforcer checks the declared capability,
active extension state, project membership or active-Brain relationship, and
the supplied task/run project scope before calling the HTTP adapter.
