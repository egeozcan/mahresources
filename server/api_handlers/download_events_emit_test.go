package api_handlers

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/auth"
	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/plugin_system"
)

// A legacy job stream writes a frame in exactly one place, the emit closure,
// which takes the credential check after the frame's projection and just before
// the write. A second write path is how the frames after a slow read or a
// stalled flush kept going out on an old answer, one path at a time.
func TestEveryLegacyStreamWriteGoesThroughEmit(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "download_queue_handlers.go", nil, 0)
	if err != nil {
		t.Fatalf("parse the handler: %v", err)
	}
	var handler *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "GetDownloadEventsHandler" {
			handler = fn
		}
	}
	if handler == nil {
		t.Fatal("GetDownloadEventsHandler not found")
	}
	var emit *ast.FuncLit
	ast.Inspect(handler, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		if ident, ok := assign.Lhs[0].(*ast.Ident); ok && ident.Name == "emit" {
			if lit, ok := assign.Rhs[0].(*ast.FuncLit); ok {
				emit = lit
			}
		}
		return true
	})
	if emit == nil {
		t.Fatal("GetDownloadEventsHandler has no emit closure")
	}
	writes := 0
	ast.Inspect(handler, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver, _ := selector.X.(*ast.Ident)
		if receiver == nil {
			return true
		}
		isWrite := false
		switch {
		case receiver.Name == "fmt" && strings.HasPrefix(selector.Sel.Name, "Fprint"):
			if len(call.Args) > 0 {
				if target, ok := call.Args[0].(*ast.Ident); ok && target.Name == "writer" {
					isWrite = true
				}
			}
		case receiver.Name == "writer" && selector.Sel.Name == "Write":
			isWrite = true
		case receiver.Name == "flusher" && selector.Sel.Name == "Flush":
			isWrite = true
		}
		if !isWrite {
			return true
		}
		writes++
		if call.Pos() < emit.Pos() || call.End() > emit.End() {
			t.Errorf("%s writes to the stream outside emit", fset.Position(call.Pos()))
		}
		return true
	})
	if writes == 0 {
		t.Fatal("found no stream write at all; the guard is not looking at the right code")
	}
}

// streamTimeline records, in order, the credential checks a legacy stream
// takes, the projection reads it makes and the frames it writes.
type streamTimeline struct {
	mu     sync.Mutex
	events []string
}

func (tl *streamTimeline) add(event string) {
	tl.mu.Lock()
	tl.events = append(tl.events, event)
	tl.mu.Unlock()
}

func (tl *streamTimeline) snapshot() []string {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	return append([]string(nil), tl.events...)
}

type recordingEventsSource struct {
	inner    *changingJobEventsSource
	timeline *streamTimeline
}

func (s *recordingEventsSource) CurrentJobEvents() (JobEventsContext, error) {
	s.timeline.add("check")
	return s.inner.CurrentJobEvents()
}

// recordingEventsContext records every projection read the handler makes
// through it.
type recordingEventsContext struct {
	*durableActionEventsContext
	timeline *streamTimeline
}

func (c *recordingEventsContext) ProjectDownloadQueue() ([]*download_queue.DownloadJob, error) {
	c.timeline.add("project")
	return c.durableActionEventsContext.ProjectDownloadQueue()
}

func (c *recordingEventsContext) ProjectDownloadJob(id string) (download_queue.DownloadProjection, error) {
	c.timeline.add("project")
	return c.durableActionEventsContext.ProjectDownloadJob(id)
}

func (c *recordingEventsContext) ProjectActionJobs() ([]*plugin_system.ActionJob, error) {
	c.timeline.add("project")
	return c.durableActionEventsContext.ProjectActionJobs()
}

type recordingSSEWriter struct {
	*sseTestWriter
	timeline *streamTimeline
}

func (w *recordingSSEWriter) Write(data []byte) (int, error) {
	first := string(data)
	if newline := strings.IndexByte(first, '\n'); newline >= 0 {
		first = first[:newline]
	}
	w.timeline.add("write " + first)
	return w.sseTestWriter.Write(data)
}

// Whatever a frame is (the initial state, a queue event, a durable poll row, a
// removal), a credential check comes between the last projection read and the
// write.
func TestEveryLegacyStreamFrameIsWrittenOnACheckTakenAfterItsProjection(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)
	timeline := &streamTimeline{}
	other := uint(8)
	stub := &legacyJobEventsContextStub{manager: manager}
	ctx := &recordingEventsContext{
		durableActionEventsContext: &durableActionEventsContext{
			principalJobEventsContext: &principalJobEventsContext{stub, &auth.Principal{UserID: 7, Role: models.RoleAdmin}},
		},
		timeline: timeline,
	}
	inner := &changingJobEventsSource{}
	inner.set(ctx)
	source := &recordingEventsSource{inner: inner, timeline: timeline}

	writer := &recordingSSEWriter{sseTestWriter: newSSETestWriter(), timeline: timeline}
	requestCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events", nil).WithContext(requestCtx)
	finished := make(chan struct{})
	go func() {
		GetDownloadEventsHandler(source)(writer, request)
		close(finished)
	}()
	t.Cleanup(func() {
		cancel()
		<-finished
	})
	select {
	case <-writer.initWritten:
	case <-time.After(2 * time.Second):
		t.Fatal("the legacy stream did not send its initial state")
	}

	for i := 0; i < 3; i++ {
		submitOwnedLegacyJob(t, manager, other)
	}
	ctx.mu.Lock()
	ctx.rows = []*plugin_system.ActionJob{plugin_system.ProjectedActionJob{
		Handle: "polled", CanonicalJobID: "job-polled", Plugin: "p", ActionID: "a", Status: "running", Owner: &other,
	}.ActionJob()}
	ctx.mu.Unlock()
	if !waitForBody(writer.sseTestWriter, "event: action_added", 5*time.Second) {
		t.Fatalf("the durable poll wrote no row: %s", writer.String())
	}
	ctx.mu.Lock()
	ctx.rows = nil
	ctx.mu.Unlock()
	if !waitForBody(writer.sseTestWriter, "event: action_removed", 5*time.Second) {
		t.Fatalf("the durable poll wrote no removal: %s", writer.String())
	}

	events := timeline.snapshot()
	projectedSinceCheck := false
	kinds := map[string]bool{}
	for i, event := range events {
		switch {
		case event == "project":
			projectedSinceCheck = true
		case event == "check":
			projectedSinceCheck = false
		case strings.HasPrefix(event, "write "):
			kinds[strings.TrimPrefix(event, "write ")] = true
			if projectedSinceCheck {
				t.Fatalf("%q at %d followed a projection read with no credential check between them: %v", event, i, events)
			}
		}
	}
	for _, kind := range []string{"event: init", "event: added", "event: action_added", "event: action_removed"} {
		if !kinds[kind] {
			t.Fatalf("no %q frame was written, so the invariant was not exercised for it: %v", kind, events)
		}
	}
}

// blockingQueueContext holds its first queue projection until released, the way
// the initial state's read waits on a slow database.
type blockingQueueContext struct {
	*principalJobEventsContext
	blocked, release chan struct{}
	once             *sync.Once
}

func (c *blockingQueueContext) ProjectDownloadQueue() ([]*download_queue.DownloadJob, error) {
	c.once.Do(func() {
		close(c.blocked)
		<-c.release
	})
	return c.principalJobEventsContext.ProjectDownloadQueue()
}

// The initial state is a frame like any other: projected for an administrator,
// then outlasted by a demotion, it is projected again before it is sent.
func TestTheInitialStateProjectedBeforeADemotionIsNotSentAfterIt(t *testing.T) {
	manager := download_queue.NewDownloadManager(nil, download_queue.TimeoutConfig{})
	t.Cleanup(manager.Shutdown)
	hidden := &download_queue.DownloadJob{ID: "admin-visible-row", Status: download_queue.JobStatusPending}
	blocked, release := make(chan struct{}), make(chan struct{})
	once := &sync.Once{}
	admin := &blockingQueueContext{&principalJobEventsContext{
		&legacyJobEventsContextStub{manager: manager, rows: []*download_queue.DownloadJob{hidden}},
		&auth.Principal{UserID: 7, Role: models.RoleAdmin},
	}, blocked, release, once}
	demoted := &blockingQueueContext{&principalJobEventsContext{
		&legacyJobEventsContextStub{manager: manager},
		&auth.Principal{UserID: 7, Role: models.RoleUser},
	}, blocked, release, once}
	source := &changingJobEventsSource{}
	source.set(admin)

	response := newSSETestWriter()
	requestCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events", nil).WithContext(requestCtx)
	finished := make(chan struct{})
	go func() {
		GetDownloadEventsHandler(source)(response, request)
		close(finished)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-release:
		default:
			close(release)
		}
		<-finished
	})

	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("the initial state was never projected")
	}
	source.set(demoted)
	close(release)
	select {
	case <-response.initWritten:
	case <-time.After(2 * time.Second):
		t.Fatal("the legacy stream did not send its initial state")
	}
	if strings.Contains(response.String(), hidden.ID) {
		t.Fatalf("the initial state projected for the administrator was sent after the demotion: %s", response.String())
	}
}
