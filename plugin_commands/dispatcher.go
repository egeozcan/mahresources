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
	"time"
)

const (
	maxActiveCommands          = 4
	maxActiveCommandsPerPlugin = 2
	maxActiveImports           = 2
	dispatchFailureRetryDelay  = 100 * time.Millisecond
)

var errDispatcherStopped = errors.New("plugin command dispatcher is stopped")

type Dispatcher struct {
	deps Dependencies

	mu      sync.Mutex
	started bool
	inbox   chan any
	done    chan struct{}
}

type commandSubmission struct {
	run   QueuedRun
	reply chan error
}

type importSubmission struct {
	spec  ImportJobSpec
	run   func(context.Context, Progress) Outcome
	reply chan error
}

type cancelSubmission struct {
	runID  string
	reason string
	reply  chan error
}

type stopDispatcher struct{ reply chan struct{} }
type commandCompleted struct{ runID string }
type importCompleted struct{ importID string }

type queuedImport struct {
	spec ImportJobSpec
	run  func(context.Context, Progress) Outcome
}

type activeCommand struct {
	plugin string
	cancel context.CancelFunc
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
	activeImports         map[string]context.CancelFunc
	activeByPlugin        map[string]int
	failedCommandDispatch map[string]*commandDispatchFailure
	failedImportDispatch  map[string]*importDispatchFailure
	queuedCancellations   map[string]*queuedCancellation
}

func NewDispatcher(deps Dependencies) *Dispatcher {
	if deps.Logf == nil {
		deps.Logf = func(string, ...any) {}
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
	reply := make(chan struct{})
	if err := d.send(ctx, stopDispatcher{reply: reply}); err != nil {
		if errors.Is(err, errDispatcherStopped) {
			return nil
		}
		return err
	}
	select {
	case <-reply:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-d.done:
		return nil
	}
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
	submission := commandSubmission{
		run:   QueuedRun{RunID: runID, Request: request, ExchangeDir: exchangeDir, Invocation: invocation},
		reply: make(chan error, 1),
	}
	if err := d.send(context.Background(), submission); err != nil {
		return "", err
	}
	if err := awaitDispatcherReply(submission.reply, d.done); err != nil {
		return "", err
	}
	return runID, nil
}

func (d *Dispatcher) Cancel(runID, reason string) error {
	request := cancelSubmission{runID: runID, reason: reason, reply: make(chan error, 1)}
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
	request := importSubmission{spec: spec, run: run, reply: make(chan error, 1)}
	if err := d.send(context.Background(), request); err != nil {
		return err
	}
	return awaitDispatcherReply(request.reply, d.done)
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
		activeImports:         make(map[string]context.CancelFunc),
		activeByPlugin:        make(map[string]int),
		failedCommandDispatch: make(map[string]*commandDispatchFailure),
		failedImportDispatch:  make(map[string]*importDispatchFailure),
		queuedCancellations:   make(map[string]*queuedCancellation),
	}

	for {
		select {
		case <-ctx.Done():
			state.cancelActive()
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
				message.reply <- d.cancel(&state, message.runID, message.reason)
			case commandCompleted:
				if active, ok := state.activeCommands[message.runID]; ok {
					active.cancel()
					delete(state.activeCommands, message.runID)
					state.activeByPlugin[active.plugin]--
				}
			case importCompleted:
				if cancel, ok := state.activeImports[message.importID]; ok {
					cancel()
					delete(state.activeImports, message.importID)
				}
			case stopDispatcher:
				state.cancelActive()
				close(message.reply)
				return
			}
			d.schedule(&state)
		}
	}
}

func (s *dispatcherState) cancelActive() {
	for _, active := range s.activeCommands {
		active.cancel()
	}
	for _, cancel := range s.activeImports {
		cancel()
	}
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
	plugin := request.spec.PluginName
	if plugin == "" || request.spec.ImportID == "" {
		return fmt.Errorf("plugin import requires plugin name and import id")
	}
	limit := d.deps.Settings.PendingPerPluginLimit()
	if limit <= 0 {
		return fmt.Errorf("plugin import pending limit must be positive")
	}
	if len(state.imports[plugin]) >= limit {
		return fmt.Errorf("plugin %q import queue is full (max %d pending)", plugin, limit)
	}
	state.imports[plugin] = append(state.imports[plugin], queuedImport{spec: request.spec, run: request.run})
	if !state.importSeen[plugin] {
		state.importSeen[plugin] = true
		state.importPlugins = append(state.importPlugins, plugin)
	}
	return nil
}

func (d *Dispatcher) cancel(state *dispatcherState, runID, reason string) error {
	if err := d.deps.Store.RequestRunCancel(runID, reason); err != nil {
		return err
	}
	if failure, ok := state.failedCommandDispatch[runID]; ok {
		failure.status = RunStatusCancelled
		failure.reason = reason
		_, err := d.persistCommandDispatchFailure(state, failure)
		return err
	}
	if active, ok := state.activeCommands[runID]; ok {
		active.cancel()
		return nil
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
			return err
		}
	}
	return ErrRunNotCancellable
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
	execCtx, cancel := context.WithCancel(context.Background())
	state.activeCommands[run.RunID] = activeCommand{plugin: run.Request.PluginName, cancel: cancel}
	state.activeByPlugin[run.Request.PluginName]++

	_, err := d.deps.Jobs.SubmitCommandJob(RunJobSpec{
		RunID: run.RunID, PluginName: run.Request.PluginName, OwnerUserID: copyUint(run.Request.ActorUserID),
	}, func(reason string) error {
		return d.Cancel(run.RunID, reason)
	}, func(liveCtx context.Context, progress Progress) Outcome {
		stop := context.AfterFunc(liveCtx, cancel)
		outcome := d.deps.Executor.Execute(execCtx, run)
		stop()
		deliverCompletion(run.Request.Completion, Result{OK: outcome.Status == RunStatusSucceeded, Error: outcome.Error, RunID: run.RunID})
		d.post(commandCompleted{runID: run.RunID})
		return outcome
	})
	if err == nil {
		return
	}

	cancel()
	failure := &commandDispatchFailure{
		run: run, dispatchErr: err, status: RunStatusFailed, reason: err.Error(),
	}
	state.failedCommandDispatch[run.RunID] = failure
	_, _ = d.persistCommandDispatchFailure(state, failure)
}

func (d *Dispatcher) startImport(state *dispatcherState, item queuedImport) {
	runCtx, cancel := context.WithCancel(context.Background())
	state.activeImports[item.spec.ImportID] = cancel
	_, err := d.deps.Jobs.SubmitImportJob(item.spec, func(liveCtx context.Context, progress Progress) Outcome {
		stop := context.AfterFunc(liveCtx, cancel)
		outcome := item.run(runCtx, progress)
		stop()
		d.post(importCompleted{importID: item.spec.ImportID})
		return outcome
	})
	if err == nil {
		return
	}
	cancel()
	failure := &importDispatchFailure{item: item, dispatchErr: err}
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
	if active, ok := state.activeCommands[failure.run.RunID]; ok {
		active.cancel()
		delete(state.activeCommands, failure.run.RunID)
		state.activeByPlugin[active.plugin]--
	}
	deliverCompletion(failure.run.Request.Completion, result)
}

func (d *Dispatcher) persistImportDispatchFailure(state *dispatcherState, failure *importDispatchFailure) (bool, error) {
	won, err := d.deps.Store.FinishImport(failure.item.spec.ImportID, ImportFinish{
		Status: ImportStatusFailed, Error: failure.dispatchErr.Error(), FinishedAt: time.Now().UTC(),
	})
	if err != nil {
		d.deps.Logf("finish plugin command import %s dispatch failure: %v", failure.item.spec.ImportID, err)
		return false, err
	}
	delete(state.failedImportDispatch, failure.item.spec.ImportID)
	if cancel, ok := state.activeImports[failure.item.spec.ImportID]; ok {
		cancel()
		delete(state.activeImports, failure.item.spec.ImportID)
	}
	if !won {
		d.deps.Logf("plugin command import %s dispatch failure was already terminal", failure.item.spec.ImportID)
	}
	return true, nil
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
