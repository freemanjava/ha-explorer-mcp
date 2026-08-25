package ha

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeCaller stands in for *Manager: it answers whatever Command it is asked
// for from a responses table, with no real WebSocket involved, and counts
// how many times each command was actually issued so a test can assert on
// upstream call volume directly instead of on timing.
type fakeCaller struct {
	mu        sync.Mutex
	responses map[string]json.RawMessage
	calls     map[string]int
	block     chan struct{} // if non-nil, Call waits on it before answering
}

func newFakeCaller() *fakeCaller {
	return &fakeCaller{
		responses: make(map[string]json.RawMessage),
		calls:     make(map[string]int),
	}
}

func (f *fakeCaller) set(command string, result json.RawMessage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses[command] = result
}

func (f *fakeCaller) callCount(command string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[command]
}

func (f *fakeCaller) Call(ctx context.Context, cmd Command) (json.RawMessage, error) {
	f.mu.Lock()
	f.calls[cmd.CommandType()]++
	result := f.responses[cmd.CommandType()]
	block := f.block
	f.mu.Unlock()

	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return result, nil
}

func entityRegistryJSON(name string) json.RawMessage {
	raw, _ := json.Marshal([]map[string]any{
		{"entity_id": "light.kitchen", "platform": "hue", "name": name},
	})
	return raw
}

func TestRegistryCache_ServedValue_ExposesObservationTime(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandEntityRegistryList, entityRegistryJSON("Kitchen"))
	cache := NewRegistryCache(fc)

	before := time.Now()
	entities, observedAt, err := cache.Entities(context.Background())
	after := time.Now()
	if err != nil {
		t.Fatalf("Entities: %v", err)
	}
	if len(entities) != 1 || entities[0].Name != "Kitchen" {
		t.Fatalf("unexpected entities: %+v", entities)
	}
	if observedAt.Before(before) || observedAt.After(after) {
		t.Fatalf("observedAt %v not within [%v, %v]", observedAt, before, after)
	}
}

func TestRegistryCache_WithinTTL_ServesCachedValueWithoutRefetch(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandEntityRegistryList, entityRegistryJSON("Kitchen"))
	cache := NewRegistryCache(fc)

	fakeNow := time.Now()
	cache.entities.now = func() time.Time { return fakeNow }

	if _, _, err := cache.Entities(context.Background()); err != nil {
		t.Fatalf("first Entities: %v", err)
	}
	if _, _, err := cache.Entities(context.Background()); err != nil {
		t.Fatalf("second Entities: %v", err)
	}
	if got := fc.callCount(CommandEntityRegistryList); got != 1 {
		t.Fatalf("expected 1 upstream call within TTL, got %d", got)
	}
}

func TestRegistryCache_ExpiredTTL_Refetches(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandEntityRegistryList, entityRegistryJSON("Kitchen"))
	cache := NewRegistryCache(fc)

	fakeNow := time.Now()
	cache.entities.now = func() time.Time { return fakeNow }

	if _, _, err := cache.Entities(context.Background()); err != nil {
		t.Fatalf("first Entities: %v", err)
	}

	fakeNow = fakeNow.Add(entityRegistryTTL + time.Millisecond)
	if _, _, err := cache.Entities(context.Background()); err != nil {
		t.Fatalf("second Entities: %v", err)
	}
	if got := fc.callCount(CommandEntityRegistryList); got != 2 {
		t.Fatalf("expected 2 upstream calls across expiry, got %d", got)
	}
}

// TestRegistryCache_RenameAfterExpiry_Reflected is the stale-registry failure
// mode from the architecture doc's Appendix B ("cache contains old registry
// data after entity rename/move"): a rename made upstream between two calls
// must show up once the cached copy expires, not be served stale forever.
func TestRegistryCache_RenameAfterExpiry_Reflected(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandEntityRegistryList, entityRegistryJSON("Kitchen"))
	cache := NewRegistryCache(fc)

	fakeNow := time.Now()
	cache.entities.now = func() time.Time { return fakeNow }

	entities, _, err := cache.Entities(context.Background())
	if err != nil {
		t.Fatalf("first Entities: %v", err)
	}
	if entities[0].Name != "Kitchen" {
		t.Fatalf("expected initial name Kitchen, got %q", entities[0].Name)
	}

	// The entity is renamed upstream while still within the TTL window.
	fc.set(CommandEntityRegistryList, entityRegistryJSON("Kitchen Light"))

	entities, _, err = cache.Entities(context.Background())
	if err != nil {
		t.Fatalf("second Entities (within TTL): %v", err)
	}
	if entities[0].Name != "Kitchen" {
		t.Fatalf("expected stale name Kitchen still served within TTL, got %q", entities[0].Name)
	}

	fakeNow = fakeNow.Add(entityRegistryTTL + time.Millisecond)
	entities, _, err = cache.Entities(context.Background())
	if err != nil {
		t.Fatalf("third Entities (after TTL): %v", err)
	}
	if entities[0].Name != "Kitchen Light" {
		t.Fatalf("expected renamed value after expiry, got %q", entities[0].Name)
	}
}

func TestRegistryCache_ConcurrentReadersPastExpiry_SingleUpstreamFetch(t *testing.T) {
	fc := newFakeCaller()
	fc.set(CommandEntityRegistryList, entityRegistryJSON("Kitchen"))
	fc.block = make(chan struct{})
	cache := NewRegistryCache(fc)

	const readers = 20
	var wg sync.WaitGroup
	var succeeded atomic.Int32
	wg.Add(readers)
	for range readers {
		go func() {
			defer wg.Done()
			if _, _, err := cache.Entities(context.Background()); err == nil {
				succeeded.Add(1)
			}
		}()
	}

	// Give every goroutine a chance to reach the blocked fetch (or the
	// single-flight wait) before releasing it.
	time.Sleep(50 * time.Millisecond)
	close(fc.block)
	wg.Wait()

	if got := fc.callCount(CommandEntityRegistryList); got != 1 {
		t.Fatalf("expected exactly 1 upstream fetch for %d concurrent readers, got %d", readers, got)
	}
	if got := succeeded.Load(); got != readers {
		t.Fatalf("expected all %d readers to succeed, got %d", readers, got)
	}
}
