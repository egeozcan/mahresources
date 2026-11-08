package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/types"
)

// Below-masker injections of the originals the source replaces by something
// other than a Placeholder are blocked before the transport sends (round 2, #1).
func TestOriginalsReplacedWithoutAPlaceholderStayGuarded(t *testing.T) {
	zone := time.FixedZone("X", 5*3600+1800)
	when := time.Date(2024, 5, 6, 7, 8, 9, 123456789, zone)
	deleted := time.Date(2023, 1, 2, 3, 4, 5, 987654321, time.UTC)
	cases := map[string]struct {
		mask  func(p *TemplatePrivacy)
		leaks []string
	}{
		"time.Time": {func(p *TemplatePrivacy) {
			_, _ = p.Entity(context.Background(), &models.Group{CreatedAt: when})
		}, []string{when.Format(time.RFC3339Nano), when.UTC().Format(time.RFC3339Nano), when.Format(time.RFC3339), when.UTC().Format(time.RFC3339)}},
		"Date": {func(p *TemplatePrivacy) { p.Date(when) }, []string{when.Format(time.RFC3339Nano), when.UTC().Format(time.RFC3339Nano)}},
		"content type parameters": {func(p *TemplatePrivacy) {
			_, _ = p.Entity(context.Background(), &models.Resource{ContentType: `image/png; name="PRIVATE_FILENAME"`})
		}, []string{`image/png; name="PRIVATE_FILENAME"`}},
		"deleted at": {func(p *TemplatePrivacy) {
			type holder struct {
				Name string
				At   *time.Time
			}
			_, _ = p.Entity(context.Background(), &holder{At: &deleted})
		}, []string{deleted.Format(time.RFC3339Nano)}},
	}
	for name, c := range cases {
		for _, provider := range []string{"deepseek", "openai", "anthropic"} {
			for _, leak := range c.leaks {
				rt := &capturingTransport{}
				conv := NewGenerationConversationProvider(provider, "key", "m", &http.Client{Transport: rt})
				p := NewTemplatePrivacy("or")
				c.mask(p)
				_, err := conv.Converse(WithTemplatePrivacy(context.Background(), p), transportTestRequest("injected: "+leak))
				if !errors.Is(err, ErrTemplatePrivacyLeak) || rt.count() != 0 {
					t.Errorf("%s/%s: %q reached the transport (err=%v calls=%d)", name, provider, leak, err, rt.count())
				}
			}
		}
	}
}

// One check's cancellation must not poison the next check on the same masker (#2),
// and an interrupted reconciliation must not leave a stale matcher (#3).
func TestGuardCancellationDoesNotPoisonLaterChecks(t *testing.T) {
	p := NewTemplatePrivacy("pz")
	for i := 0; i < 2000; i++ {
		p.Text(fmt.Sprintf("guarded value number %d with padding", i))
	}
	if err := p.CheckOutbound(context.Background(), []byte(`{"a":"clean"}`)); err != nil {
		t.Fatal(err)
	}
	// Cancel in the middle of scanning a large payload with the cached matcher.
	big := `{"a":"` + strings.Repeat("lorem ipsum ", 30000) + `"}`
	ctx := &countdownCtx{Context: context.Background()}
	ctx.left.Store(2)
	if err := p.CheckOutbound(ctx, []byte(big)); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected a cancellation: %v", err)
	}
	if err := p.CheckOutbound(context.Background(), []byte(`{"a":"clean again"}`)); err != nil {
		t.Fatalf("a later clean check on a live context must pass: %v", err)
	}
}

func TestInterruptedReconciliationLeavesNoStaleMatcher(t *testing.T) {
	p := NewTemplatePrivacy("pr")
	for i := 0; i < 300; i++ {
		p.Text(fmt.Sprintf("value filler %d xxxx", i))
	}
	p.Text("Shared Heading")
	if err := p.CheckOutbound(context.Background(), []byte(`{"a":"x"}`)); err != nil {
		t.Fatal(err)
	}
	// New readable text exempts the value, but reconciliation is cancelled after
	// that exemption was found.
	p.Keep("our Shared Heading is public")
	for n := int32(1); n < 12; n++ {
		ctx := &countdownCtx{Context: context.Background()}
		ctx.left.Store(n)
		_ = p.CheckOutbound(ctx, []byte(`{"a":"x"}`))
	}
	if err := p.CheckOutbound(context.Background(), []byte(`{"a":"Shared Heading"}`)); err != nil {
		t.Fatalf("an exempt value must not be reported through a stale matcher: %v", err)
	}
}

// The P14 case through the guard: a stored value that equals an issued
// Placeholder must not make the real Placeholder look like a leak (#6).
func TestPlaceholderCollisionPassesThroughTheGuard(t *testing.T) {
	p := NewTemplatePrivacy("p4")
	first := p.Text("secret thing")
	second := p.Text(first)
	body, _ := json.Marshal(map[string]string{"m": "see " + first + " and " + second})
	if err := p.CheckOutbound(context.Background(), body); err != nil {
		t.Fatalf("generated Placeholders are readable: %v", err)
	}
	alias := p.URLAlias("https://example.com/secret-path")
	body, _ = json.Marshal(map[string]string{"m": alias})
	if err := p.CheckOutbound(context.Background(), body); err != nil {
		t.Fatalf("the alias is readable: %v", err)
	}
}

// HTML-escaped readable vocabulary must not be mistaken for a protected value (#8),
// and an HTML-escaped protected value must still be caught (#15).
func TestHTMLEscapedSpellings(t *testing.T) {
	p := NewTemplatePrivacy("he")
	p.Text("Public &lt;heading&gt;")
	p.Keep("Public <heading>")
	if err := outbound(p, "rendered: Public &lt;heading&gt;"); err != nil {
		t.Fatalf("the HTML-escaped readable name is readable: %v", err)
	}
	q := NewTemplatePrivacy("hq")
	q.Text(`Fish & "Chips" <b>`)
	if err := outbound(q, "rendered: Fish &amp; &#34;Chips&#34; &lt;b&gt;"); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("an HTML-escaped protected value is a leak: %v", err)
	}
}

// Cancellation inside one large JSON field (#12).
func TestCancellationInsideOneLargeJSONField(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"numbers":[`)
	for i := 0; i < 200000; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprint(&b, i)
	}
	b.WriteString(`]}`)
	p := NewTemplatePrivacy("jc")
	ctx := &countdownCtx{Context: context.Background()}
	ctx.left.Store(3)
	_, err := p.Entity(ctx, &models.Group{Name: "g", Meta: types.JSON(b.String())})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a large JSON field must observe cancellation: %v", err)
	}
	if _, err := p.Entity(&countdownCtx{Context: context.Background()}, &models.Group{Name: "g"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("an already-cancelled context must fail at entry: %v", err)
	}
}

// The cold, corpus-first lifecycle: a 2 MB corpus, then 20,000 unmatched 80-byte
// values, then one 2 MB payload check (#13). The plan's bound is 300 ms; the
// test is looser because shared machines are noisy.
func TestColdCorpusFirstLifecycle(t *testing.T) {
	p := NewTemplatePrivacy("cl")
	p.Keep(strings.Repeat("readable template text with no protected values in it. ", 40000)[:2<<20])
	for i := 0; i < 20000; i++ {
		p.Text(fmt.Sprintf("%080d", i) + "unmatched value")
	}
	payload := []byte(`{"m":"` + strings.Repeat("lorem ipsum dolor sit amet ", 80000)[:2<<20] + `"}`)
	start := time.Now()
	if err := p.CheckOutbound(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("cold reconciliation plus check took %v", took)
	}
}

func BenchmarkPrivacyGuardColdLifecycle(b *testing.B) {
	payload := []byte(`{"m":"` + strings.Repeat("lorem ipsum dolor sit amet ", 80000)[:2<<20] + `"}`)
	corpus := strings.Repeat("readable template text with no protected values in it. ", 40000)[:2<<20]
	for i := 0; i < b.N; i++ {
		p := NewTemplatePrivacy("cl")
		p.Keep(corpus)
		for n := 0; n < 20000; n++ {
			p.Text(fmt.Sprintf("%080d", n) + "unmatched value")
		}
		if err := p.CheckOutbound(context.Background(), payload); err != nil {
			b.Fatal(err)
		}
	}
}
