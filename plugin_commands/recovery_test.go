package plugin_commands

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type recoveryStore struct{ *dispatcherTestStore }

func (s *recoveryStore) NonterminalRuns() ([]RunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []RunRecord
	for _, run := range s.runs {
		if !RunStatusTerminal(run.Status) {
			result = append(result, run)
		}
	}
	return result, nil
}

type recoveryInspector struct {
	mu       sync.Mutex
	states   map[int][]GroupIdentity
	errs     map[int]error
	kills    []int
	killErr  error
	killDead bool
}

func (i *recoveryInspector) InspectGroup(pgid int, _ string) (GroupIdentity, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if err := i.errs[pgid]; err != nil {
		return GroupIdentity{}, err
	}
	sequence := i.states[pgid]
	if len(sequence) == 0 {
		return GroupIdentity{State: GroupDead}, nil
	}
	state := sequence[0]
	if len(sequence) > 1 {
		i.states[pgid] = sequence[1:]
	}
	return state, nil
}

func (i *recoveryInspector) KillGroup(pgid int) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.kills = append(i.kills, pgid)
	if i.killDead {
		i.states[pgid] = []GroupIdentity{{State: GroupDead}}
	}
	return i.killErr
}

func TestRecoveryClassifiesNonterminalRuns(t *testing.T) {
	pgidDead, pgidOwned, pgidCancelled, pgidDeadCancelled := 11, 12, 13, 15
	store := &recoveryStore{newDispatcherTestStore()}
	store.runs = map[string]RunRecord{
		"queued-cancel": {ID: "queued-cancel", Status: RunStatusQueued, CancelRequested: true, Error: "operator cancelled"},
		"queued":        {ID: "queued", Status: RunStatusQueued},
		"no-pgid":       {ID: "no-pgid", Status: RunStatusRunning},
		"dead":          {ID: "dead", Status: RunStatusRunning, ProcessGroupID: &pgidDead},
		"dead-cancel":   {ID: "dead-cancel", Status: RunStatusRunning, ProcessGroupID: &pgidDeadCancelled, CancelRequested: true, Error: "operator cancelled"},
		"owned":         {ID: "owned", Status: RunStatusRunning, ProcessGroupID: &pgidOwned},
		"owned-cancel":  {ID: "owned-cancel", Status: RunStatusRunning, ProcessGroupID: &pgidCancelled, CancelRequested: true, Error: "disable"},
	}
	inspector := &recoveryInspector{killDead: true, states: map[int][]GroupIdentity{
		pgidDead:          {{State: GroupDead}},
		pgidDeadCancelled: {{State: GroupDead}},
		pgidOwned:         {{State: GroupAliveOwned}, {State: GroupAliveOwned}},
		pgidCancelled:     {{State: GroupAliveOwned}, {State: GroupAliveOwned}},
	}}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector})
	if err := d.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := map[string]struct {
		status     string
		unverified bool
	}{
		"queued-cancel": {RunStatusCancelled, false},
		"queued":        {RunStatusInterrupted, false},
		"no-pgid":       {RunStatusInterrupted, true},
		"dead":          {RunStatusInterrupted, false},
		"dead-cancel":   {RunStatusCancelled, false},
		"owned":         {RunStatusInterrupted, false},
		"owned-cancel":  {RunStatusCancelled, false},
	}
	for id, expectation := range want {
		record, _, err := store.Run(id)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status != expectation.status || record.OutputUnverified != expectation.unverified {
			t.Errorf("%s = status %q unverified=%v, want %q/%v", id, record.Status, record.OutputUnverified, expectation.status, expectation.unverified)
		}
	}
	inspector.mu.Lock()
	defer inspector.mu.Unlock()
	if len(inspector.kills) != 2 {
		t.Fatalf("kills = %v, want only the two owned groups", inspector.kills)
	}
}

func TestRecoveryLeavesLiveUnverifiedGroupNonterminal(t *testing.T) {
	pgid := 21
	store := &recoveryStore{newDispatcherTestStore()}
	store.runs["live-unverified"] = RunRecord{ID: "live-unverified", Status: RunStatusRunning, ProcessGroupID: &pgid}
	inspector := &recoveryInspector{states: map[int][]GroupIdentity{
		pgid: {{State: GroupAliveUnverified}},
	}}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector})
	err := d.Recover(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ownership") {
		t.Fatalf("Recover error = %v, want ownership refusal", err)
	}
	record, _, readErr := store.Run("live-unverified")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if record.Status != RunStatusRunning || record.FinishedAt != nil || record.OutputUnverified {
		t.Fatalf("run was settled while its group remained alive: %+v", record)
	}
	if len(inspector.kills) != 0 {
		t.Fatalf("unverified group was killed: %v", inspector.kills)
	}
}

func TestRecoveryRechecksOwnershipImmediatelyBeforeKill(t *testing.T) {
	pgid := 22
	store := &recoveryStore{newDispatcherTestStore()}
	store.runs["reused-between-checks"] = RunRecord{ID: "reused-between-checks", Status: RunStatusRunning, ProcessGroupID: &pgid}
	inspector := &recoveryInspector{states: map[int][]GroupIdentity{
		pgid: {{State: GroupAliveOwned}, {State: GroupAliveUnverified}},
	}}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector})
	if err := d.Recover(context.Background()); err == nil || !strings.Contains(err.Error(), "ownership changed") {
		t.Fatalf("Recover error = %v", err)
	}
	record, _, err := store.Run("reused-between-checks")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != RunStatusRunning || record.FinishedAt != nil || record.OutputUnverified {
		t.Fatalf("record = %+v", record)
	}
	if len(inspector.kills) != 0 {
		t.Fatalf("ownership changed but group was killed: %v", inspector.kills)
	}
}

func TestRecoveryWaitsForDeathWhenOwnershipMarkerDisappearsAfterSignal(t *testing.T) {
	pgid := 32
	store := &recoveryStore{newDispatcherTestStore()}
	store.runs["lost-after-signal"] = RunRecord{ID: "lost-after-signal", Status: RunStatusRunning, ProcessGroupID: &pgid}
	inspector := &recoveryInspector{states: map[int][]GroupIdentity{
		pgid: {{State: GroupAliveOwned}, {State: GroupAliveOwned}, {State: GroupAliveUnverified}, {State: GroupDead}},
	}}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector})
	if err := d.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	record, _, err := store.Run("lost-after-signal")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != RunStatusInterrupted || record.FinishedAt == nil || record.OutputUnverified {
		t.Fatalf("record = %+v", record)
	}
	if len(inspector.kills) != 1 || inspector.kills[0] != pgid {
		t.Fatalf("kills = %v", inspector.kills)
	}
}

func TestRecoveryInspectionFailureFailsClosed(t *testing.T) {
	pgid := 33
	store := &recoveryStore{newDispatcherTestStore()}
	store.runs["inspect-error"] = RunRecord{ID: "inspect-error", Status: RunStatusRunning, ProcessGroupID: &pgid, CancelRequested: true}
	inspector := &recoveryInspector{states: map[int][]GroupIdentity{}, errs: map[int]error{pgid: errors.New("unavailable")}}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector})
	if err := d.Recover(context.Background()); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Recover error = %v", err)
	}
	record, _, _ := store.Run("inspect-error")
	if record.Status != RunStatusRunning || record.FinishedAt != nil || record.OutputUnverified {
		t.Fatalf("record = %+v", record)
	}
	if len(inspector.kills) != 0 {
		t.Fatalf("unverified group was killed: %v", inspector.kills)
	}
}

func TestRecoveryHonoursContextWhileWaitingForOwnedGroupDeath(t *testing.T) {
	pgid := 44
	store := &recoveryStore{newDispatcherTestStore()}
	store.runs["stuck"] = RunRecord{ID: "stuck", Status: RunStatusRunning, ProcessGroupID: &pgid}
	inspector := &recoveryInspector{states: map[int][]GroupIdentity{
		pgid: {{State: GroupAliveOwned}, {State: GroupAliveOwned}, {State: GroupAliveOwned}},
	}}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := d.Recover(ctx); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Recover error = %v", err)
	}
	record, _, _ := store.Run("stuck")
	if record.Status != RunStatusRunning || record.FinishedAt != nil || record.OutputUnverified {
		t.Fatalf("record = %+v", record)
	}
}
