// Package policy enforces the envelope every MCP invocation runs inside: the
// query budget it may spend, the rate at which invocations may arrive, and
// (from P2-02 on) the privacy classification applied to what they return.
//
// Nothing here fetches, formats or masks. It decides; other packages act on
// the decision.
package policy

import "errors"

var (
	// ErrBudgetExceeded indicates a query would spend more of an invocation's
	// budget than remains, on one named dimension. It is always an explicit
	// result — never a silently shortened answer (doc §10, phase 02 design
	// notes): the agent must be able to tell "here is everything" from "here
	// is what fit". Errors carrying it are *BudgetError.
	ErrBudgetExceeded = errors.New("policy: query budget exceeded")

	// ErrRateLimited indicates invocations arrived faster than the server is
	// willing to serve them — the request-storm mitigation for threat T1
	// (Appendix B, "MCP client requests maximum page repeatedly"). It bounds
	// arrivals across invocations; a QueryBudget bounds one invocation's
	// spend, and neither substitutes for the other. Errors carrying it are
	// *RateLimitError.
	ErrRateLimited = errors.New("policy: rate limited")

	// ErrPolicyDenied indicates the privacy profile refuses to serve a
	// request at all, rather than serving it masked. It must stay
	// distinguishable all the way to the MCP response from "absent"
	// (ErrNotFound) and "cannot check" (ErrUnsupported): a refusal is a
	// decision this server made, not a fact about the installation, and an
	// empty list would claim the opposite (CLAUDE.md rule 7). Errors carrying
	// it are *PolicyError.
	ErrPolicyDenied = errors.New("policy: denied by privacy profile")
)
