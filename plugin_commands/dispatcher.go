package plugin_commands

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxActiveCommands          = 4
	maxActiveCommandsPerPlugin = 2
	maxActiveImports           = 2
	dispatchFailureRetryDelay  = 100 * time.Millisecond
)

var (
	errDispatcherStopped  = errors.New("plugin command dispatcher is stopped")
	errDispatcherShutdown = errors.New("plugin command dispatcher is shutting down")
	errOperatorCancelled  = errors.New("plugin command was cancelled")
)

type commandAdmission interface {
	Prepare(QueuedRun) error
	Cleanup(QueuedRun)
}

type Dispatcher struct {
	deps Dependencies

	mu      sync.Mutex
	started bool
	inbox   chan any
	done    chan struct{}

	controls sync.Map // run id -> *runControl

	workerMu      sync.Mutex
	workerClosing bool
	workers       sync.WaitGroup

	shutdownMu  sync.Mutex
	shutdownErr error

	importerMu sync.RWMutex
	importer   Importer

	shutdownStarted func() // test barrier; nil in production
}

type runControl struct {
	forkMu    sync.Mutex
	cancelled atomic.Bool
}

type commandSubmission struct {
	run   QueuedRun
	reply chan error
}

type importSubmission struct {
	item  queuedImport
	reply chan error
}

type cancelSubmission struct {
	runID  string
	reason string
	reply  chan error
}

type stopDispatcher struct {
	ctx   context.Context
	reply chan error
}
type disablePluginSubmission struct {
	plugin string
	reason string
	reply  chan error
}
type commandCompleted struct {
	runID   string
	outcome Outcome
}
type importCompleted struct{ importID string }

type queuedImport struct {
	spec       ImportJobSpec
	run        func(context.Context, Progress) Outcome
	release    func()
	cleanup    func()
	completion func(ImportResult)
}

type activeCommand struct {
	plugin string
	cancel context.CancelCauseFunc
}

type activeImport struct {
	cancel  context.CancelCauseFunc
	release func()
}

type commandDispatchFailure struct {
	run           QueuedRun
	dispatchErr   error
	markedRunning bool
	status        string
	reason        string
}

type importDispatchFailure struct {
	item        queuedImport
	dispatchErr error
	status      string
	reason      string
}

type queuedCancellation struct {
	run    QueuedRun
	plugin string
	reason string
}

type dispatcherState struct {
	commands map[string][]QueuedRun
	imports  map[string][]queuedImport

	commandPlugins []string
	importPlugins  []string
	commandSeen    map[string]bool
	importSeen     map[string]bool
	commandCursor  int
	importCursor   int

	activeCommands        map[string]activeCommand
	activeImports         map[string]activeImport
	activeByPlugin        map[string]int
	failedCommandDispatch map[string]*commandDispatchFailure
	failedImportDispatch  map[string]*importDispatchFailure
	queuedCancellations   map[string]*queuedCancellation
	cancelWaiters         map[string][]chan error
}

func NewDispatcher(deps Dependencies) *Dispatcher {
	if deps.Logf == nil {
		deps.Logf = func(string, ...any) {}
	}
	if deps.Inspector == nil {
		deps.Inspector = nativeProcessInspector{}
	}
	if deps.Leases == nil {
		deps.Leases = NewLeaseManager()
	}
	return &Dispatcher{deps: deps, inbox: make(chan any, 256), done: make(chan struct{})}
}

func (d *Dispatcher) Start(ctx context.Context) error {
	if d.deps.Store == nil || d.deps.Jobs == nil || d.deps.Executor == nil || d.deps.Settings == nil {
		return fmt.Errorf("plugin_commands: dispatcher dependencies are incomplete")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return fmt.Errorf("plugin_commands: dispatcher already started")
	}
	d.started = true
	go d.run(ctx)
	return nil
}

func (d *Dispatcher) Stop(ctx context.Context) error {
	reply := make(chan error, 1)
	if err := d.send(ctx, stopDispatcher{ctx: ctx, reply: reply}); err != nil {
		if errors.Is(err, errDispatcherStopped) {
			return d.shutdownResult()
		}
		return err
	}
	// Once shutdown is accepted, the owner uses ctx only to bound worker drain.
	// Stop waits for the owner's reply so terminal classification and persistence
	// have completed before the caller is told shutdown is done.
	select {
	case err := <-reply:
		return err
	case <-d.done:
		select {
		case err := <-reply:
			return err
		default:
			return d.shutdownResult()
		}
	}
}

func (d *Dispatcher) setShutdownResult(err error) {
	d.shutdownMu.Lock()
	d.shutdownErr = err
	d.shutdownMu.Unlock()
}

func (d *Dispatcher) shutdownResult() error {
	d.shutdownMu.Lock()
	defer d.shutdownMu.Unlock()
	return d.shutdownErr
}

func (d *Dispatcher) Submit(request CommandRequest) (string, error) {
	request = cloneCommandRequest(request)
	runID, err := newRunID()
	if err != nil {
		return "", err
	}
	exchangeDir := filepath.Join(d.deps.Settings.StagingRoot(), "plugin_exchange", request.PluginName, runID)
	invocation, err := BuildInvocation(request.Declaration, request.Params, exchangeDir)
	if err != nil {
		return "", err
	}
	run := QueuedRun{RunID: runID, Request: request, ExchangeDir: exchangeDir, Invocation: invocation, control: &runControl{}}
	admission, prepared := d.deps.Executor.(commandAdmission)
	if prepared {
		if err := admission.Prepare(run); err != nil {
			return "", err
		}
	}
	cleanup := func() {
		if prepared {
			admission.Cleanup(run)
		}
	}
	d.controls.Store(runID, run.control)
	submission := commandSubmission{run: run, reply: make(chan error, 1)}
	if err := d.send(context.Background(), submission); err != nil {
		d.controls.Delete(runID)
		cleanup()
		return "", err
	}
	if err := awaitDispatcherReply(submission.reply, d.done); err != nil {
		d.controls.Delete(runID)
		cleanup()
		return "", err
	}
	return runID, nil
}

func (d *Dispatcher) Cancel(runID, reason string) error {
	// The per-run fork barrier orders durable cancellation against Start. If the
	// runner owns it, Start happens first and this is a post-fork cancellation;
	// otherwise the durable latch and in-memory latch are both visible before the
	// runner performs its final check. The dispatcher owner is contacted only
	// after that boundary has been settled.
	var control *runControl
	if value, ok := d.controls.Load(runID); ok {
		control, _ = value.(*runControl)
	}
	if control != nil {
		control.forkMu.Lock()
	}
	err := d.deps.Store.RequestRunCancel(runID, reason)
	if err == nil && control != nil {
		control.cancelled.Store(true)
	}
	if control != nil {
		control.forkMu.Unlock()
	}
	if err != nil {
		return err
	}
	request := cancelSubmission{runID: runID, reason: reason, reply: make(chan error, 1)}
	if err := d.send(context.Background(), request); err != nil {
		return err
	}
	return awaitDispatcherReply(request.reply, d.done)
}

// DisablePlugin cancels every nonterminal command for plugin through the same
// durable primitive used by operator cancellation, then cancels imports which
// are still pending in the dispatcher's private queue. Running imports are left
// alone so a commit already in progress can finish atomically.
func (d *Dispatcher) DisablePlugin(plugin, reason string) error {
	runs, err := d.deps.Store.NonterminalRuns()
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.PluginName != plugin {
			continue
		}
		if err := d.Cancel(run.ID, reason); err != nil && !errors.Is(err, ErrRunNotCancellable) {
			return err
		}
	}
	request := disablePluginSubmission{plugin: plugin, reason: reason, reply: make(chan error, 1)}
	if err := d.send(context.Background(), request); err != nil {
		return err
	}
	return awaitDispatcherReply(request.reply, d.done)
}

// submitImport is the Task 5 queue seam. Task 9 places validated durable import
// submissions on it after performing the file and principal checks.
func (d *Dispatcher) submitImport(spec ImportJobSpec, run func(context.Context, Progress) Outcome) error {
	if run == nil {
		return fmt.Errorf("plugin_commands: import run function is required")
	}
	if spec.RunID == "" {
		return fmt.Errorf("plugin_commands: import requires run id")
	}
	// Pin at durable admission, before the import waits in the private queue.
	// The sweep cannot pass its skip-check until the import commits or reaches a
	// durable terminal failure.
	release, err := d.deps.Leases.Acquire(spec.RunID)
	if err != nil {
		return err
	}
	return d.submitClaimedImport(queuedImport{spec: spec, run: run, release: release})
}

func (d *Dispatcher) submitClaimedImport(item queuedImport) error {
	if item.run == nil {
		if item.release != nil {
			item.release()
		}
		return fmt.Errorf("plugin_commands: import run function is required")
	}
	if item.spec.RunID == "" {
		if item.release != nil {
			item.release()
		}
		return fmt.Errorf("plugin_commands: import requires run id")
	}
	request := importSubmission{item: item, reply: make(chan error, 1)}
	if err := d.send(context.Background(), request); err != nil {
		if item.release != nil {
			item.release()
		}
		return err
	}
	if err := awaitDispatcherReply(request.reply, d.done); err != nil {
		if item.release != nil {
			item.release()
		}
		return err
	}
	return nil
}

// awaitDispatcherReply gives an accepted message's reply precedence over
// shutdown. The owner sends replies before it can close done, so the second
// receive is stable after observing done and cannot miss a later acceptance.
func awaitDispatcherReply(reply <-chan error, done <-chan struct{}) error {
	select {
	case err := <-reply:
		return err
	case <-done:
		select {
		case err := <-reply:
			return err
		default:
			return errDispatcherStopped
		}
	}
}

func (d *Dispatcher) send(ctx context.Context, message any) error {
	d.mu.Lock()
	started := d.started
	d.mu.Unlock()
	if !started {
		return fmt.Errorf("plugin_commands: dispatcher is not started")
	}
	select {
	case d.inbox <- message:
		return nil
	case <-d.done:
		return errDispatcherStopped
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Dispatcher) run(ctx context.Context) {
	defer close(d.done)
	retryTicker := time.NewTicker(dispatchFailureRetryDelay)
	defer retryTicker.Stop()
	state := dispatcherState{
		commands:              make(map[string][]QueuedRun),
		imports:               make(map[string][]queuedImport),
		commandSeen:           make(map[string]bool),
		importSeen:            make(map[string]bool),
		activeCommands:        make(map[string]activeCommand),
		activeImports:         make(map[string]activeImport),
		activeByPlugin:        make(map[string]int),
		failedCommandDispatch: make(map[string]*commandDispatchFailure),
		failedImportDispatch:  make(map[string]*importDispatchFailure),
		queuedCancellations:   make(map[string]*queuedCancellation),
		cancelWaiters:         make(map[string][]chan error),
	}

	for {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), groupDrainTimeout)
			err := d.shutdown(shutdownCtx, &state)
			cancel()
			d.setShutdownResult(err)
			if err != nil {
				d.deps.Logf("plugin command dispatcher shutdown: %v", err)
			}
			return
		case <-retryTicker.C:
			d.retryDispatchFailures(&state)
			d.schedule(&state)
		case raw := <-d.inbox:
			switch message := raw.(type) {
			case commandSubmission:
				message.reply <- d.acceptCommand(&state, message.run)
			case importSubmission:
				message.reply <- d.acceptImport(&state, message)
			case cancelSubmission:
				deferred, err := d.cancel(&state, message.runID, message.reason, message.reply)
				if !deferred {
					message.reply <- err
				}
			case disablePluginSubmission:
				message.reply <- d.disableQueuedImports(&state, message.plugin, message.reason)
			case commandCompleted:
				d.completeCommand(&state, message)
			case importCompleted:
				if active, ok := state.activeImports[message.importID]; ok {
					active.cancel(nil)
					active.release()
					delete(state.activeImports, message.importID)
				}
			case stopDispatcher:
				err := d.shutdown(message.ctx, &state)
				d.setShutdownResult(err)
				message.reply <- err
				return
			}
			d.schedule(&state)
		}
	}
}

func (s *dispatcherState) cancelActive(cause error) {
	for _, active := range s.activeCommands {
		active.cancel(cause)
	}
	for _, active := range s.activeImports {
		active.cancel(cause)
	}
}

func (d *Dispatcher) disableQueuedImports(state *dispatcherState, plugin, reason string) error {
	queue := state.imports[plugin]
	kept := queue[:0]
	var firstErr error
	for _, item := range queue {
		won, err := d.deps.Store.FinishImport(item.spec.ImportID, ImportFinish{
			Status: ImportStatusCancelled, Error: reason, FinishedAt: time.Now().UTC(),
		})
		if err != nil {
			kept = append(kept, item)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !won {
			d.deps.Logf("plugin command import %s was no longer pending during plugin disable", item.spec.ImportID)
		}
		releaseImportItem(item, ImportResult{ImportID: item.spec.ImportID, Error: reason})
	}
	state.imports[plugin] = kept
	for _, failure := range state.failedImportDispatch {
		if failure.item.spec.PluginName != plugin {
			continue
		}
		failure.status = ImportStatusCancelled
		failure.reason = reason
		if _, err := d.persistImportDispatchFailure(state, failure); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (d *Dispatcher) shutdown(ctx context.Context, state *dispatcherState) error {
	const shutdownReason = "server interrupted"
	d.workerMu.Lock()
	d.workerClosing = true
	d.workerMu.Unlock()
	if d.shutdownStarted != nil {
		d.shutdownStarted()
	}
	var firstErr error

	for plugin, queue := range state.commands {
		for _, run := range queue {
			result, err := d.finishShutdownRun(run.RunID, false)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			d.controls.Delete(run.RunID)
			deliverCompletion(run.Request.Completion, result)
		}
		delete(state.commands, plugin)
	}
	for id, cancellation := range state.queuedCancellations {
		if _, err := d.persistQueuedCancellation(state, cancellation); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(state.queuedCancellations, id)
	}
	for id, failure := range state.failedCommandDispatch {
		result, err := d.finishShutdownRun(id, false)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		d.releaseCommandDispatchFailure(state, failure, result)
	}
	for plugin, queue := range state.imports {
		for _, item := range queue {
			_, err := d.deps.Store.FinishImport(item.spec.ImportID, ImportFinish{
				Status: ImportStatusInterrupted, Error: shutdownReason, FinishedAt: time.Now().UTC(),
			})
			if err != nil && firstErr == nil {
				firstErr = err
			}
			releaseImportItem(item, ImportResult{ImportID: item.spec.ImportID, Error: shutdownReason})
		}
		delete(state.imports, plugin)
	}
	for id, failure := range state.failedImportDispatch {
		_, err := d.deps.Store.FinishImport(id, ImportFinish{
			Status: ImportStatusInterrupted, Error: shutdownReason, FinishedAt: time.Now().UTC(),
		})
		if err != nil && firstErr == nil {
			firstErr = err
		}
		delete(state.failedImportDispatch, id)
		releaseImportItem(failure.item, ImportResult{ImportID: id, Error: shutdownReason})
		if active, ok := state.activeImports[failure.item.spec.ImportID]; ok {
			active.cancel(errDispatcherShutdown)
			active.release()
			delete(state.activeImports, failure.item.spec.ImportID)
		}
	}

	state.cancelActive(errDispatcherShutdown)
	workersDone := make(chan struct{})
	go func() {
		d.workers.Wait()
		close(workersDone)
	}()
	timedOut := false
	select {
	case <-workersDone:
	case <-ctx.Done():
		timedOut = true
		if firstErr == nil {
			firstErr = ctx.Err()
		}
	}

	if err := d.deps.Store.InterruptNonterminalImports(time.Now().UTC()); err != nil && firstErr == nil {
		firstErr = err
	}
	for id, active := range state.activeImports {
		active.cancel(errDispatcherShutdown)
		active.release()
		delete(state.activeImports, id)
	}
	for id, active := range state.activeCommands {
		result, err := d.finishShutdownRun(id, timedOut)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else {
			_ = result
		}
		active.cancel(errDispatcherShutdown)
		delete(state.activeCommands, id)
		state.activeByPlugin[active.plugin]--
		d.controls.Delete(id)
		for _, waiter := range state.cancelWaiters[id] {
			waiter <- err
		}
		delete(state.cancelWaiters, id)
	}
	return firstErr
}

func (d *Dispatcher) finishShutdownRun(runID string, outputUnverified bool) (Result, error) {
	record, output, err := d.deps.Store.Run(runID)
	if err != nil {
		return Result{}, err
	}
	if RunStatusTerminal(record.Status) {
		return resultFromRun(record), nil
	}
	status, reason := RunStatusInterrupted, "server interrupted"
	if record.CancelRequested {
		status, reason = RunStatusCancelled, record.Error
	}
	won, err := d.deps.Store.FinishRun(runID, RunFinish{
		Status: status, Error: reason, OutputTail: output.OutputTail,
		OutputUnverified: outputUnverified, FinishedAt: time.Now().UTC(),
	})
	if err != nil {
		return Result{}, err
	}
	if won {
		record.Status, record.Error = status, reason
		return resultFromRun(record), nil
	}
	record, _, err = d.deps.Store.Run(runID)
	if err != nil {
		return Result{}, err
	}
	return resultFromRun(record), nil
}

func (d *Dispatcher) acceptCommand(state *dispatcherState, run QueuedRun) error {
	plugin := run.Request.PluginName
	if plugin == "" {
		return fmt.Errorf("plugin command requires plugin name")
	}
	limit := d.deps.Settings.PendingPerPluginLimit()
	if limit <= 0 {
		return fmt.Errorf("plugin command pending limit must be positive")
	}
	if len(state.commands[plugin])+state.cancellationCount(plugin) >= limit {
		return fmt.Errorf("plugin %q command queue is full (max %d pending)", plugin, limit)
	}

	paramsJSON, err := json.Marshal(run.Invocation.ParamView)
	if err != nil {
		return err
	}
	argvJSON, err := json.Marshal(run.Invocation.RedactedArgv)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	actorless := run.Request.ActorUserID == nil
	if err := d.deps.Store.CreateRun(RunRecord{
		ID: run.RunID, PluginName: plugin, CommandName: run.Request.Declaration.Name,
		ParamsJSON: string(paramsJSON), Status: RunStatusQueued,
		ActorlessAtSubmission: actorless, CreatedByUserID: copyUint(run.Request.ActorUserID), CreatedAt: now,
	}, RunOutput{RunID: run.RunID, ArgvJSON: string(argvJSON), CreatedAt: now}); err != nil {
		return err
	}

	state.commands[plugin] = append(state.commands[plugin], run)
	if !state.commandSeen[plugin] {
		state.commandSeen[plugin] = true
		state.commandPlugins = append(state.commandPlugins, plugin)
	}
	return nil
}

func (s *dispatcherState) cancellationCount(plugin string) int {
	count := 0
	for _, cancellation := range s.queuedCancellations {
		if cancellation.plugin == plugin {
			count++
		}
	}
	return count
}

func (d *Dispatcher) acceptImport(state *dispatcherState, request importSubmission) error {
	item := request.item
	plugin := item.spec.PluginName
	if plugin == "" || item.spec.ImportID == "" {
		return fmt.Errorf("plugin import requires plugin name and import id")
	}
	limit := d.deps.Settings.PendingPerPluginLimit()
	if limit <= 0 {
		return fmt.Errorf("plugin import pending limit must be positive")
	}
	if len(state.imports[plugin]) >= limit {
		return fmt.Errorf("plugin %q import queue is full (max %d pending)", plugin, limit)
	}
	state.imports[plugin] = append(state.imports[plugin], item)
	if !state.importSeen[plugin] {
		state.importSeen[plugin] = true
		state.importPlugins = append(state.importPlugins, plugin)
	}
	return nil
}

func (d *Dispatcher) cancel(state *dispatcherState, runID, reason string, reply chan error) (bool, error) {
	if failure, ok := state.failedCommandDispatch[runID]; ok {
		failure.status = RunStatusCancelled
		failure.reason = reason
		_, err := d.persistCommandDispatchFailure(state, failure)
		return false, err
	}
	if active, ok := state.activeCommands[runID]; ok {
		state.cancelWaiters[runID] = append(state.cancelWaiters[runID], reply)
		active.cancel(errOperatorCancelled)
		return true, nil
	}
	for plugin, queue := range state.commands {
		for i := range queue {
			if queue[i].RunID != runID {
				continue
			}
			cancellation := &queuedCancellation{run: queue[i], plugin: plugin, reason: reason}
			state.commands[plugin] = append(queue[:i], queue[i+1:]...)
			state.queuedCancellations[runID] = cancellation
			_, err := d.persistQueuedCancellation(state, cancellation)
			return false, err
		}
	}
	// A durable queued row can outlive the process which owned its private
	// queue. Disable and operator cancellation still have to settle it without
	// spawning work; recovery must not be required to complete an acknowledged
	// cancellation in the current process.
	record, output, err := d.deps.Store.Run(runID)
	if err != nil {
		return false, err
	}
	if !RunStatusTerminal(record.Status) && record.CancelRequested {
		_, err = d.deps.Store.FinishRun(runID, RunFinish{
			Status: RunStatusCancelled, Error: reason, OutputTail: output.OutputTail, FinishedAt: time.Now().UTC(),
		})
		return false, err
	}
	if record.Status == RunStatusCancelled && record.CancelRequested {
		return false, nil
	}
	return false, ErrRunNotCancellable
}

func (d *Dispatcher) completeCommand(state *dispatcherState, message commandCompleted) {
	if active, ok := state.activeCommands[message.runID]; ok {
		active.cancel(nil)
		delete(state.activeCommands, message.runID)
		state.activeByPlugin[active.plugin]--
	}
	d.controls.Delete(message.runID)
	waiters := state.cancelWaiters[message.runID]
	delete(state.cancelWaiters, message.runID)
	if len(waiters) == 0 {
		return
	}
	var resultErr error
	record, _, err := d.deps.Store.Run(message.runID)
	switch {
	case err != nil:
		resultErr = fmt.Errorf("read cancelled plugin command %s: %w", message.runID, err)
	case record.Status != RunStatusCancelled:
		resultErr = fmt.Errorf("plugin command %s cancellation did not reach cancelled state (status %s): %s", message.runID, record.Status, message.outcome.Error)
	}
	for _, waiter := range waiters {
		waiter <- resultErr
	}
}

func (d *Dispatcher) schedule(state *dispatcherState) {
	for len(state.activeCommands) < maxActiveCommands {
		run, ok := state.nextCommand()
		if !ok {
			break
		}
		d.startCommand(state, run)
	}
	for len(state.activeImports) < maxActiveImports {
		item, ok := state.nextImport()
		if !ok {
			break
		}
		d.startImport(state, item)
	}
}

func (s *dispatcherState) nextCommand() (QueuedRun, bool) {
	if len(s.commandPlugins) == 0 {
		return QueuedRun{}, false
	}
	for checked := 0; checked < len(s.commandPlugins); checked++ {
		index := (s.commandCursor + checked) % len(s.commandPlugins)
		plugin := s.commandPlugins[index]
		queue := s.commands[plugin]
		if len(queue) == 0 || s.activeByPlugin[plugin] >= maxActiveCommandsPerPlugin {
			continue
		}
		run := queue[0]
		s.commands[plugin] = queue[1:]
		s.commandCursor = (index + 1) % len(s.commandPlugins)
		return run, true
	}
	return QueuedRun{}, false
}

func (s *dispatcherState) nextImport() (queuedImport, bool) {
	if len(s.importPlugins) == 0 {
		return queuedImport{}, false
	}
	for checked := 0; checked < len(s.importPlugins); checked++ {
		index := (s.importCursor + checked) % len(s.importPlugins)
		plugin := s.importPlugins[index]
		queue := s.imports[plugin]
		if len(queue) == 0 {
			continue
		}
		item := queue[0]
		s.imports[plugin] = queue[1:]
		s.importCursor = (index + 1) % len(s.importPlugins)
		return item, true
	}
	return queuedImport{}, false
}

func (d *Dispatcher) startCommand(state *dispatcherState, run QueuedRun) {
	execCtx, cancel := context.WithCancelCause(context.Background())
	state.activeCommands[run.RunID] = activeCommand{plugin: run.Request.PluginName, cancel: cancel}
	state.activeByPlugin[run.Request.PluginName]++

	_, err := d.deps.Jobs.SubmitCommandJob(RunJobSpec{
		RunID: run.RunID, PluginName: run.Request.PluginName, OwnerUserID: copyUint(run.Request.ActorUserID),
	}, func(reason string) error {
		return d.Cancel(run.RunID, reason)
	}, func(liveCtx context.Context, progress Progress) Outcome {
		if !d.beginWorker() {
			return Outcome{Status: RunStatusInterrupted, Error: "server interrupted"}
		}
		workerDone := false
		defer func() {
			if !workerDone {
				d.workers.Done()
			}
		}()
		stop := context.AfterFunc(liveCtx, func() { cancel(context.Cause(liveCtx)) })
		outcome := d.deps.Executor.Execute(execCtx, run)
		stop()
		result := Result{OK: outcome.Status == RunStatusSucceeded, Error: outcome.Error, RunID: run.RunID}
		if record, _, err := d.deps.Store.Run(run.RunID); err == nil && RunStatusTerminal(record.Status) {
			result = resultFromRun(record)
		}
		deliverCompletion(run.Request.Completion, result)
		d.workers.Done()
		workerDone = true
		d.post(commandCompleted{runID: run.RunID, outcome: outcome})
		return outcome
	})
	if err == nil {
		return
	}

	cancel(err)
	failure := &commandDispatchFailure{
		run: run, dispatchErr: err, status: RunStatusFailed, reason: err.Error(),
	}
	state.failedCommandDispatch[run.RunID] = failure
	_, _ = d.persistCommandDispatchFailure(state, failure)
}

func (d *Dispatcher) startImport(state *dispatcherState, item queuedImport) {
	runCtx, cancel := context.WithCancelCause(context.Background())
	state.activeImports[item.spec.ImportID] = activeImport{cancel: cancel, release: item.release}
	_, err := d.deps.Jobs.SubmitImportJob(item.spec, func(liveCtx context.Context, progress Progress) Outcome {
		defer func() {
			if item.cleanup != nil {
				item.cleanup()
			}
			if item.release != nil {
				item.release()
			}
		}()
		if !d.beginWorker() {
			return Outcome{Status: ImportStatusInterrupted, Error: "server interrupted"}
		}
		workerDone := false
		defer func() {
			if !workerDone {
				d.workers.Done()
			}
		}()
		stop := context.AfterFunc(liveCtx, func() { cancel(context.Cause(liveCtx)) })
		outcome := item.run(runCtx, progress)
		stop()
		d.workers.Done()
		workerDone = true
		d.post(importCompleted{importID: item.spec.ImportID})
		return outcome
	})
	if err == nil {
		return
	}
	cancel(err)
	failure := &importDispatchFailure{
		item: item, dispatchErr: err, status: ImportStatusFailed, reason: err.Error(),
	}
	state.failedImportDispatch[item.spec.ImportID] = failure
	_, _ = d.persistImportDispatchFailure(state, failure)
}

func (d *Dispatcher) retryDispatchFailures(state *dispatcherState) {
	for _, cancellation := range state.queuedCancellations {
		_, _ = d.persistQueuedCancellation(state, cancellation)
	}
	for _, failure := range state.failedCommandDispatch {
		_, _ = d.persistCommandDispatchFailure(state, failure)
	}
	for _, failure := range state.failedImportDispatch {
		_, _ = d.persistImportDispatchFailure(state, failure)
	}
}

func (d *Dispatcher) persistQueuedCancellation(state *dispatcherState, cancellation *queuedCancellation) (bool, error) {
	finish := RunFinish{Status: RunStatusCancelled, Error: cancellation.reason, FinishedAt: time.Now().UTC()}
	won, err := d.deps.Store.FinishRun(cancellation.run.RunID, finish)
	if err != nil {
		d.deps.Logf("finish queued plugin command %s cancellation: %v", cancellation.run.RunID, err)
		return false, err
	}
	result := Result{RunID: cancellation.run.RunID, Error: cancellation.reason}
	if !won {
		record, _, err := d.deps.Store.Run(cancellation.run.RunID)
		if err != nil {
			d.deps.Logf("read queued plugin command %s after lost cancellation: %v", cancellation.run.RunID, err)
			return false, err
		}
		if !RunStatusTerminal(record.Status) {
			return false, nil
		}
		result = resultFromRun(record)
	}
	delete(state.queuedCancellations, cancellation.run.RunID)
	d.controls.Delete(cancellation.run.RunID)
	deliverCompletion(cancellation.run.Request.Completion, result)
	return true, nil
}

func (d *Dispatcher) persistCommandDispatchFailure(state *dispatcherState, failure *commandDispatchFailure) (bool, error) {
	if failure.status == RunStatusFailed && !failure.markedRunning {
		won, err := d.deps.Store.MarkRunRunning(failure.run.RunID, time.Now().UTC())
		if err != nil {
			d.deps.Logf("mark plugin command %s dispatch failure running: %v", failure.run.RunID, err)
			return false, err
		}
		if won {
			failure.markedRunning = true
		} else {
			record, _, err := d.deps.Store.Run(failure.run.RunID)
			if err != nil {
				d.deps.Logf("read plugin command %s after lost dispatch transition: %v", failure.run.RunID, err)
				return false, err
			}
			if RunStatusTerminal(record.Status) {
				d.releaseCommandDispatchFailure(state, failure, resultFromRun(record))
				return true, nil
			}
			if record.Status == RunStatusRunning {
				failure.markedRunning = true
			} else {
				return false, nil
			}
		}
	}

	finish := RunFinish{Status: failure.status, Error: failure.reason, FinishedAt: time.Now().UTC()}
	won, err := d.deps.Store.FinishRun(failure.run.RunID, finish)
	if err != nil {
		d.deps.Logf("finish plugin command %s dispatch failure: %v", failure.run.RunID, err)
		return false, err
	}
	if !won {
		record, _, err := d.deps.Store.Run(failure.run.RunID)
		if err != nil {
			d.deps.Logf("read plugin command %s after lost dispatch finish: %v", failure.run.RunID, err)
			return false, err
		}
		if !RunStatusTerminal(record.Status) {
			return false, nil
		}
		d.releaseCommandDispatchFailure(state, failure, resultFromRun(record))
		return true, nil
	}

	d.releaseCommandDispatchFailure(state, failure, Result{RunID: failure.run.RunID, Error: failure.reason})
	return true, nil
}

func (d *Dispatcher) releaseCommandDispatchFailure(state *dispatcherState, failure *commandDispatchFailure, result Result) {
	delete(state.failedCommandDispatch, failure.run.RunID)
	d.controls.Delete(failure.run.RunID)
	if active, ok := state.activeCommands[failure.run.RunID]; ok {
		active.cancel(nil)
		delete(state.activeCommands, failure.run.RunID)
		state.activeByPlugin[active.plugin]--
	}
	deliverCompletion(failure.run.Request.Completion, result)
}

func (d *Dispatcher) persistImportDispatchFailure(state *dispatcherState, failure *importDispatchFailure) (bool, error) {
	// Remove host-managed bytes before publishing the terminal state. A caller
	// which observes failed must not race a later cleanup of its claim directory.
	if failure.item.cleanup != nil {
		failure.item.cleanup()
	}
	won, err := d.deps.Store.FinishImport(failure.item.spec.ImportID, ImportFinish{
		Status: failure.status, Error: failure.reason, FinishedAt: time.Now().UTC(),
	})
	if err != nil {
		d.deps.Logf("finish plugin command import %s dispatch failure: %v", failure.item.spec.ImportID, err)
		return false, err
	}
	delete(state.failedImportDispatch, failure.item.spec.ImportID)
	releaseImportItem(failure.item, ImportResult{ImportID: failure.item.spec.ImportID, Error: failure.reason})
	if active, ok := state.activeImports[failure.item.spec.ImportID]; ok {
		active.cancel(nil)
		active.release()
		delete(state.activeImports, failure.item.spec.ImportID)
	}
	if !won {
		d.deps.Logf("plugin command import %s dispatch failure was already terminal", failure.item.spec.ImportID)
	}
	return true, nil
}

func releaseImportItem(item queuedImport, result ImportResult) {
	if item.cleanup != nil {
		item.cleanup()
	}
	if item.release != nil {
		item.release()
	}
	deliverImportCompletion(item.completion, result)
}

func resultFromRun(record RunRecord) Result {
	return Result{
		OK: record.Status == RunStatusSucceeded, ExitCode: record.ExitCode,
		Error: record.Error, RunID: record.ID,
	}
}

func deliverCompletion(completion func(Result), result Result) {
	if completion != nil {
		go completion(result)
	}
}

func (d *Dispatcher) beginWorker() bool {
	d.workerMu.Lock()
	defer d.workerMu.Unlock()
	if d.workerClosing {
		return false
	}
	d.workers.Add(1)
	return true
}

func (d *Dispatcher) post(message any) {
	select {
	case d.inbox <- message:
	case <-d.done:
	}
}

func cloneCommandRequest(request CommandRequest) CommandRequest {
	request.Params = cloneStringMap(request.Params)
	request.Declaration.Argv = append([]string(nil), request.Declaration.Argv...)
	request.Declaration.SensitiveParams = append([]string(nil), request.Declaration.SensitiveParams...)
	request.ActorUserID = copyUint(request.ActorUserID)
	return request
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func copyUint(value *uint) *uint {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func newRunID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate plugin command run id: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}
