# ADR 0001 — Gate distributed workers on demonstrated need

- Status: **Proposed — awaiting maintainer approval**
- Date: 2026-10-22
- Decision gate for: ROADMAP Phase 37 (Distributed Workers), ARCHITECTURE §110
  (Distributed Queue) and §111 (Scalability Strategy)
- Evidence: [in-process baseline measurements](../measurements/2026-10-22-in-process-baseline.md)

While this ADR is unapproved, the decision it records is already in force: no
broker, queue service, or distributed worker runtime is added to boAIrd.

---

## 1. Context

boAIrd is a local-first modular monolith:

```text
Angular dashboard ── REST/SSE ── Go core ── SQLite (single writer)
                                     └────── MCP ── external Agents (pull)
```

Execution is pull-based. External Agents connect over MCP, list the tasks
assigned to them, claim a task, and report progress; the Go core owns all task
state and records an audit event for every transition. The Brain executes one
synchronous provider call per objective. There is no background execution pool
and no message broker.

The roadmap lists distributed workers (NATS, Valkey, Redis Streams, RabbitMQ)
as a post-beta concern, and the architecture document requires the Orchestrator
to hide any future queue behind an abstraction. The risk this ADR addresses is
the opposite of under-provisioning: adding broker infrastructure before there is
evidence that the single-process design cannot carry the load. A broker brings a
new failure domain, a new durability domain, new credentials, a new upgrade
path, and new attack surface, and it does not by itself make the product more
available.

## 2. Decision

1. **The in-process design remains the only execution and queue mechanism.**
   Bounded, synchronous, single-process: SQLite serializes writes through one
   connection; work is claimed by Agents through optimistic, version-checked
   transitions.
2. **No broker, queue service, or distributed runtime is introduced** while the
   measured capacity envelope in §3 covers the demand envelope in §4.
3. **When the gate in §3 trips, the response order is fixed:** measure again,
   optimize the single node (query shape, pagination, payload size), then Phase
   36 (PostgreSQL) for concurrency, and only then consider a broker. A broker is
   the last option, not the first.
4. **Introducing a broker requires a new or amended ADR approved by a
   maintainer**, containing measurements from the deployment that tripped the
   gate and the queue semantics, recovery, security, and operating cost defined
   in §5–§8. Infrastructure or runtime changes must not precede that approval.
5. **Bounded behaviour is preserved until then.** No unbounded goroutines,
   unbounded channels, or fire-and-forget execution are added while the gate is
   closed.

## 3. Gate criteria

The gate opens only when **measured evidence** from a real deployment shows at
least one of the following. Each criterion is intentionally conservative: it
should be hard to trip by accident and easy to falsify.

| ID | Criterion | Threshold | Why this number |
| --- | --- | --- | --- |
| G1 | Sustained task-state writes | > 25 transitions/s for 10 continuous minutes, **or** write p95 > 250 ms while offered write demand is below 25 transitions/s | Measured floor is ~34 writes/s on a durable SQLite file; 25 stays below that floor, and is ~2× the pathological demand in §4 and ~25× typical demand |
| G2 | Board/list reads | > 30 list requests/s with p95 > 500 ms | Measured floor is ~37 list reads/s; 30 stays below it, and the latency condition makes the verdict about the design rather than a missing index |
| G3 | Availability requirement | The product must survive a process restart without a supervision window, or needs unattended recovery/failover | A single process cannot provide this regardless of throughput; that is the real reason to distribute |
| G4 | Execution topology | Work must execute on hosts that cannot reach the same boAIrd process while keeping one authoritative board | Today Agents already run on other hosts; they reach the same process over MCP |
| G5 | Durability/queue semantics | The product needs delivery guarantees the pull model cannot express (e.g. broker-style retries with dead-lettering across process restarts) | Would require explicit queue semantics, which the current pull model leaves to the Brain and Human Review |

Notes:

- G1 and G2 must be measured with the reproduction method in the measurement
  record, not from dashboards or guesses.
- Tripping one criterion opens the discussion; it does not authorize
  implementation. §2.4 still applies.
- PostgreSQL (Phase 36) is a separate decision and is **not** authorized by this
  ADR. If PostgreSQL arrives, `SELECT … FOR UPDATE SKIP LOCKED` may satisfy
  multiple API processes without a broker; that path also needs its own
  approved ADR because it changes the deployment topology.

## 4. Demand envelope assumed while the gate is closed

The gate is meaningful only against a stated demand. The design targets:

```text
1 local administrator
1 Brain
up to 10 specialized Agents
task turns measured in seconds to minutes (LLM latency)
1 dashboard session, maybe a few browser tabs
```

Worst case with 10 Agents completing a turn every 5 s and emitting 6 state
changes per turn: **12 writes/s**, about a third of the conservative floor.
Typical operation is far below 1 write/s. Board reads are dominated by
dashboard polling, currently one list request per view refresh.

If the product target changes (multi-user Phase 38, remote mode Phase 35 with
many dashboards, or Agents far faster than LLM latency), the demand envelope
must be restated in this ADR before the gate is re-evaluated.

## 5. Queue semantics required if the gate opens

Any broker integration must define, in the superseding ADR, and implement:

- **Delivery model:** at-least-once. Exactly-once delivery is not claimed.
- **Claim:** a task is claimed by a version-checked transition; a second claim
  for the same version must be rejected, not queued.
- **Lease and visibility:** a claim is a lease with an explicit timeout. An
  expired lease returns the task to `READY` and emits an audit event.
- **Acknowledgment:** acknowledgment is the task transition itself, not a
  separate message ack that can drift from the board.
- **Retries:** bounded per task, with the attempt count persisted; exhaustion
  moves the task to `BLOCKED` or `FAILED` and notifies the Administrator.
- **Dead letters:** messages that cannot be processed are parked, visible to
  the Administrator, and auditable. No silent drops.
- **Ordering:** no global ordering guarantee. Per-task ordering is preserved by
  the version check.
- **Fairness:** no starvation of a project or Agent; priority reordering must be
  explicit and audited.
- **Idempotency:** duplicate processing of the same task version is a no-op.

## 6. Recovery semantics required if the gate opens

- **Process crash:** leases expire; affected tasks return to `READY` with an
  audit event; no task stays silently in `WORKING`.
- **Agent crash:** same as process crash, scoped to the agent's leases.
- **Broker outage:** the board degrades to the pull model over REST/MCP; the
  API must not depend on the broker to stay consistent.
- **Split brain:** a single authoritative writer for task state. Multiple
  consumers may read; only the state owner writes.
- **Recovery evidence:** every recovery action is an audit event, because the
  recovery path is exactly where silent data loss hides.

## 7. Security requirements if the gate opens

The broker is **transport, never authority**. Invariants that must not move:

- Capability checks, project scoping, Human Review, and audit stay in the Go
  core; a message on a topic is not an authorization.
- Every producer and consumer authenticates with its own credential from the
  credential service; no shared plaintext secrets, no tokens in payloads or
  message headers.
- Transport is authenticated and encrypted; the broker is network-isolated
  (loopback or an explicit allow-list, mirroring the provider endpoint policy).
- Topic/queue access is deny-by-default and least-privilege per Agent role.
- Message payloads carry identifiers and versions, not secrets, credentials, or
  raw provider responses.
- Claim, ack, expiry, retry, and dead-letter actions are audited.
- A hostile or replayed message must not be able to advance a task past a
  pending Human Review.

## 8. Operating cost if the gate opens

Recorded here so that "distributed workers" is priced honestly:

- One more service to install, version, upgrade, back up, and monitor, plus its
  client library in the dependency graph.
- A second durability domain: SQLite file **and** broker persistence must be
  consistent enough for recovery; backups and restore drills double.
- New failure modes: broker down, queue backlog, redelivery storms, expired
  leases, poison messages.
- New credentials to rotate and new network paths to keep closed.
- Higher debugging cost: an operator can no longer read one file and one log to
  explain the board.
- Small local deployments (the project's primary target) gain nothing from any
  of it.

## 9. Alternatives considered

| Alternative | Verdict |
| --- | --- |
| Do nothing, keep the in-process pull model | **Chosen** while the gate is closed; measurements show ≥30× headroom over the modelled demand |
| Optimize the single node first | **Preferred first response** when G1/G2 trip: query shape, pagination, payload size, list indexing. Cheaper and reversible |
| PostgreSQL with `SKIP LOCKED` (Phase 36) | A real option for multi-process API workers; still a topology change and still needs its own ADR; does not provide broker delivery semantics |
| External queue/broker (NATS, Valkey, Redis Streams, RabbitMQ) | Deferred behind this gate; highest cost, added failure domain, no availability benefit on its own |
| Push-based Agent dispatch over MCP | Not required: pull-based claims already let Agents on other hosts execute work against one authoritative board |

## 10. Consequences

- The board stays a single source of truth in one process with one writer
  connection; reconciliation problems do not exist yet.
- Current capacity is bounded and known; the first saturation signal is the
  list endpoint and the single writer, both addressable without a broker.
- Availability is that of a single process: restart is the recovery mechanism.
  This is acceptable for local-first operation and is explicitly **not**
  acceptable for a remote multi-user deployment, which is exactly what G3
  captures.
- Any PR that adds a broker, queue client, or distributed worker runtime must
  link an approved ADR. Without it, the change is out of scope (see AGENTS.md
  §83 and §93).

## 11. Approval record

| Role | Name | Decision | Date |
| --- | --- | --- | --- |
| Maintainer | _pending_ | _pending_ | _pending_ |

The gate is closed until a maintainer records approval above (or amends this
ADR). Approval of this ADR approves **only** the gate; it does not authorize
Phase 37 implementation.

## 12. References

- [In-process baseline measurements](../measurements/2026-10-22-in-process-baseline.md)
- `ROADMAP.md` — Phase 37, P3 Scale
- `ARCHITECTURE.md` — §108 Task Queue, §109 Internal Queue, §110 Distributed
  Queue, §111 Scalability Strategy
- `AGENTS.md` — §64 bounded concurrency, §83 dependencies, §93 architecture
  changes
- Reproduction tool: `tests/load/main.go`
