# In-process baseline — 2026-10-22

Measurement record supporting [ADR 0001 — Gate distributed workers on
demonstrated need](../adr/0001-gate-distributed-workers.md).

The purpose is to answer one question with numbers: **does the current
single-process design (Go core + SQLite single writer + pull-based MCP claims)
carry the workload boAIrd targets, and where does it saturate?** The result
decides whether Phase 37 (distributed workers) may start.

## 1. Environment

```text
CPU       Intel Core i7-10510U (8 threads, 1.80 GHz)
RAM       15 GiB
Storage   /  on NVMe (CT500P3PSSD8, ext4) — database path
Kernel    Linux 7.0.0-38-generic
Go        1.27.1
SQLite    modernc.org/sqlite 1.60.1, journal_mode=delete, synchronous=FULL
Server    boaird serve, single process, no TLS, dashboard not running
Client    tests/load, same host, HTTP keep-alive
```

The host was shared with other work during the runs (a second boaird process was
started for the tmpfs comparison), so absolute numbers are pessimistic and
noisy. That is acceptable: the decision compares orders of magnitude, not
percentages.

## 2. Method

`tests/load` drives the public REST API only, through the same authorization
path as the dashboard:

1. create a project;
2. create N tasks sequentially and record each latency (write path);
3. create N tasks, move them to `READY`, then claim them (`READY → WORKING`) with
   C concurrent workers, each claiming a distinct task;
4. read one task repeatedly with C concurrent workers;
5. read the board list (`limit=50`) repeatedly with C concurrent workers.

Plus two targeted checks:

6. **contended claim:** 16 threads attempt to claim the same `READY` task at the
   same time;
7. **tmpfs comparison:** the same write scenario against a database on
   `/dev/shm`, isolating the cost of durable commits from the rest of the path.

A task transition reads the task, validates the state machine, and updates the
row with a version check (`WHERE id=? AND version=?`); the audit event is written
by a SQLite trigger in the same statement, so the write figures cover the full
audited mutation, not just an `UPDATE`.

## 3. Results — durable SQLite (the real deployment shape)

Ranges are across three runs (one 400-task run and two 120-task runs, all with
the Go harness); `p50`/`p95` are per-operation latencies. A separate 720-task run
with a Python client at concurrency 1/4/16 landed in the same band and is not
tabulated separately.

| Scenario | Throughput | p50 | p95 |
| --- | --- | --- | --- |
| create task, sequential | 38–56 ops/s | 20.8–21.8 ms | 50–53 ms |
| claim `READY → WORKING`, concurrency 1 | 34–77 ops/s | 12.0–24.7 ms | 20–63 ms |
| claim `READY → WORKING`, concurrency 8 | 37–105 ops/s | 68–202 ms | 128–376 ms |
| claim `READY → WORKING`, concurrency 32 | 42–80 ops/s | 302–660 ms | 779 ms–1.29 s |
| get one task, concurrency 1 | 266–315 ops/s | 2.2–2.7 ms | 7–9 ms |
| get one task, concurrency 8 | 266–640 ops/s | 10–22 ms | 29–79 ms |
| get one task, concurrency 32 | 321–950 ops/s | 28–85 ms | 61–194 ms |
| list board `limit=50`, concurrency 1 | 45–107 ops/s | 8.6–21.3 ms | 13–33 ms |
| list board `limit=50`, concurrency 8 | 43–88 ops/s | 72–161 ms | 145–334 ms |
| list board `limit=50`, concurrency 32 | 37–42 ops/s | 625–692 ms | 1.4–1.8 s |

Observations:

- **Write throughput plateaus in the 30–100 ops/s band** regardless of
  concurrency. The single SQLite connection is the serialization point, and each
  commit costs a durable journal write, so adding workers past ~8 converts into
  latency instead of throughput (p95 grows from ~0.2 s to ~1.3 s).
- **Reads of one task reach up to ~950 ops/s** at concurrency 32; the per-query
  cost is ~2 ms, so the ceiling is roughly `1 / 2 ms` on one connection.
- **Board list is the first endpoint to saturate:** 37–107 ops/s, with p95
  above 1.4 s at concurrency 32. The query filters by project and sorts by a
  status rank plus `created_at`, then serializes up to 50 task rows — it is
  cheap per row but competes for the same single connection.
- **Resource footprint is small:** ~28–29 MB RSS, 7.2 MB database holding 7
  projects, 3,142 tasks, and 8,544 audit events.

## 4. Results — tmpfs database (cost of durability)

Same harness, same host, database on `/dev/shm` (no fsync to disk):

| Scenario | Throughput | p50 | p95 |
| --- | --- | --- | --- |
| create task, sequential | 80 ops/s | 5.1 ms | 39 ms |
| claim `READY → WORKING`, concurrency 1 | 148 ops/s | 4.0 ms | 17 ms |
| claim `READY → WORKING`, concurrency 8 | 196 ops/s | 32 ms | 84 ms |
| claim `READY → WORKING`, concurrency 32 | 214 ops/s | 124 ms | 251 ms |

Durable commits cost roughly **2–5×** write throughput. That is the price of
crash safety (`journal_mode=delete`, `synchronous=FULL`) and it is not a reason
to distribute work: a broker adds its own durability cost on top.

## 5. Contended claim (correctness under contention)

16 threads claiming the same `READY` task simultaneously:

```text
HTTP 200 × 1   claim granted
HTTP 409 × 1   optimistic-lock conflict (lost the version race)
HTTP 400 × 14  task was already WORKING when the request was evaluated
```

Exactly one winner, no duplicate ownership, no partial state. The rejected
callers receive a deterministic error and must reload the board. Disjoint claims
(§3) produced **zero** conflicts, so the version check rejects real races without
punishing independent work.

Conclusion: atomic claiming is already solved in-process. A broker would not
improve it; it would only move where the lock lives.

## 6. Where the design actually stops

Measured limits, in the order they matter:

1. **One SQLite connection (`SetMaxOpenConns(1)`)** — every query, read or
   write, queues behind the same connection. This is the hard ceiling and the
   first thing to change when read demand grows (it does not require a broker).
2. **Durable commit per audited mutation** — correct and required, but it caps
   writes at ~30–100/s on this class of machine.
3. **Board list cost** — status-ranked sort plus a 50-row payload on the shared
   connection; the first endpoint to show queueing latency.
4. **Availability is single-process** — restart is the recovery mechanism; there
   is no lease, no lease expiry, and no cross-process ownership. A crashed Agent
   leaves its task in `WORKING` until a human or the Brain moves it. This is a
   real gap for remote/multi-user operation (gate G3) and invisible to the
   local-first workload.
5. **No delivery guarantees** — the pull model has no redelivery, visibility
   timeout, or dead-letter path. Retries are the Brain's responsibility today
   (gate G5).

## 7. Headroom against the modelled demand

Target workload (ADR §4): 1 administrator, 1 Brain, ≤10 Agents, LLM-paced turns.

```text
10 Agents × 1 turn / 5 s × 6 state changes = 12 writes/s (pathological)
measured conservative floor                  = 34 writes/s
headroom                                     ≈ 3× worst case, ≈ 30× typical (<1 write/s)
```

Board reads: one dashboard refreshing a list view at 1 Hz is ~1 request/s
against a measured floor of ~37/s. The list endpoint only becomes a problem when
many tabs or multi-user dashboards poll it, which is exactly the trigger G2
records.

The in-process design is sufficient for the product as specified. The gate stays
closed.

## 8. Reproduction

```bash
# throwaway database and token; never reuse production secrets
export BOAIRD_MASTER_KEY=$(printf 'a%.0s' {1..64})
export BOAIRD_LOCAL_AUTH_TOKEN=$(printf 'b%.0s' {1..64})
BOAIRD_DATABASE_PATH=/tmp/boaird-baseline/boaird.db BOAIRD_PORT=8080 \
  go run ./cmd/boaird serve

# in a second shell, same token
BOAIRD_LOCAL_AUTH_TOKEN=$(printf 'b%.0s' {1..64}) \
  go run ./tests/load -base http://127.0.0.1:8080 -tasks 120
```

`tests/load` creates projects and tasks and is therefore not run by CI; it is a
measurement tool, not runtime infrastructure.

## 9. Caveats

- Client and server share a host; a remote client adds network latency but does
  not change the server-side serialization point.
- No provider (LLM) calls are exercised. Agent turn latency dominates the real
  workload and is orders of magnitude larger than any value here.
- TLS and the Angular dashboard were not running, so CPU was available to the
  API process.
- Run-to-run variance is significant on a shared machine (~2× in write
  throughput). The conclusions use the **worst** observed run as the floor.
