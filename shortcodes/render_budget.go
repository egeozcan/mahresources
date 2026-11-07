package shortcodes

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
)

// ErrRenderBudgetExceeded ends a render that went past its RenderBudget, its
// schema-work budget, or was cancelled. A cancelled render wraps the context's
// own error too, so errors.Is(err, context.Canceled) also holds.
var ErrRenderBudgetExceeded = errors.New("shortcode render budget exceeded")

// ErrQueryUnavailable is what a query executor returns when [mrql] may not run
// (the preview's MRQL opt-in is off). Handlers render it as an inert comment,
// never as "no match".
var ErrQueryUnavailable = errors.New("mrql queries are unavailable in this render")

// RenderBudget bounds one render: the bytes it may produce and the bytes of
// source (templates, partials, plugin output) it may parse. Zero means no limit.
type RenderBudget struct {
	OutputBytes       int
	ParsedSourceBytes int
}

type renderBudgetKey struct{}

// WithRenderBudget returns ctx carrying b. ProcessChecked enforces it: each
// top-level render gets its own meter, so the limits are per render.
func WithRenderBudget(ctx context.Context, b RenderBudget) context.Context {
	return context.WithValue(ctx, renderBudgetKey{}, b)
}

// RenderBudgetFrom returns the budget on ctx, if any.
func RenderBudgetFrom(ctx context.Context) (RenderBudget, bool) {
	if ctx == nil {
		return RenderBudget{}, false
	}
	b, ok := ctx.Value(renderBudgetKey{}).(RenderBudget)
	return b, ok
}

// Caps that apply only to a budgeted render (pages keep the unbounded parse).
const (
	budgetedTokenCap = 50_000 // shortcode tokens one parse may retain
	budgetedAttrCap  = 1_000  // attributes one token may carry
)

type renderMeterKey struct{}

// renderMeter is the live state of one render. It is sticky: once a limit is
// crossed or the context is done, err is set and every later check fails, so a
// handler that returns a plain string still ends the whole render. A nil
// *renderMeter is valid and checks nothing (handlers called directly).
type renderMeter struct {
	ctx    context.Context
	budget RenderBudget

	mu     sync.Mutex
	out    int64
	parsed int64
	steps  uint32
	err    error
}

func meterFrom(ctx context.Context) *renderMeter {
	if ctx == nil {
		return nil
	}
	m, _ := ctx.Value(renderMeterKey{}).(*renderMeter)
	return m
}

func newRenderMeter(ctx context.Context) *renderMeter {
	b, _ := RenderBudgetFrom(ctx)
	return &renderMeter{ctx: ctx, budget: b}
}

// limited reports whether a byte budget applies (not just cancellation).
func (m *renderMeter) limited() bool {
	return m != nil && (m.budget.OutputBytes > 0 || m.budget.ParsedSourceBytes > 0)
}

func (m *renderMeter) failLocked(err error) {
	if m.err == nil {
		m.err = err
	}
}

// fail records err (wrapped as a budget error) unless one is already recorded.
func (m *renderMeter) fail(format string, args ...any) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.failLocked(fmt.Errorf("%w: %s", ErrRenderBudgetExceeded, fmt.Sprintf(format, args...)))
	m.mu.Unlock()
}

// Err is the sticky error, nil while the render may continue.
func (m *renderMeter) Err() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.err
}

func (m *renderMeter) checkCtxLocked() {
	if m.err == nil && m.ctx != nil {
		if cerr := m.ctx.Err(); cerr != nil {
			m.err = fmt.Errorf("%w: %w", ErrRenderBudgetExceeded, cerr)
		}
	}
}

// alive reports whether the render may continue, checking the context.
func (m *renderMeter) alive() bool {
	if m == nil {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkCtxLocked()
	return m.err == nil
}

// tick is the cheap per-iteration check for inner loops: it consults the
// context every 256th call and is otherwise a counter bump.
func (m *renderMeter) tick() bool {
	if m == nil {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.steps++
	if m.steps&0xff == 0 {
		m.checkCtxLocked()
	}
	return m.err == nil
}

// chargeOut reserves n bytes of output before they are appended. n may be the
// result of caller arithmetic, so negative values are refused outright.
func (m *renderMeter) chargeOut(n int64) bool {
	if m == nil {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkCtxLocked()
	if m.err != nil {
		return false
	}
	if n < 0 || n > math.MaxInt64-m.out {
		m.failLocked(fmt.Errorf("%w: output size overflow", ErrRenderBudgetExceeded))
		return false
	}
	m.out += n
	if lim := int64(m.budget.OutputBytes); lim > 0 && m.out > lim {
		m.failLocked(fmt.Errorf("%w: output exceeds %d bytes", ErrRenderBudgetExceeded, lim))
		return false
	}
	return true
}

// chargeParsed reserves n bytes of template source before it is parsed.
func (m *renderMeter) chargeParsed(n int64) bool {
	if m == nil {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkCtxLocked()
	if m.err != nil {
		return false
	}
	if n < 0 || n > math.MaxInt64-m.parsed {
		m.failLocked(fmt.Errorf("%w: parsed source overflow", ErrRenderBudgetExceeded))
		return false
	}
	m.parsed += n
	if lim := int64(m.budget.ParsedSourceBytes); lim > 0 && m.parsed > lim {
		m.failLocked(fmt.Errorf("%w: parsed source exceeds %d bytes", ErrRenderBudgetExceeded, lim))
		return false
	}
	return true
}

// ProcessChecked is Process that reports a budget or cancellation stop. It
// enforces the RenderBudget on reqCtx (none means unlimited) and checks the
// context during parsing and expansion. On error the returned HTML is empty.
// A meter already on reqCtx (a nested top-level call) is shared, so one render
// is one budget.
func ProcessChecked(reqCtx context.Context, input string, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor) (string, error) {
	if reqCtx == nil {
		reqCtx = context.Background()
	}
	m := meterFrom(reqCtx)
	if m == nil {
		m = newRenderMeter(reqCtx)
		reqCtx = context.WithValue(reqCtx, renderMeterKey{}, m)
	}
	out := processWithDepth(reqCtx, input, ctx, renderer, executor, 0)
	if err := m.Err(); err != nil {
		return "", err
	}
	return out, nil
}
