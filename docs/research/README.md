# Research

Dated evidence produced by `devflow verify`. Append-only, **stale by design**: a
file here records what was observed to be true on a date, against a named Home
Assistant version. It is never edited to stay current — a newer observation is a
newer file.

Name files `<YYYY-MM-DD>-<subject>.md` and state, at minimum: what was asked, how
it was observed (the exact command or request), the verbatim result (redacted),
the HA/Supervisor version it ran against, and what the result means for the plan.

Never put a token, a credential or unredacted private entity history in here.

Curated, maintained facts that are read *while implementing* live in
`docs/reference/` instead — keeping the two apart is what stops a dated snapshot
from being mistaken for a maintained fact.
