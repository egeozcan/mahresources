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

type dispatcherState struct {
	commands map[string][]QueuedRun
	imports  map[string][]queuedImport

	commandPlugins []string
	importPlugins  []string
	commandSeen    map[string]bool
	importSeen     map[string]bool
	commandCursor  int
	importCursor   int

	activeCommands map[string]activeCommand
	activeImports  map[string]context.CancelFunc
	activeByPlugin map[string]int
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
	select {
	case err := <-submission.reply:
		if err != nil {
			return "", err
		}
		return runID, nil
	case <-d.done:
		return "", errDispatcherStopped
	}
}

func (d *Dispatcher) Cancel(runID, reason string) error {
	request := cancelSubmission{runID: runID, reason: reason, reply: make(chan error, 1)}
	if err := d.send(context.Background(), request); err != nil {
		return err
	}
	select {
	case err := <-request.reply:
		return err
	case <-d.done:
		return errDispatcherStopped
	}
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
	select {
	case err := <-request.reply:
		return err
	case <-d.done:
		return errDispatcherStopped
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
	state := dispatcherState{
		commands:       make(map[string][]QueuedRun),
		imports:        make(map[string][]queuedImport),
		commandSeen:    make(map[string]bool),
		importSeen:     make(map[string]bool),
		activeCommands: make(map[string]activeCommand),
		activeImports:  make(map[string]context.CancelFunc),
		activeByPlugin: make(map[string]int),
	}

	for {
		select {
		case <-ctx.Done():
			state.cancelActive()
			return
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
	if len(state.commands[plugin]) >= limit {
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
	if active, ok := state.activeCommands[runID]; ok {
		active.cancel()
		return nil
	}
	for plugin, queue := range state.commands {
		for i := range queue {
			if queue[i].RunID != runID {
				continue
			}
			run := queue[i]
			state.commands[plugin] = append(queue[:i], queue[i+1:]...)
			now := time.Now().UTC()
			_, err := d.deps.Store.FinishRun(runID, RunFinish{Status: RunStatusCancelled, Error: reason, FinishedAt: now})
			if run.Request.Completion != nil {
				run.Request.Completion(Result{RunID: runID, Error: reason})
			}
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
		if run.Request.Completion != nil {
			run.Request.Completion(Result{OK: outcome.Status == RunStatusSucceeded, Error: outcome.Error, RunID: run.RunID})
		}
		d.post(commandCompleted{runID: run.RunID})
		return outcome
	})
	if err == nil {
		return
	}

	cancel()
	delete(state.activeCommands, run.RunID)
	state.activeByPlugin[run.Request.PluginName]--
	d.finishDispatchFailure(run, err)
}

func (d *Dispatcher) finishDispatchFailure(run QueuedRun, dispatchErr error) {
	now := time.Now().UTC()
	if won, err := d.deps.Store.MarkRunRunning(run.RunID, now); err != nil {
		d.deps.Logf("mark plugin command %s dispatch failure running: %v", run.RunID, err)
	} else if won {
		if _, err := d.deps.Store.FinishRun(run.RunID, RunFinish{Status: RunStatusFailed, Error: dispatchErr.Error(), FinishedAt: now}); err != nil {
			d.deps.Logf("finish plugin command %s dispatch failure: %v", run.RunID, err)
		}
	}
	if run.Request.Completion != nil {
		run.Request.Completion(Result{RunID: run.RunID, Error: dispatchErr.Error()})
	}
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
	delete(state.activeImports, item.spec.ImportID)
	now := time.Now().UTC()
	if won, markErr := d.deps.Store.MarkImportRunning(item.spec.ImportID, now); markErr != nil {
		d.deps.Logf("mark plugin command import %s dispatch failure running: %v", item.spec.ImportID, markErr)
	} else if won {
		if _, finishErr := d.deps.Store.FinishImport(item.spec.ImportID, ImportFinish{Status: ImportStatusFailed, Error: err.Error(), FinishedAt: now}); finishErr != nil {
			d.deps.Logf("finish plugin command import %s dispatch failure: %v", item.spec.ImportID, finishErr)
		}
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
