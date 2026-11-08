package application_context

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPrivacyGuardReadablePrefixNeverExemptsTheWholeValue(t *testing.T) {
	p := NewTemplatePrivacy("pf")
	prefix := strings.Repeat("public words ", 6)[:64]
	whole := prefix + "PRIVATE_SUFFIX_TEXT"
	p.Text(whole)
	p.Keep("template source: " + prefix + " and nothing more")
	if err := outbound(p, "see "+whole); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("a readable prefix must not exempt the value: %v", err)
	}
	if err := outbound(p, "see "+prefix); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("a truncated copy is still found by its prefix: %v", err)
	}
	p.Keep("the whole thing is readable too: " + whole)
	if err := outbound(p, "see "+whole); err != nil {
		t.Fatalf("the complete value in readable text exempts it: %v", err)
	}
}

func TestPrivacyGuardCorpusMatchesEscapedSpellings(t *testing.T) {
	p := NewTemplatePrivacy("es")
	p.Text(`Public heading \"quoted\"`)
	p.Keep(`Public heading "quoted"`)
	// The readable source, encoded into a Tool result, carries backslash-quote.
	if err := outbound(p, `{"source":"`+jsonStringBody(`Public heading "quoted"`, false)+`"}`); err != nil {
		t.Fatalf("an escaped copy of readable text is readable: %v", err)
	}
}

// countdownCtx reports cancellation once Err has been asked a set number of times.
type countdownCtx struct {
	context.Context
	left atomic.Int32
}

func (c *countdownCtx) Err() error {
	if c.left.Add(-1) < 0 {
		return context.Canceled
	}
	return nil
}

func TestPrivacyGuardCancelsDuringReconciliationOfManyShortEntries(t *testing.T) {
	p := NewTemplatePrivacy("cc")
	for i := 0; i < 30000; i++ {
		p.Keep(fmt.Sprintf("short %d", i))
	}
	p.Text("one guarded value")
	ctx := &countdownCtx{Context: context.Background()}
	ctx.left.Store(3)
	if err := p.CheckOutbound(ctx, []byte(`{"a":"b"}`)); !errors.Is(err, context.Canceled) {
		t.Fatalf("reconciliation over many short strings must observe cancellation: %v", err)
	}
}

func TestPrivacyGuardCancelsDuringAutomatonConstruction(t *testing.T) {
	p := NewTemplatePrivacy("cb")
	for i := 0; i < 20000; i++ {
		p.Text(fmt.Sprintf("guarded value number %d with some padding", i))
	}
	ctx := &countdownCtx{Context: context.Background()}
	ctx.left.Store(3)
	if err := p.CheckOutbound(ctx, []byte(`{"a":"b"}`)); !errors.Is(err, context.Canceled) {
		t.Fatalf("automaton construction must observe cancellation: %v", err)
	}
}

func TestPrivacyGuardCancelsAcrossManyShortPayloadLeaves(t *testing.T) {
	p := NewTemplatePrivacy("cl")
	p.Text("one guarded value")
	if err := p.CheckOutbound(context.Background(), []byte(`{"a":"b"}`)); err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	body.WriteString(`{"messages":[`)
	for i := 0; i < 20000; i++ {
		if i > 0 {
			body.WriteByte(',')
		}
		body.WriteString(`{"content":"tiny"}`)
	}
	body.WriteString(`]}`)
	ctx := &countdownCtx{Context: context.Background()}
	ctx.left.Store(2)
	if err := p.CheckOutbound(ctx, []byte(body.String())); !errors.Is(err, context.Canceled) {
		t.Fatalf("a payload of many short strings must observe cancellation: %v", err)
	}
}
