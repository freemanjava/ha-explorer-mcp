# Development Method

How work is planned, chosen, executed and recorded in this project. Copied from
the `devflow` skill; adapt the project-specific parts (commands, branch names,
phase list), keep the structure.

## The problem this solves

A long-lived project outgrows any single context window. Two failures follow:

1. **A session cannot tell what to do next**, so it guesses — picks the wrong
   task, redoes finished work, or starts something blocked.
2. **Status drifts from reality**, because the same fact is written by hand in
   several places and one copy is always stale.

Everything below exists to prevent those two things specifically.

## Files and their jobs

| file | job | read | grows |
|---|---|---|---|
| `product-and-architecture.md` | what is built, why, what is settled | when planning | rarely |
| `NEXT.md` | what to do next, and current state | every session | **never** — rewritten in place |
| `phases/NN-*.md` | tasks, DoD, decision records | the one you need | slowly |
| `journal/NN-*.md` | what actually happened | rarely, on purpose | forever |
| `FINDINGS.md` | things discovered that change the plan | when triaging | drains into tasks |
| `CLAUDE.md` | engineering standards, commands, git rules | every session | rarely |

The critical boundary is between `NEXT.md` and `journal/`. Everything a session
needs at startup is bounded and cheap to read; everything that accumulates is
somewhere a session never has to open. Do not merge them back together — that
merge is exactly what makes a plan file grow to thousands of lines and start
getting truncated, taking the pointer with it.

## Three levels, each derived from the one above

- **Brief** (`docs/product-and-architecture.md`) — what is being built and why,
  the domain rules the software does not get to choose, the settled
  architectural decisions, the constraints, and a coarse roadmap. Written once,
  revised rarely, deliberately.
- **Phase files** — the roadmap's slices broken into checkboxes, each with a
  Definition of Done stating what must be asserted. A phase's Goal restates what
  its slice makes true; its Design Notes inherit the brief's decisions.
- **Tasks** — one checkbox, one reviewable change, one branch, one cycle.

Nothing is invented at a lower level. A phase that the brief does not justify is
a phase nobody decided to build; a task whose DoD cannot be traced to a phase
goal is scope that arrived by accident. When a lower level needs something the
level above never settled, that is a finding — take it up, don't fill it in.

Execution order is a separate thing from phase numbering, and it lives in
`NEXT.md`'s queue. Dependencies decide order; numbers do not.

## The pointer

`NEXT.md` opens with one line naming the active task, its phase file and its
model. That line is the single source of "what to pick next".

Advancing it is **part of finishing a task**, not cleanup afterwards. A task is
closed only when all four happen together:

1. the checkbox is ticked in the phase file,
2. the pointer moves to the next queued task,
3. status is recomputed,
4. a journal entry is appended.

Do fewer than four and the plan lies to the next session.

## Definition of Done

**Global** (every task, adapt per project): code compiles · new behavior has
tests · the build command is green · non-obvious public types documented ·
project-specific invariants respected.

**Per task**: the phase file states what to assert. Write that before writing
the implementation. A task whose DoD cannot be phrased as an assertion is too
vague to start — split or sharpen it first.

## Model selection

Default to the cheaper capable model. Most tasks are implementation against a
clear DoD, which is what the planning step exists to produce.

Use the stronger model for work dominated by *judgment* rather than
spec-following: foundational data models, concurrency designs, anything whose
mistakes are expensive to unwind. Mark those tasks 🧠.

Two consequences of the 🧠 flag:

- **Design on the strong model, implement on the cheap one.** Settling the design
  while the heavier model is loaded is what turns the rest into spec-following.
  Write the decision down; that is the artifact that makes the switch safe.
- **A task flagged `needs-decision` is not available work.** It waits on the
  owner. No model may choose the answer — ask, record, then implement.

A third kind of block sits beside those two, and it is the one most often missed:
a task can rest on **a fact nobody has established**. That is not the owner's to
decide — asking them invites a sincere, confident, unverified answer, and a
roadmap derived from one of those is wrong in a way careful implementation never
repairs. `needs-verify` sends it to observation instead: call the endpoint, run
the query, watch the thing. Gathering evidence is legwork and runs on the default
model; interpreting evidence that contradicts itself or overturns the question is
judgment and wants the stronger one.

The valuable outcome of verifying is often not an answer but a **better
question** — the premise turns out to be false, and the boxes written on it need
re-deriving rather than patching. Which is why verification belongs before
planning, not after it.

**The model is derived from the flag, never written twice:** 🧠 on a task line
means the stronger model, its absence means the default. `NEXT.md` names both
models once, in project facts, and displays the resulting choice on the pointer
and in the queue so a session sees it without opening the phase file.

Flag the task when you *write* it, not when you queue it. Whether work turns on
judgment is knowable at planning time, and deciding it then is what stops
"start it and see" on the wrong model.

A session that finds itself on the wrong model **stops and says so** rather than
proceeding — that gate is part of the loop, not a nicety.

## One task per cycle

Take one checkbox. Branch for it. Test first. Implement. Green build. Observe it
run if the change is behavioral. Close it atomically. Stop.

If the task turns out bigger than its box, finish what the box specifies and log
the rest as a finding. Silently widening scope produces diffs nobody can review
and status nobody can trust.

## When reality breaks the plan

Plans are wrong in predictable ways: an API behaves differently than documented,
a fix turns out too broad, two documents disagree. That is normal, and the flow
has a place for it rather than treating it as an interruption.

Log it in `FINDINGS.md` with an impact assessment and one of four triage states —
`blocks-active`, `queue-next`, `defer`, `wont-fix`. Then keep going. Findings
become tasks through planning, not by being fixed opportunistically inside an
unrelated change.

Nothing gets "reopened". A phase with new boxes simply has open boxes again, and
derived status reflects it with no bookkeeping.

## Derived status

Counts and phase status are computed from the checkboxes, never written by hand
in two places. Any fact stored twice will drift; this one drifted repeatedly
before it was derived.

## Git

- One branch per task, named for its id, cut from the base branch.
- **The agent does not commit, merge or push.** Its work ends when the tree is
  changed, the build is green, boxes are ticked and the pointer has moved. The
  owner reviews and commits.
- Code and its plan updates land together, so a commit never leaves status
  disagreeing with reality.

## Journal

At most five lines per task. The line that matters is the **surprise** — the
environment quirk, the endpoint that ignores its own parameters, the test that
had to be shaped oddly. What changed is in the diff; why it is designed that way
is in the phase file's decision record. Only the surprise is unrecoverable
anywhere else.

`NEXT.md` shows the last five entries, one line each. The rest lives in
`journal/` and is read on demand.
