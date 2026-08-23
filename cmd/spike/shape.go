package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// A shape is a JSON value described by its structure only: which types were
// observed, and for objects which keys, in how many of the merged samples.
//
// Phase 00's design note says a verify task records "a redacted sample
// response". A type skeleton is the strongest form of that redaction: the
// research file gets every field name and its nullability — which is what
// Phase 01's mapping needs — while no entity id, friendly name, coordinate or
// token from the owner's installation can reach the file at all.
type shape struct {
	types   map[string]bool
	fields  map[string]*field
	elem    *shape // merged shape of every array element
	elemLen int    // elements observed in the largest array merged here
	samples int    // values merged into this shape
}

type field struct {
	shape *shape
	seen  int // samples of the parent object in which this key was present
}

func newShape() *shape {
	return &shape{types: map[string]bool{}, fields: map[string]*field{}}
}

// shapeOf describes v without retaining any of its values.
func shapeOf(v any) *shape {
	s := newShape()
	s.merge(v)
	return s
}

func (s *shape) merge(v any) {
	s.samples++

	switch t := v.(type) {
	case nil:
		s.types["null"] = true
	case bool:
		s.types["bool"] = true
	case float64:
		s.types["number"] = true
	case string:
		s.types["string"] = true
	case []any:
		s.types["array"] = true
		if len(t) > s.elemLen {
			s.elemLen = len(t)
		}
		if s.elem == nil {
			s.elem = newShape()
		}
		for _, e := range t {
			s.elem.merge(e)
		}
	case map[string]any:
		s.types["object"] = true
		for k, e := range t {
			f, ok := s.fields[k]
			if !ok {
				f = &field{shape: newShape()}
				s.fields[k] = f
			}
			f.seen++
			f.shape.merge(e)
		}
	default:
		// json.Unmarshal into `any` produces nothing else; a new case here
		// would mean the decoder changed, and guessing would hide that.
		s.types[fmt.Sprintf("unknown(%T)", v)] = true
	}
}

// HA uses objects as maps keyed by an id in several payloads (notably
// config_entries_subentries, and every history and statistics result). There
// the keys are values from the installation, not schema, so emitting them would
// leak exactly what renderShape exists to withhold (F-9). These patterns are
// HA's own id formats — a ULID, a 32-char hex digest, and a `domain.object_id`
// entity id — and are deliberately narrow: a schema key like "should_expose"
// or "conversation" must never match.
//
// The entity-id pattern was added by P0-07: F-9's fix covered only the first
// two, so history/history_during_period, which keys its answer by entity id,
// re-leaked one on the first run of that task's probe.
var (
	ulidKey   = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
	hexKey    = regexp.MustCompile(`^[0-9a-f]{32}$`)
	entityKey = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z0-9_]+$`)
)

func isIDKey(k string) bool {
	return ulidKey.MatchString(k) || hexKey.MatchString(k) || entityKey.MatchString(k)
}

// mapKeyShape merges every id-keyed entry into one shape when the object is a
// map keyed by id, and returns nil when it is an ordinary schema object.
func (s *shape) mapKeyShape() *shape {
	if len(s.fields) == 0 {
		return nil
	}
	merged := newShape()
	for k, f := range s.fields {
		if !isIDKey(k) {
			return nil
		}
		merged.mergeShape(f.shape)
	}
	return merged
}

// mergeShape folds another shape into this one, used to collapse the entries
// of a map keyed by id into a single description of their common value.
func (s *shape) mergeShape(other *shape) {
	s.samples += other.samples
	for t := range other.types {
		s.types[t] = true
	}
	if other.elemLen > s.elemLen {
		s.elemLen = other.elemLen
	}
	if other.elem != nil {
		if s.elem == nil {
			s.elem = newShape()
		}
		s.elem.mergeShape(other.elem)
	}
	for k, f := range other.fields {
		existing, ok := s.fields[k]
		if !ok {
			existing = &field{shape: newShape()}
			s.fields[k] = existing
		}
		existing.seen += f.seen
		existing.shape.mergeShape(f.shape)
	}
}

func (s *shape) typeList() string {
	if len(s.types) == 0 {
		return "unknown"
	}
	ts := make([]string, 0, len(s.types))
	for t := range s.types {
		ts = append(ts, t)
	}
	sort.Strings(ts)
	return strings.Join(ts, "|")
}

// renderShape formats a shape as an indented tree, field names and types only.
func renderShape(s *shape) string {
	var r report
	s.render(&r, 0)
	return r.String()
}

func (s *shape) render(b *report, depth int) {
	indent := strings.Repeat("  ", depth)

	if s.types["array"] {
		b.writef("%sarray[%d]\n", indent, s.elemLen)
		if s.elem != nil {
			s.elem.render(b, depth+1)
		}
		return
	}

	if !s.types["object"] {
		b.writef("%s%s\n", indent, s.typeList())
		return
	}

	if m := s.mapKeyShape(); m != nil {
		b.writef("%s<id>: (map keyed by id, %d entries — keys withheld)\n", indent, len(s.fields))
		m.render(b, depth+1)
		return
	}

	keys := make([]string, 0, len(s.fields))
	for k := range s.fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		f := s.fields[k]
		child := f.shape

		// Presence is only interesting once several samples were merged; for a
		// single object every key is trivially present in 1/1.
		presence := ""
		if s.samples > 1 {
			presence = fmt.Sprintf("  (%d/%d)", f.seen, s.samples)
		}

		switch {
		case child.types["object"] && len(child.fields) > 0:
			b.writef("%s%s: object%s\n", indent, k, presence)
			child.render(b, depth+1)
		case child.types["array"]:
			b.writef("%s%s: array[%d]%s\n", indent, k, child.elemLen, presence)
			if child.elem != nil {
				child.elem.render(b, depth+1)
			}
		default:
			b.writef("%s%s: %s%s\n", indent, k, child.typeList(), presence)
		}
	}
}
