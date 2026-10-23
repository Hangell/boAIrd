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
  credentials.
- `input_schema` / `output_schema` — required JSON object schemas.
- `required_capabilities` — required; at least one lowercase dotted capability
  identifier, enforced by the tool Enforcer on every invocation.
- `data_access` — `none` (default) or `network:<host>`. Filesystem, secret,
  credential, and shell/exec access are rejected.

## Boundaries

Extensions are declarative manifests, never executable code. They cannot read
arbitrary files or secrets and cannot execute arbitrary programs. An `http` tool
performs one bounded, redacted `POST` of its validated input to its declared
HTTPS endpoint; responses are size-limited and secret-redacted. `declarative`
tools expose a reviewed contract but have no execution until a core-reviewed
adapter is added.

## Lifecycle

```text
install (pending)  ->  activate (active, tools registered)  ->  deactivate (removed, tools unregistered)
```

- Installation validates the manifest and stores it as `pending`.
- Activation is an administrator-only (`extension.manage`) operation that
  registers every declared executable tool with the shared tool Enforcer.
- Deactivation unregisters the tools and removes the extension.
- Every lifecycle change is recorded in the immutable `extension_audit` stream
  (`extension.installed`, `extension.activated`, `extension.deactivated`).

## API

```text
GET    /api/v1/extensions
POST   /api/v1/extensions
GET    /api/v1/extensions/{name}
PUT    /api/v1/extensions/{name}/activate
DELETE /api/v1/extensions/{name}
```

All routes require the `extension.manage` capability.
