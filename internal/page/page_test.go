package page

import (
	"strconv"
	"testing"
)

type record struct {
	id   string
	size int64
}

func recordsFrom(ids ...int) []record {
	out := make([]record, len(ids))
	for i, id := range ids {
		out[i] = record{id: fixedWidth(id), size: 10}
	}
	return out
}

// fixedWidth keeps string ordering equal to numeric ordering for small ids.
func fixedWidth(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}

func keyOf(r record) string { return r.id }
func sizeOf(r record) int64 { return r.size }
func idsOf(rs []record) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.id
	}
	return out
}

func TestResolveLimit_DefaultOnNonPositive(t *testing.T) {
	if l, clamped := ResolveLimit(0); l != DefaultLimit || clamped {
		t.Fatalf("ResolveLimit(0) = %d, %v; want %d, false", l, clamped, DefaultLimit)
	}
	if l, clamped := ResolveLimit(-5); l != DefaultLimit || clamped {
		t.Fatalf("ResolveLimit(-5) = %d, %v; want %d, false", l, clamped, DefaultLimit)
	}
}

func TestResolveLimit_WithinRangePassesThrough(t *testing.T) {
	if l, clamped := ResolveLimit(75); l != 75 || clamped {
		t.Fatalf("ResolveLimit(75) = %d, %v; want 75, false", l, clamped)
	}
}

func TestResolveLimit_AboveMaxIsClampedExplicitly(t *testing.T) {
	l, clamped := ResolveLimit(9999)
	if l != MaxLimit {
		t.Fatalf("ResolveLimit(9999) limit = %d, want %d", l, MaxLimit)
	}
	if !clamped {
		t.Fatal("ResolveLimit(9999) clamped = false, want true — a limit over 200 must not be silently honored")
	}
}

func TestCursor_RoundTrips(t *testing.T) {
	c := EncodeCursor("0007")
	key, err := DecodeCursor(c)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if key != "0007" {
		t.Fatalf("DecodeCursor = %q, want 0007", key)
	}
}

func TestCursor_EmptyIsStartOfList(t *testing.T) {
	key, err := DecodeCursor("")
	if err != nil || key != "" {
		t.Fatalf("DecodeCursor(\"\") = %q, %v; want \"\", nil", key, err)
	}
}

func TestCursor_MalformedIsRejected(t *testing.T) {
	cases := []string{"not-a-cursor", "c1:not-base64!!!", "c2:AAAA"}
	for _, c := range cases {
		if _, err := DecodeCursor(c); err != ErrInvalidCursor {
			t.Errorf("DecodeCursor(%q) err = %v, want ErrInvalidCursor", c, err)
		}
	}
}

func TestPaginate_FirstPageDefaultLimit(t *testing.T) {
	items := recordsFrom(seq(1, 60)...)
	res, err := Paginate(items, "", 0, 1<<20, keyOf, sizeOf)
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if len(res.Items) != DefaultLimit {
		t.Fatalf("got %d items, want default limit %d", len(res.Items), DefaultLimit)
	}
	if !res.Truncated || res.NextCursor == "" {
		t.Fatalf("Truncated=%v NextCursor=%q; want truncated with a usable cursor", res.Truncated, res.NextCursor)
	}
}

func TestPaginate_CursorResumesWithoutOverlap(t *testing.T) {
	items := recordsFrom(seq(1, 10)...)
	first, err := Paginate(items, "", 3, 1<<20, keyOf, sizeOf)
	if err != nil {
		t.Fatalf("Paginate page 1: %v", err)
	}
	second, err := Paginate(items, first.NextCursor, 3, 1<<20, keyOf, sizeOf)
	if err != nil {
		t.Fatalf("Paginate page 2: %v", err)
	}
	if got, want := idsOf(first.Items), []string{"0001", "0002", "0003"}; !equal(got, want) {
		t.Fatalf("page 1 = %v, want %v", got, want)
	}
	if got, want := idsOf(second.Items), []string{"0004", "0005", "0006"}; !equal(got, want) {
		t.Fatalf("page 2 = %v, want %v", got, want)
	}
}

func TestPaginate_LastPageNotTruncated(t *testing.T) {
	items := recordsFrom(seq(1, 5)...)
	res, err := Paginate(items, "", 10, 1<<20, keyOf, sizeOf)
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if res.Truncated || res.NextCursor != "" {
		t.Fatalf("Truncated=%v NextCursor=%q; want a full page reporting no more data", res.Truncated, res.NextCursor)
	}
	if len(res.Items) != 5 {
		t.Fatalf("got %d items, want 5", len(res.Items))
	}
}

func TestPaginate_ByteCapCutsAtRecordBoundary(t *testing.T) {
	items := recordsFrom(seq(1, 10)...) // each record is size 10
	res, err := Paginate(items, "", 200, 35, keyOf, sizeOf)
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	// 35 bytes / 10 per record fits 3 whole records, never a partial one.
	if len(res.Items) != 3 {
		t.Fatalf("got %d items, want 3 whole records under the byte cap", len(res.Items))
	}
	if !res.Truncated || res.NextCursor == "" {
		t.Fatalf("Truncated=%v NextCursor=%q; want truncated with a usable cursor", res.Truncated, res.NextCursor)
	}
}

func TestPaginate_SingleOversizedRecordStillReturnsWhole(t *testing.T) {
	items := []record{{id: "0001", size: 1000}, {id: "0002", size: 10}}
	res, err := Paginate(items, "", 200, 50, keyOf, sizeOf)
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].id != "0001" {
		t.Fatalf("got %v, want the single oversized record returned whole, not an empty page", res.Items)
	}
	if !res.Truncated {
		t.Fatal("Truncated = false, want true — the second record did not fit")
	}
}

func TestPaginate_ChangedListNoDuplicatesOrSkips(t *testing.T) {
	original := recordsFrom(seq(1, 6)...)
	first, err := Paginate(original, "", 3, 1<<20, keyOf, sizeOf)
	if err != nil {
		t.Fatalf("Paginate page 1: %v", err)
	}
	if got, want := idsOf(first.Items), []string{"0001", "0002", "0003"}; !equal(got, want) {
		t.Fatalf("page 1 = %v, want %v", got, want)
	}

	// The underlying list changes between calls: the cursor's own record
	// (0003) is removed, and a new record (0007) is appended.
	changed := []record{
		{id: "0001", size: 10}, {id: "0002", size: 10},
		{id: "0004", size: 10}, {id: "0005", size: 10}, {id: "0006", size: 10}, {id: "0007", size: 10},
	}
	second, err := Paginate(changed, first.NextCursor, 3, 1<<20, keyOf, sizeOf)
	if err != nil {
		t.Fatalf("Paginate page 2 over changed list: %v", err)
	}
	if got, want := idsOf(second.Items), []string{"0004", "0005", "0006"}; !equal(got, want) {
		t.Fatalf("page 2 over changed list = %v, want %v (no duplicate of 0001-0003, no skip of 0004)", got, want)
	}
}

func TestPaginate_InvalidCursorRejected(t *testing.T) {
	items := recordsFrom(seq(1, 5)...)
	if _, err := Paginate(items, "garbage", 10, 1<<20, keyOf, sizeOf); err != ErrInvalidCursor {
		t.Fatalf("err = %v, want ErrInvalidCursor", err)
	}
}

func seq(from, to int) []int {
	out := make([]int, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
