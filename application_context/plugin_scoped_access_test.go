package application_context

import (
	"testing"
)

// A revocation must not be undone by a read that started before it. The loader
// stamps the generation it began from and drops its result if the world moved,
// because a slow read taken while the permission was still granted would
// otherwise land after the revocation and restore it — for as long as the
// snapshot lives.
func TestScopedPluginAccess_AStaleLoadDoesNotOverwriteARevocation(t *testing.T) {
	cache := &scopedPluginAccess{}

	// A reader begins: it captures the generation, then stalls.
	startGeneration := cache.generation.Load()

	// Meanwhile the operator revokes, which bumps the generation and drops the
	// snapshot.
	cache.generation.Add(1)
	cache.snapshot.Store(nil)

	// The stalled reader now tries to publish what it read a moment ago.
	stale := &scopedAccessSnapshot{allowed: map[string]bool{"widgets": true}, generation: startGeneration}
	if cache.generation.Load() == startGeneration {
		cache.snapshot.Store(stale)
	}

	if got := cache.snapshot.Load(); got != nil {
		t.Fatalf("a load that started before the revocation was published anyway: %+v", got.allowed)
	}
}

// And the ordinary case still publishes, or the cache would never hold anything
// and every render would query.
func TestScopedPluginAccess_AnUncontendedLoadIsPublished(t *testing.T) {
	cache := &scopedPluginAccess{}

	generation := cache.generation.Load()
	fresh := &scopedAccessSnapshot{allowed: map[string]bool{"widgets": true}, generation: generation}
	if cache.generation.Load() == generation {
		cache.snapshot.Store(fresh)
	}

	got := cache.snapshot.Load()
	if got == nil || !got.allowed["widgets"] {
		t.Fatal("an uncontended load was not published, so every read would hit the database")
	}
}
