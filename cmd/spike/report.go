package main

import (
	"fmt"
	"strings"
)

// report accumulates the probe's markdown output.
//
// It exists to own the one write error in this program that genuinely cannot
// happen: strings.Builder's Write methods are documented never to return a
// non-nil error. Centralising that discard here keeps the ~30 formatting call
// sites free of a check that can never fire, and — more usefully — means any
// unhandled error still flagged in this package is a real one.
type report struct {
	b strings.Builder
}

func (r *report) writef(format string, args ...any) {
	_, _ = fmt.Fprintf(&r.b, format, args...)
}

func (r *report) String() string {
	return r.b.String()
}
