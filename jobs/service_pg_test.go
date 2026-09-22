//go:build postgres && json1 && fts5

package jobs

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"mahresources/internal/testpgutil"
	"mahresources/models"
)

var pgContainer *testpgutil.Container

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	pgContainer, err = testpgutil.StartContainer(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	pgContainer.Stop(ctx)
	os.Exit(code)
}

func newPGDeps(t *testing.T) Deps {
	t.Helper()
	db := pgContainer.CreateTestDB(t)
	if err := db.AutoMigrate(
		&models.Job{}, &models.JobEvent{}, &models.JobEventSequence{}, &models.JobLink{},
	); err != nil {
		t.Fatalf("migrate job core: %v", err)
	}
	return Deps{DB: db}
}

// TestJobPublishOrdersOutOfOrderCommitsPG is the regression the publisher exists
// for, and only PostgreSQL can show it: SQLite has a single writer, so two write
// transactions can never commit in the opposite order from the order they
// recorded their rows.
//
// Transaction A records an event first and commits last; transaction B records
// and commits while A is still open. If delivery sequences were allocated where
// the event is recorded — a database sequence, a counter bumped inside the
// domain transaction — A would take the lower value and be committed later, and
// a subscriber whose cursor had already consumed B would never be told about A.
// Allocating on a separate transaction that runs after each commit is what makes
// the sequence describe durability order instead.
func TestJobPublishOrdersOutOfOrderCommitsPG(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()

	txA := deps.DB.Begin()
	if txA.Error != nil {
		t.Fatalf("begin A: %v", txA.Error)
	}
	jobA, err := svc.Accept(Deps{DB: txA}, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "ui",
	})
	if err != nil {
		t.Fatalf("accept A: %v", err)
	}

	txB := deps.DB.Begin()
	if txB.Error != nil {
		t.Fatalf("begin B: %v", txB.Error)
	}
	jobB, err := svc.Accept(Deps{DB: txB}, Acceptance{
		Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "ui",
	})
	if err != nil {
		t.Fatalf("accept B: %v", err)
	}

	// B commits first and is published. A is still open, so the publisher cannot
	// see it — it is not a committed fact yet.
	if committed := txB.Commit(); committed.Error != nil {
		t.Fatalf("commit B: %v", committed.Error)
	}
	published, err := svc.PublishPendingEvents(deps, DefaultPublishBatch)
	if err != nil {
		t.Fatalf("publish after B: %v", err)
	}
	if published != 1 {
		t.Fatalf("published %d events while A was uncommitted, want only B's", published)
	}

	eventB := jobEvents(t, deps, jobB.ID)[0]
	if eventB.DeliverySequence == nil {
		t.Fatal("B's committed event has no delivery sequence")
	}
	cursor := *eventB.DeliverySequence

	// A commits now — after the subscriber has already consumed B.
	if committed := txA.Commit(); committed.Error != nil {
		t.Fatalf("commit A: %v", committed.Error)
	}
	if _, err := svc.PublishPendingEvents(deps, DefaultPublishBatch); err != nil {
		t.Fatalf("publish after A: %v", err)
	}

	eventA := jobEvents(t, deps, jobA.ID)[0]
	if eventA.DeliverySequence == nil {
		t.Fatal("A's later commit was never published")
	}
	if *eventA.DeliverySequence <= cursor {
		t.Fatalf("A committed after B but took delivery sequence %d, at or below B's %d: "+
			"a subscriber that consumed B would never see A", *eventA.DeliverySequence, cursor)
	}

	var catchUp []models.JobEvent
	if err := deps.DB.Where("delivery_sequence > ?", cursor).
		Order("delivery_sequence").Find(&catchUp).Error; err != nil {
		t.Fatalf("catch-up: %v", err)
	}
	if len(catchUp) != 1 || catchUp[0].JobID != jobA.ID {
		t.Fatalf("catch-up from B's cursor returned %d events, want exactly A's", len(catchUp))
	}
}

// TestJobPublishSerializesConcurrentPublishersPG proves the allocator row does
// what it is for: several publishers running at once hand every committed event
// exactly one delivery sequence and assign no value twice. The guarded update
// claims each event, and the row lock is what keeps two publishers from taking
// the same counter value.
func TestJobPublishSerializesConcurrentPublishersPG(t *testing.T) {
	deps := newPGDeps(t)
	svc := NewService()

	const jobs = 12
	for i := 0; i < jobs; i++ {
		if _, err := svc.Accept(deps, Acceptance{
			Kind: "group-export", KindVersion: 1, State: StateQueued, Origin: "ui",
		}); err != nil {
			t.Fatalf("accept %d: %v", i, err)
		}
	}

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.PublishPendingEvents(deps, DefaultPublishBatch)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent publisher %d: %v", i, err)
		}
	}

	var sequences []*uint64
	if err := deps.DB.Model(&models.JobEvent{}).Order("id").Pluck("delivery_sequence", &sequences).Error; err != nil {
		t.Fatalf("read sequences: %v", err)
	}
	if len(sequences) != jobs {
		t.Fatalf("%d events, want %d", len(sequences), jobs)
	}
	seen := map[uint64]bool{}
	for i, sequence := range sequences {
		if sequence == nil {
			t.Fatalf("event %d was never published", i)
		}
		if seen[*sequence] {
			t.Fatalf("delivery sequence %d was assigned twice", *sequence)
		}
		seen[*sequence] = true
	}
}
