# Phase 07 — Controlled Change (Admin)

**Milestone:** M4 (roadmap "Phase 4 — Controlled Change") · **Target version:** v2.0

> ⚠️ **Gated phase, separate security review, and by preference a separate
> process.** ADR-011: an Admin capability must not be an extra flag on the
> Observer. The security boundary of "this server cannot write" is worth more
> than the convenience of one binary, and it is lost the moment a write path is
> linked in.

## Goal

An approved proposal can be applied through the doc's Appendix C pipeline:
approval → backup/snapshot → narrow typed apply → verify → rollback on failed
verification. High-impact operations (system update, restart, deleting an
integration or entity, network changes, backup restore, Supervisor admin) get
their own confirmation model and never become generic tool capabilities.

## Depends On

Phase 06, an explicit owner decision, and a security review that treats this as
a new system rather than a new feature.

## Add Under

*A separate binary and a separate App.* Sharing `internal/ha`'s reader is fine;
adding a writer to the observer's binary is not.

## Design Notes

- **Observer and Admin are different processes with different permissions**
  (ADR-011). The Observer's App manifest does not change when this ships.
- **Verification is part of apply, not a follow-up.** An apply that cannot be
  verified rolls back.
- **Approval is a human's, per change, on the specific diff** — never a standing
  grant to a class of changes, and never inferable from a previous approval.

## Tasks

*Not planned.* See the gate above.

## Decisions

- [ ] **`needs-decision` — Whether an Admin capability is wanted at all**
  A read-only diagnostic server that is trusted may be worth more than a
  read-write one that is not. This is the owner's call, made with v1 experience
  in hand, and it is legitimate for the answer to be permanently "no".

## Phase Definition of Done

- Every applied change was individually approved, backed up, verified, and
  rolled back on failure.
- The Observer's security posture is provably unchanged by this phase.
- `make check` is green.
