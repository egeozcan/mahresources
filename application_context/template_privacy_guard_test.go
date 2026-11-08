package application_context

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func TestACAutomatonFindsOverlappingPatterns(t *testing.T) {
	a := newACAutomaton()
	for i, p := range []string{"he", "she", "his", "hers"} {
		a.add(p, i)
	}
	a.build()
	var got []int
	_, err := a.scan(context.Background(), "ushers", false, func(id int) bool { got = append(got, id); return false })
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, id := range got {
		seen[id] = true
	}
	if !seen[0] || !seen[1] || !seen[3] || seen[2] {
		t.Fatalf("ushers should contain he, she and hers but not his: %v", got)
	}
}

func TestACAutomatonDuplicatePatternsKeepEveryID(t *testing.T) {
	a := newACAutomaton()
	a.add("same", 1)
	a.add("same", 2)
	a.build()
	seen := map[int]bool{}
	_, _ = a.scan(context.Background(), "a same b", true, func(id int) bool { seen[id] = true; return false })
	if !seen[1] || !seen[2] {
		t.Fatalf("both ids must be reported: %v", seen)
	}
}

func outbound(p *TemplatePrivacy, leaves ...string) error {
	body := `{"model":"m","messages":[`
	for i, l := range leaves {
		if i > 0 {
			body += ","
		}
		body += `{"role":"user","content":"` + jsonStringBody(l, false) + `"}`
	}
	return p.CheckOutbound(context.Background(), []byte(body+`]}`))
}

func TestPrivacyGuardThresholdIsFourCharacters(t *testing.T) {
	p := NewTemplatePrivacy("gd")
	p.Text("abc")
	p.Text("abcd")
	if err := outbound(p, "contains abc only"); err != nil {
		t.Fatalf("a value of 3 characters is not guarded: %v", err)
	}
	if err := outbound(p, "contains abcd here"); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("a value of 4 characters is guarded: %v", err)
	}
}

func TestPrivacyGuardIsCaseInsensitive(t *testing.T) {
	p := NewTemplatePrivacy("gd")
	p.Text("Moonbase Alpha")
	if err := outbound(p, "see MOONBASE alpha today"); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("case must not matter: %v", err)
	}
}

func TestPrivacyGuardFindsJSONEscapedAndHTMLEscapedForms(t *testing.T) {
	p := NewTemplatePrivacy("gd")
	value := "He said \"hi\"\nthen left <b> & café"
	p.Text(value)
	// Inside a JSON Tool result the value is JSON-escaped, and that text then sits in a message string.
	escaped := `{"name":"` + jsonStringBody(value, false) + `"}`
	if err := outbound(p, escaped); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("JSON-escaped form: %v", err)
	}
	htmlEscaped := `{"name":"` + jsonStringBody(value, true) + `"}`
	if err := outbound(p, htmlEscaped); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("HTML-escaped form: %v", err)
	}
	if err := outbound(p, value); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("raw form: %v", err)
	}
}

func TestPrivacyGuardErrorNamesThePathNeverTheValue(t *testing.T) {
	p := NewTemplatePrivacy("gd")
	secret := "SuperSecretQuarterlyFigures"
	p.Text(secret)
	err := outbound(p, "fine", "also fine", "leak "+secret)
	if err == nil {
		t.Fatal("expected a leak")
	}
	var leak *TemplatePrivacyLeakError
	if !errors.As(err, &leak) || leak.Path != "messages[2].content" {
		t.Fatalf("path: %+v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(secret)) {
		t.Fatalf("the error must never carry the value: %v", err)
	}
}

func TestPrivacyGuardCorpusExemptsAValuePerValue(t *testing.T) {
	p := NewTemplatePrivacy("gd")
	p.Text("Projects")
	p.Text("Hidden Plans")
	p.Keep("The Projects category holds your work")
	if err := outbound(p, "Projects"); err != nil {
		t.Fatalf("a value that readable text carries is exempt: %v", err)
	}
	if err := outbound(p, "Hidden Plans"); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("a value not in the corpus is still guarded: %v", err)
	}
	// A corpus text registered later exempts the value too, and a value added after the text.
	p.Keep("our hidden plans are public")
	if err := outbound(p, "Hidden Plans"); err != nil {
		t.Fatalf("late corpus text: %v", err)
	}
	p.Text("Later Value")
	p.Keep("later value in prose")
	if err := outbound(p, "Later Value"); err != nil {
		t.Fatalf("value added before its corpus text: %v", err)
	}
	p.Keep("x")
	p.Text("Another Name")
	if err := outbound(p, "Another Name"); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("a value added after the corpus and not in it is guarded: %v", err)
	}
	// Exemption is permanent: the corpus only grows.
	if err := outbound(p, "Projects"); err != nil {
		t.Fatalf("exemption must be permanent: %v", err)
	}
}

func TestPrivacyGuardLongValueMatchesItsPrefix(t *testing.T) {
	p := NewTemplatePrivacy("gd")
	long := strings.Repeat("distinctive prefix text ", 40)
	p.Text(long)
	if err := outbound(p, long[:100]); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("a truncated copy of a long value must be found: %v", err)
	}
}

func TestPrivacyGuardNilAndEmptyAreQuiet(t *testing.T) {
	var p *TemplatePrivacy
	if err := p.CheckOutbound(context.Background(), []byte(`{"a":"b"}`)); err != nil {
		t.Fatal(err)
	}
	p = NewTemplatePrivacy("gd")
	if err := p.CheckOutbound(context.Background(), []byte(`{"a":"b"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestPrivacyGuardNonJSONBodyIsScannedWhole(t *testing.T) {
	p := NewTemplatePrivacy("gd")
	p.Text("raw body secret")
	if err := p.CheckOutbound(context.Background(), []byte("not json: raw body secret")); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("%v", err)
	}
}

func TestPrivacyGuardHonoursCancellation(t *testing.T) {
	p := NewTemplatePrivacy("gd")
	p.Text("some guarded value")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	big := strings.Repeat("x", 300000)
	err := p.CheckOutbound(ctx, []byte(`{"a":"`+big+`"}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled context stops the scan: %v", err)
	}
}

func randomWord(r *rand.Rand, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('a' + r.Intn(26))
	}
	return string(b)
}

// The plan's budgets: 20,000 random values against a 2 MB payload under 50 ms.
// The test bound is looser than the plan's, because a shared CI box is noisy;
// BenchmarkPrivacyGuardRandom reports the real figure.
func TestPrivacyGuardScalesToTwentyThousandValues(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	p := NewTemplatePrivacy("gd")
	for i := 0; i < 20000; i++ {
		p.Text(fmt.Sprintf("%s %s %d", randomWord(r, 8), randomWord(r, 6), i))
	}
	p.Keep("warm the corpus")
	payload := []byte(`{"m":"` + strings.Repeat("lorem ipsum dolor sit amet ", 80000)[:2<<20] + `"}`)
	if err := p.CheckOutbound(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := p.CheckOutbound(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Fatalf("scan took %v", took)
	}
}

func TestPrivacyGuardAdversarialOverlappingValues(t *testing.T) {
	p := NewTemplatePrivacy("gd")
	for i := 4; i < 2000; i++ {
		p.Text(strings.Repeat("a", i))
	}
	// Every value is a run of one character, and the corpus is a run of it: each
	// value is exempt, found once each, in linear time.
	p.Keep(strings.Repeat("a", 2<<20))
	start := time.Now()
	if err := p.CheckOutbound(context.Background(), []byte(`{"m":"`+strings.Repeat("a", 2<<20)+`"}`)); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("adversarial settle+scan took %v", took)
	}
}

func BenchmarkPrivacyGuardRandom(b *testing.B) {
	r := rand.New(rand.NewSource(1))
	p := NewTemplatePrivacy("gd")
	for i := 0; i < 20000; i++ {
		p.Text(fmt.Sprintf("%s %s %d", randomWord(r, 8), randomWord(r, 6), i))
	}
	payload := []byte(`{"m":"` + strings.Repeat("lorem ipsum dolor sit amet ", 80000)[:2<<20] + `"}`)
	_ = p.CheckOutbound(context.Background(), payload)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := p.CheckOutbound(context.Background(), payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPrivacyGuardAdversarial(b *testing.B) {
	for i := 0; i < b.N; i++ {
		p := NewTemplatePrivacy("gd")
		for n := 4; n < 2000; n++ {
			p.Text(strings.Repeat("a", n))
		}
		p.Keep(strings.Repeat("a", 2<<20))
		if err := p.CheckOutbound(context.Background(), []byte(`{"m":"`+strings.Repeat("a", 2<<20)+`"}`)); err != nil {
			b.Fatal(err)
		}
	}
}
