// Package page is the single place that enforces the byte cap and emits
// cursor pagination for every list_* tool (doc §9, Appendix A.1).
//
// It decides nothing about privacy or budget — internal/policy already
// charged and classified whatever internal/redact already masked. This
// package only decides where one response ends: at a record boundary, never
// mid-structure, so the agent can always tell "here is everything" from
// "here is what fit" (CLAUDE.md rule 7, phase 02 design notes).
package page

import (
	"encoding/base64"
	"errors"
	"sort"
	"strings"
)

// ErrInvalidCursor is returned when a cursor cannot be decoded — malformed,
// forged, or carried over from a different response shape. Fail closed
// (CLAUDE.md rule 3): an unrecognized cursor is refused, not silently
// treated as "start from the beginning", which would look like a cursor that
// simply expired rather than one that was wrong.
var ErrInvalidCursor = errors.New("page: invalid cursor")

// Default and maximum page sizes (doc §9): every list_* tool honors these,
// never a caller-supplied limit beyond MaxLimit.
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// cursorPrefix versions the cursor encoding, so a future format change can
// tell its own cursors from an old client's without guessing.
const cursorPrefix = "c1:"

// ResolveLimit clamps a caller's requested limit into [1, MaxLimit],
// substituting DefaultLimit for a non-positive request. clamped reports
// whether the request exceeded MaxLimit and was cut down — the response
// must say so explicitly, never silently honor a bigger page (P2-04 DoD).
func ResolveLimit(requested int) (limit int, clamped bool) {
	if requested <= 0 {
		return DefaultLimit, false
	}
	if requested > MaxLimit {
		return MaxLimit, true
	}
	return requested, false
}

// EncodeCursor turns a record's sort key into an opaque cursor. An empty key
// encodes to an empty cursor — "start from the beginning" — so a zero Result
// never emits a cursor a client could mistake for "resume here".
func EncodeCursor(key string) string {
	if key == "" {
		return ""
	}
	return cursorPrefix + base64.RawURLEncoding.EncodeToString([]byte(key))
}

// DecodeCursor recovers the sort key an EncodeCursor call produced. An empty
// cursor decodes to an empty key (start from the beginning); anything else
// that doesn't carry cursorPrefix, or fails to decode, is ErrInvalidCursor.
func DecodeCursor(cursor string) (key string, err error) {
	if cursor == "" {
		return "", nil
	}
	rest, ok := strings.CutPrefix(cursor, cursorPrefix)
	if !ok {
		return "", ErrInvalidCursor
	}
	raw, decErr := base64.RawURLEncoding.DecodeString(rest)
	if decErr != nil {
		return "", ErrInvalidCursor
	}
	return string(raw), nil
}

// Result is one page cut from a larger list.
type Result[T any] struct {
	// Items is this page's records, in the same order they were given.
	Items []T
	// NextCursor resumes after the last item in Items. Empty means the list
	// had nothing left after this page.
	NextCursor string
	// Truncated is true when items remain beyond this page — by the
	// resolved limit or by MaxBytes, whichever cut first. A caller renders
	// this explicitly rather than letting a full-looking page pass as
	// complete (CLAUDE.md rule 7).
	Truncated bool
	// LimitClamped is true when the caller's requested limit exceeded
	// MaxLimit and was cut down to it (P2-04 DoD).
	LimitClamped bool
}

// Paginate slices one page out of items, which must already be sorted
// ascending by keyOf and carry keys unique within the list — the same
// contract every list_* tool's underlying query already provides (a stable
// registry order). Cutting happens at the first of three boundaries: the
// resolved limit, byteSize's cumulative total against maxBytes, or the end
// of the list.
//
// The cut always keeps whole records: a record already in progress is never
// split, so a single record larger than maxBytes is still returned whole
// (Appendix B, "response approaches max bytes after aggregation") rather
// than producing an empty, unusable page.
//
// Resuming from a cursor searches for the first key strictly greater than
// the cursor's — not the cursor's own index — so a record that was removed,
// or one inserted or removed elsewhere in the list, changes only how much
// the next page returns, never causes a duplicate or a skipped record over
// what the current key ordering agrees is "after" the cursor.
func Paginate[T any](items []T, cursor string, limit int, maxBytes int64, keyOf func(T) string, byteSize func(T) int64) (Result[T], error) {
	resolvedLimit, clamped := ResolveLimit(limit)

	startKey, err := DecodeCursor(cursor)
	if err != nil {
		return Result[T]{}, err
	}

	start := 0
	if startKey != "" {
		start = sort.Search(len(items), func(i int) bool { return keyOf(items[i]) > startKey })
	}

	out := make([]T, 0, min(resolvedLimit, len(items)-start))
	var used int64
	end := start
	for i := start; i < len(items); i++ {
		if len(out) >= resolvedLimit {
			break
		}
		sz := byteSize(items[i])
		if used+sz > maxBytes && len(out) > 0 {
			break
		}
		out = append(out, items[i])
		used += sz
		end = i + 1
	}

	truncated := end < len(items)
	next := ""
	if truncated {
		next = EncodeCursor(keyOf(items[end-1]))
	}

	return Result[T]{
		Items:        out,
		NextCursor:   next,
		Truncated:    truncated,
		LimitClamped: clamped,
	}, nil
}
