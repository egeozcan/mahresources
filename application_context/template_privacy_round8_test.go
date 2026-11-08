package application_context

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/shortcodes"
)

// Every PRESENT instant becomes the stand-in, the zero instant included; nil
// stays nil (round 8, #1).
func TestZeroInstantsAreMaskedLikeAnyOther(t *testing.T) {
	p := NewTemplatePrivacy("zt")
	var zero time.Time
	note := &models.Note{StartDate: &zero, EndDate: nil}
	out, err := p.Entity(context.Background(), note)
	if err != nil {
		t.Fatal(err)
	}
	masked := out.(*models.Note)
	if masked.StartDate == nil || !masked.StartDate.Equal(templatePrivacyStandIn) {
		t.Fatalf("a present zero StartDate must become the stand-in, got %v", masked.StartDate)
	}
	if masked.EndDate != nil {
		t.Fatal("a nil date stays nil")
	}
	if !masked.CreatedAt.Equal(templatePrivacyStandIn) {
		t.Fatalf("a value-typed zero time field is a present instant too: %v", masked.CreatedAt)
	}
	// The original is enrolled: it cannot be sent below the masker.
	rt := &capturingTransport{}
	conv := NewGenerationConversationProvider("deepseek", "key", "m", &http.Client{Transport: rt})
	_, err = conv.Converse(WithTemplatePrivacy(context.Background(), p), transportTestRequest("stored 0001-01-01T00:00:00Z here"))
	if err == nil || rt.count() != 0 {
		t.Fatalf("the zero instant's original must be guarded (err=%v calls=%d)", err, rt.count())
	}
}

// The scalar-format hook reports text made from numbers, booleans and times only:
// never a container, never a string (round 8, #2).
func TestScalarObserverNeverRegistersContainersOrStrings(t *testing.T) {
	const marker = "PRIVATE_MARKER_9321"
	p := NewTemplatePrivacy("so")
	p.Text(marker)
	ctx := shortcodes.WithScalarFormatObserver(context.Background(), func(s string) { p.Keep(s) })
	meta := []byte(`{"object":{"value":"` + marker + `"},"list":["` + marker + `"],"when":"2024-05-06T07:08:09Z","n":1234567}`)
	mc := shortcodes.MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: meta}
	rendered := shortcodes.Process(ctx, `[meta path="object" inline="true"]|[meta path="list" inline="true"]|[meta path="when" inline="true" format="date"]`, mc, nil, nil)
	if !strings.Contains(rendered, marker) {
		t.Fatalf("the injected original is rendered, as a below-mask miss would:\n%s", rendered)
	}
	rt := &capturingTransport{}
	conv := NewGenerationConversationProvider("deepseek", "key", "m", &http.Client{Transport: rt})
	_, err := conv.Converse(WithTemplatePrivacy(context.Background(), p), transportTestRequest(rendered))
	if err == nil || rt.count() != 0 {
		t.Fatalf("a container rendered from a below-mask miss must not be exempted (err=%v calls=%d)", err, rt.count())
	}

	// Positive controls: formatted numbers and times are still readable.
	q := NewTemplatePrivacy("sq")
	q.Text("1,177 KiB")
	q.Text("2001-02-03")
	qctx := shortcodes.WithScalarFormatObserver(context.Background(), func(s string) { q.Keep(s) })
	out := shortcodes.Process(qctx, `[meta path="size" inline="true" format="filesize"]|[property path="CreatedAt" format="date"]`,
		shortcodes.MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: []byte(`{"size":1205000}`), Entity: &models.Group{CreatedAt: templatePrivacyStandIn}}, nil, nil)
	rt2 := &capturingTransport{}
	conv2 := NewGenerationConversationProvider("deepseek", "key", "m", &http.Client{Transport: rt2})
	if _, err := conv2.Converse(WithTemplatePrivacy(context.Background(), q), transportTestRequest(out)); err != nil && rt2.count() == 0 {
		t.Fatalf("formatted numbers and the stand-in date stay readable: %v (%s)", err, out)
	}
}
