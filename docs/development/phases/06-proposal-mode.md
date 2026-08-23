# Phase 06 — Proposal Mode

**Milestone:** M3 (roadmap "Phase 3 — Proposal") · **Target version:** v1.1

> ⚠️ **Gated phase. Do not start on the strength of this file.** The architecture
> doc (§23 step 12, §26) is explicit: the proposal/write subsystem is designed
> only *after* v1 usage data exists. Opening this phase early is the exact
> scope-creep the observer/admin split exists to prevent.

## Goal

The agent can produce a **structured proposal** — a change to an automation,
config or dashboard, with its reasoning, its risk and a validated diff — that a
human reads and decides on. Nothing is applied. The proposal is an artifact, not
an action.

Critically, this phase must not weaken Phase 01's guarantee. A proposal is
generated from data already readable; it does not require a single new write
capability, and the binary still contains no `HAWriter`.

## Depends On

Phase 05, **plus** real usage evidence from a running v1 installation, **plus**
an explicit owner decision to open this milestone.

## Add Under

*To be decided when the phase opens.* If a proposal needs validation against HA,
strongly prefer a read-only validation API over anything that writes.

## Design Notes

- **A proposal is inert data.** No code path turns a proposal into an
  application. That step is Phase 07, in a separate process (ADR-011).
- **Prompt injection matters more here than anywhere** (threat T2). A proposal
  is derived from HA data that may contain adversarial strings, and it is
  presented to a human as a recommendation. The path from untrusted attribute to
  proposal text needs its own review.
- Every proposal carries the evidence it was derived from, so the reviewer can
  check the reasoning rather than trust it.

## Tasks

*Not planned.* This phase is deliberately empty until the gate above is passed.
A task appearing here without that decision is scope creep and should be filed
as a finding instead.

## Decisions

- [ ] **`needs-decision` — Whether to open this milestone at all**
  Requires: v1 in real use, evidence about which changes the owner actually
  wants proposed, and a fresh security review. The doc treats write capability
  as a new security boundary, not an increment.

## Phase Definition of Done

- Proposals are generated, validated and diffed; none is applied.
- The observer binary still contains no writer implementation.
- `make check` is green.
