package ha

import "context"

// RequestMeter is charged once for every request that leaves the process. It
// is defined here, narrow, so that internal/ha does not depend on the budget's
// concrete type: *policy.QueryBudget satisfies it structurally (D-08-16).
type RequestMeter interface {
	ChargeHARequests(n int) error
}

type requestMeterKey struct{}

// WithRequestMeter attaches m to ctx. Every request issued under the returned
// context is charged to m at the wire seam.
func WithRequestMeter(ctx context.Context, m RequestMeter) context.Context {
	return context.WithValue(ctx, requestMeterKey{}, m)
}

// chargeRequest charges one upstream request to the meter in ctx. No meter is
// a no-op — probes and unit tests run without a budget. A refusal is returned
// unchanged so errors.Is still finds the budget's sentinel; the caller must
// not write anything after it.
func chargeRequest(ctx context.Context) error {
	m, ok := ctx.Value(requestMeterKey{}).(RequestMeter)
	if !ok || m == nil {
		return nil
	}
	return m.ChargeHARequests(1)
}
