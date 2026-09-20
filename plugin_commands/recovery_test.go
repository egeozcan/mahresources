package plugin_commands

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type recoveryStore struct {
	*dispatcherTestStore
	runOrder []string
}

func (s *recoveryStore) NonterminalRuns() ([]RecoveryRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []RecoveryRun
	appendRun := func(run RunRecord) {
		if !RunStatusTerminal(run.Status) {
			result = append(result, RecoveryRun{RunRecord: run, BootSessionID: s.bootSessionIDs[run.ID]})
		}
	}
	if len(s.runOrder) != 0 {
		for _, id := range s.runOrder {
			appendRun(s.runs[id])
		}
		return result, nil
	}
	for _, run := range s.runs {
		appendRun(run)
	}
	return result, nil
}

type recoveryInspector struct {
	mu          sync.Mutex
	states      map[int][]GroupIdentity
	errs        map[int]error
	inspectErrs map[int][]error
	inspections []int
	kills       []int
	killErr     error
	killDead    bool
}

func (i *recoveryInspector) InspectGroup(pgid int, _ string) (GroupIdentity, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.inspections = append(i.inspections, pgid)
	if err := i.errs[pgid]; err != nil {
		return GroupIdentity{}, err
	}
	if sequence := i.inspectErrs[pgid]; len(sequence) != 0 {
		err := sequence[0]
		if len(sequence) == 1 {
			delete(i.inspectErrs, pgid)
		} else {
			i.inspectErrs[pgid] = sequence[1:]
		}
		if err != nil {
			return GroupIdentity{}, err
		}
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

func requireSingleRecoveryBlocker(t *testing.T, err error, runID string, pgid int, reason string) *RecoveryBlockedError {
	t.Helper()
	var blocked *RecoveryBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("Recover error = %v, want *RecoveryBlockedError", err)
	}
	if len(blocked.Blockers) != 1 {
		t.Fatalf("blockers = %+v, want one", blocked.Blockers)
	}
	blocker := blocked.Blockers[0]
	if blocker.RunID != runID || blocker.ProcessGroupID != pgid || !strings.Contains(blocker.Reason, reason) {
		t.Fatalf("blocker = %+v, want run %q pgid %d reason containing %q", blocker, runID, pgid, reason)
	}
	return blocked
}

func TestRecoveryClassifiesNonterminalRuns(t *testing.T) {
	pgidDead, pgidOwned, pgidCancelled, pgidDeadCancelled := 11, 12, 13, 15
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
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
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
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
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
	store.runs["reused-between-checks"] = RunRecord{ID: "reused-between-checks", Status: RunStatusRunning, ProcessGroupID: &pgid}
	inspector := &recoveryInspector{states: map[int][]GroupIdentity{
		pgid: {{State: GroupAliveOwned}, {State: GroupAliveUnverified}},
	}}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector})
	requireSingleRecoveryBlocker(t, d.Recover(context.Background()), "reused-between-checks", pgid, "ownership changed")
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

func TestRecoverySignalFailureIsTypedBlocker(t *testing.T) {
	pgid := 23
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
	store.runs["signal-failed"] = RunRecord{ID: "signal-failed", Status: RunStatusRunning, ProcessGroupID: &pgid}
	inspector := &recoveryInspector{
		states:  map[int][]GroupIdentity{pgid: {{State: GroupAliveOwned}}},
		killErr: errors.New("signal refused"),
	}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector})
	requireSingleRecoveryBlocker(t, d.Recover(context.Background()), "signal-failed", pgid, "signal refused")
	if len(inspector.kills) != 1 || inspector.kills[0] != pgid {
		t.Fatalf("kills = %v, want [%d]", inspector.kills, pgid)
	}
}

func TestRecoveryPostSignalInspectionFailureIsTypedBlocker(t *testing.T) {
	pgid := 24
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
	store.runs["post-signal-inspection"] = RunRecord{ID: "post-signal-inspection", Status: RunStatusRunning, ProcessGroupID: &pgid}
	inspector := &recoveryInspector{
		states:      map[int][]GroupIdentity{pgid: {{State: GroupAliveOwned}}},
		inspectErrs: map[int][]error{pgid: {nil, nil, errors.New("post-signal scan unavailable")}},
	}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector})
	requireSingleRecoveryBlocker(t, d.Recover(context.Background()), "post-signal-inspection", pgid, "post-signal scan unavailable")
}

func TestRecoveryUnknownIdentityStateIsOrdinaryError(t *testing.T) {
	pgid := 25
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
	store.runs["unknown-state"] = RunRecord{ID: "unknown-state", Status: RunStatusRunning, ProcessGroupID: &pgid}
	inspector := &recoveryInspector{states: map[int][]GroupIdentity{
		pgid: {{State: GroupState(99)}},
	}}
	err := NewDispatcher(Dependencies{Store: store, Inspector: inspector}).Recover(context.Background())
	var blocked *RecoveryBlockedError
	if err == nil || errors.As(err, &blocked) || !strings.Contains(err.Error(), "unknown state") {
		t.Fatalf("Recover error = %v, blocker = %+v; want ordinary unknown-state error", err, blocked)
	}
}

func TestRecoveryUnknownRunStatusIsOrdinaryError(t *testing.T) {
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
	store.runs["unknown-status"] = RunRecord{ID: "unknown-status", Status: "orphaned"}
	err := NewDispatcher(Dependencies{Store: store, Inspector: &recoveryInspector{}}).Recover(context.Background())
	var blocked *RecoveryBlockedError
	if err == nil || errors.As(err, &blocked) || !strings.Contains(err.Error(), "unknown status") {
		t.Fatalf("Recover error = %v, blocker = %+v; want ordinary unknown-status error", err, blocked)
	}
	record, _, readErr := store.Run("unknown-status")
	if readErr != nil || record.Status != "orphaned" || record.FinishedAt != nil {
		t.Fatalf("malformed record was changed: record=%+v err=%v", record, readErr)
	}
}

func TestRecoveryRejectsMalformedProcessIdentityBeforeTerminalizingAnyRun(t *testing.T) {
	positive, zero, negative := 25, 0, -25
	tests := []struct {
		name          string
		status        string
		pgid          *int
		bootSessionID string
	}{
		{name: "queued with process group", status: RunStatusQueued, pgid: &positive},
		{name: "queued with boot identity", status: RunStatusQueued, bootSessionID: "boot-a"},
		{name: "queued with both identity fields", status: RunStatusQueued, pgid: &positive, bootSessionID: "boot-a"},
		{name: "running crash window with boot identity", status: RunStatusRunning, bootSessionID: "boot-a"},
		{name: "running zero process group", status: RunStatusRunning, pgid: &zero},
		{name: "running zero process group with boot identity", status: RunStatusRunning, pgid: &zero, bootSessionID: "boot-a"},
		{name: "running negative process group", status: RunStatusRunning, pgid: &negative},
		{name: "running negative process group with boot identity", status: RunStatusRunning, pgid: &negative, bootSessionID: "boot-a"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &recoveryStore{
				dispatcherTestStore: newDispatcherTestStore(),
				runOrder:            []string{"valid-queued", "malformed"},
			}
			store.runs["valid-queued"] = RunRecord{ID: "valid-queued", Status: RunStatusQueued}
			store.runs["malformed"] = RunRecord{ID: "malformed", Status: test.status, ProcessGroupID: test.pgid}
			store.bootSessionIDs["malformed"] = test.bootSessionID
			inspector := &recoveryInspector{}

			err := NewDispatcher(Dependencies{Store: store, Inspector: inspector}).Recover(context.Background())
			var blocked *RecoveryBlockedError
			if err == nil || errors.As(err, &blocked) || !strings.Contains(err.Error(), "invalid process identity") {
				t.Fatalf("Recover error = %v, blocker = %+v; want ordinary process-identity consistency error", err, blocked)
			}
			for id, wantStatus := range map[string]string{"valid-queued": RunStatusQueued, "malformed": test.status} {
				record, _, readErr := store.Run(id)
				if readErr != nil || record.Status != wantStatus || record.FinishedAt != nil {
					t.Fatalf("run %q changed before validation completed: record=%+v err=%v", id, record, readErr)
				}
			}
			malformed, _, readErr := store.Run("malformed")
			pgidChanged := (malformed.ProcessGroupID == nil) != (test.pgid == nil)
			if malformed.ProcessGroupID != nil && test.pgid != nil {
				pgidChanged = *malformed.ProcessGroupID != *test.pgid
			}
			if readErr != nil || pgidChanged || store.bootSessionIDs["malformed"] != test.bootSessionID {
				t.Fatalf("malformed identity changed: record=%+v boot=%q err=%v", malformed, store.bootSessionIDs["malformed"], readErr)
			}
			if len(inspector.inspections) != 0 || len(inspector.kills) != 0 {
				t.Fatalf("malformed identity reached process inspection: inspections=%v kills=%v", inspector.inspections, inspector.kills)
			}
		})
	}
}

func TestRecoveryWaitsForDeathWhenOwnershipMarkerDisappearsAfterSignal(t *testing.T) {
	pgid := 32
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
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
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
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
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
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

func TestRecoveryDifferentBootNeverTouchesPersistedPGID(t *testing.T) {
	pgid := 42
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
	store.bootSessionIDs["old"] = "boot-a"
	store.runs["old"] = RunRecord{ID: "old", Status: RunStatusRunning, ProcessGroupID: &pgid}
	inspector := &recoveryInspector{states: map[int][]GroupIdentity{
		pgid: {{State: GroupAliveOwned}},
	}}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector, BootSessionID: "boot-b"})
	if err := d.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	inspector.mu.Lock()
	inspections, kills := append([]int(nil), inspector.inspections...), append([]int(nil), inspector.kills...)
	inspector.mu.Unlock()
	if len(inspections) != 0 || len(kills) != 0 {
		t.Fatalf("prior-boot PGID was touched: inspections=%v kills=%v", inspections, kills)
	}
	record, _, err := store.Run("old")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != RunStatusInterrupted || !record.OutputUnverified || !strings.Contains(record.Error, "prior boot") {
		t.Fatalf("record = %+v", record)
	}
}

func TestRecoveryDifferentBootPreservesCancellationPrecedence(t *testing.T) {
	pgid := 43
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
	store.bootSessionIDs["cancelled-old"] = "boot-a"
	store.runs["cancelled-old"] = RunRecord{
		ID: "cancelled-old", Status: RunStatusRunning, ProcessGroupID: &pgid,
		CancelRequested: true, Error: "operator cancelled",
	}
	inspector := &recoveryInspector{states: map[int][]GroupIdentity{pgid: {{State: GroupAliveOwned}}}}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector, BootSessionID: "boot-b"})
	if err := d.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	record, _, err := store.Run("cancelled-old")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != RunStatusCancelled || !record.OutputUnverified || !strings.Contains(record.Error, "operator cancelled") {
		t.Fatalf("record = %+v", record)
	}
	if len(inspector.inspections) != 0 || len(inspector.kills) != 0 {
		t.Fatalf("prior-boot cancelled PGID was touched: inspections=%v kills=%v", inspector.inspections, inspector.kills)
	}
}

func TestRecoverySameOrUnknownBootInspectsPersistedPGID(t *testing.T) {
	tests := []struct {
		name, persisted, current string
	}{
		{name: "same", persisted: "boot-a", current: "boot-a"},
		{name: "unknown persisted", current: "boot-a"},
		{name: "unknown current", persisted: "boot-a"},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pgid := 50 + index
			store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
			store.bootSessionIDs["run"] = test.persisted
			store.runs["run"] = RunRecord{ID: "run", Status: RunStatusRunning, ProcessGroupID: &pgid}
			inspector := &recoveryInspector{states: map[int][]GroupIdentity{pgid: {{State: GroupDead}}}}
			d := NewDispatcher(Dependencies{Store: store, Inspector: inspector, BootSessionID: test.current})
			if err := d.Recover(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(inspector.inspections) != 1 || inspector.inspections[0] != pgid {
				t.Fatalf("inspections = %v, want [%d]", inspector.inspections, pgid)
			}
		})
	}
}

func TestRecoveryBlockersAggregateAndContinueSafeRows(t *testing.T) {
	firstPGID, deadPGID, secondPGID := 61, 62, 63
	store := &recoveryStore{
		dispatcherTestStore: newDispatcherTestStore(),
		runOrder:            []string{"blocked-first", "dead-between", "blocked-second"},
	}
	store.runs["blocked-first"] = RunRecord{ID: "blocked-first", Status: RunStatusRunning, ProcessGroupID: &firstPGID}
	store.runs["dead-between"] = RunRecord{ID: "dead-between", Status: RunStatusRunning, ProcessGroupID: &deadPGID}
	store.runs["blocked-second"] = RunRecord{ID: "blocked-second", Status: RunStatusRunning, ProcessGroupID: &secondPGID}
	inspector := &recoveryInspector{states: map[int][]GroupIdentity{
		firstPGID:  {{State: GroupAliveUnverified}},
		deadPGID:   {{State: GroupDead}},
		secondPGID: {{State: GroupAliveUnverified}},
	}}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector})
	err := d.Recover(context.Background())
	var blocked *RecoveryBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("Recover error = %v, want *RecoveryBlockedError", err)
	}
	if len(blocked.Blockers) != 2 || blocked.Blockers[0].RunID != "blocked-first" || blocked.Blockers[1].RunID != "blocked-second" {
		t.Fatalf("blockers = %+v", blocked.Blockers)
	}
	if blocked.Blockers[0].ProcessGroupID != firstPGID || blocked.Blockers[1].ProcessGroupID != secondPGID {
		t.Fatalf("blocker PGIDs = %+v", blocked.Blockers)
	}
	for _, id := range []string{"blocked-first", "blocked-second"} {
		record, _, readErr := store.Run(id)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if record.Status != RunStatusRunning || record.FinishedAt != nil {
			t.Fatalf("blocked record %s was settled: %+v", id, record)
		}
	}
	dead, _, readErr := store.Run("dead-between")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if dead.Status != RunStatusInterrupted || dead.FinishedAt == nil {
		t.Fatalf("safe record was not settled: %+v", dead)
	}
}

func TestRecoveryInspectionBlockerRetainsReason(t *testing.T) {
	pgid := 64
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
	store.runs["inspect-error"] = RunRecord{ID: "inspect-error", Status: RunStatusRunning, ProcessGroupID: &pgid}
	inspector := &recoveryInspector{errs: map[int]error{pgid: errors.New("process table unavailable")}}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector})
	err := d.Recover(context.Background())
	var blocked *RecoveryBlockedError
	if !errors.As(err, &blocked) || len(blocked.Blockers) != 1 {
		t.Fatalf("Recover error = %v, blockers = %+v", err, blocked)
	}
	if !strings.Contains(blocked.Blockers[0].Reason, "process table unavailable") {
		t.Fatalf("blocker reason = %q", blocked.Blockers[0].Reason)
	}
}

func TestRecoverySignalAttemptIsLatchedAcrossHealingScans(t *testing.T) {
	pgid := 65
	store := &recoveryStore{dispatcherTestStore: newDispatcherTestStore()}
	store.runs["stubborn"] = RunRecord{ID: "stubborn", Status: RunStatusRunning, ProcessGroupID: &pgid}
	inspector := &recoveryInspector{states: map[int][]GroupIdentity{
		pgid: {{State: GroupAliveOwned}},
	}}
	d := NewDispatcher(Dependencies{Store: store, Inspector: inspector})
	d.recoveryPollInterval = time.Millisecond
	d.recoveryDrainTimeout = 5 * time.Millisecond
	requireSingleRecoveryBlocker(t, d.Recover(context.Background()), "stubborn", pgid, "recovery deadline")
	requireSingleRecoveryBlocker(t, d.Recover(context.Background()), "stubborn", pgid, "recovery deadline")

	inspector.mu.Lock()
	killsBeforeHealing := append([]int(nil), inspector.kills...)
	inspector.states[pgid] = []GroupIdentity{{State: GroupDead}}
	inspector.mu.Unlock()
	if len(killsBeforeHealing) != 1 || killsBeforeHealing[0] != pgid {
		t.Fatalf("kills before healing = %v, want one signal attempt across two live scans", killsBeforeHealing)
	}
	if err := d.Recover(context.Background()); err != nil {
		t.Fatalf("healing Recover: %v", err)
	}
	inspector.mu.Lock()
	kills := append([]int(nil), inspector.kills...)
	inspector.mu.Unlock()
	if len(kills) != 1 || kills[0] != pgid {
		t.Fatalf("kills = %v, want one signal attempt", kills)
	}
	record, _, err := store.Run("stubborn")
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != RunStatusInterrupted {
		t.Fatalf("record = %+v", record)
	}
}
