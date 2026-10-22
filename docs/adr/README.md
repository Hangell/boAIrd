# Architecture Decision Records

An Architecture Decision Record (ADR) captures a decision that is expensive to
reverse, together with the evidence behind it and the conditions under which it
should be revisited.

| ADR | Title | Status |
| --- | --- | --- |
| [0001](0001-gate-distributed-workers.md) | Gate distributed workers on demonstrated need | Proposed — awaiting maintainer approval |

Conventions:

- One decision per record, written in present tense, in English.
- Status is one of `Proposed`, `Accepted`, `Superseded by ADR NNNN`, or `Rejected`.
- An accepted decision is only reversed by a new ADR that references the record
  it replaces.
- Measurements an ADR depends on live under [`docs/measurements/`](../measurements).
