package application_context

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The guard half of Privacy mode (P7, P8). The masker issues a Placeholder for
// every Entity value; the guard keeps those values and scans each outgoing
// request for them. A value that also appears in the run's readable corpus is
// exempt (Q12), and a value shorter than four characters is not guarded.

// privacyGuardMinRunes is the shortest value the guard looks for.
const privacyGuardMinRunes = 4

// TemplatePrivacyLeakError is ErrTemplatePrivacyLeak with the JSON path of the
// offending string, never its value.
type TemplatePrivacyLeakError struct {
	Path string
	// Limit is set when the masker, not a scan, refused the request.
	Limit error
}

func (e *TemplatePrivacyLeakError) Error() string {
	switch {
	case e.Limit != nil:
		return ErrTemplatePrivacyLeak.Error() + " (" + e.Limit.Error() + ")"
	case e.Path != "":
		return ErrTemplatePrivacyLeak.Error() + " (at " + e.Path + ")"
	}
	return ErrTemplatePrivacyLeak.Error()
}

func (e *TemplatePrivacyLeakError) Unwrap() []error {
	if e.Limit != nil {
		return []error{ErrTemplatePrivacyLeak, e.Limit}
	}
	return []error{ErrTemplatePrivacyLeak}
}

// TemplatePrivacyGuardInterrupted is a guard check that the context ended
// before it finished. Nothing was sent, so a run takes back what it marked as
// sent; the context error stays reachable with errors.Is.
type TemplatePrivacyGuardInterrupted struct{ Err error }

func (e *TemplatePrivacyGuardInterrupted) Error() string { return e.Err.Error() }
func (e *TemplatePrivacyGuardInterrupted) Unwrap() error { return e.Err }

type privacyGuard struct {
	values    []privacyGuardValue
	corpus    []string
	corpusSet map[string]struct{}

	// values[:valuesScanned] have been scanned against corpus[:corpusScanned].
	valuesScanned int
	corpusScanned int

	// version counts changes of the non-exempt set; auto is the payload
	// automaton built for autoVersion.
	version     int
	auto        *acAutomaton
	autoVersion int
}

func newPrivacyGuard() privacyGuard {
	return privacyGuard{corpusSet: map[string]struct{}{}, autoVersion: -1}
}

func (g *privacyGuard) addValue(value string) {
	if utf8.RuneCountInString(value) < privacyGuardMinRunes {
		return
	}
	g.values = append(g.values, privacyGuardValue{spellings: privacySpellings(value)})
	g.version++
}

// addCorpus registers a readable text in the same spellings a protected value is
// looked for in, so text that is escaped when it is encoded into a Tool result
// or rendered into HTML still matches.
func (g *privacyGuard) addCorpus(s string) {
	for _, form := range privacySpellings(s) {
		if _, ok := g.corpusSet[form]; ok {
			continue
		}
		g.corpusSet[form] = struct{}{}
		g.corpus = append(g.corpus, form)
	}
}

// privacySpellings are the lower-cased forms a string takes in a request: raw,
// HTML-escaped (what a shortcode render makes of it), and the JSON-string and
// JSON-with-HTML-escaping forms of each.
func privacySpellings(value string) []string {
	bases := []string{value, html.EscapeString(value)}
	var out []string
	seen := map[string]bool{}
	for _, base := range bases {
		for _, form := range []string{base, jsonStringBody(base, false), jsonStringBody(base, true)} {
			form = strings.ToLower(form)
			if form == "" || seen[form] {
				continue
			}
			seen[form] = true
			out = append(out, form)
		}
	}
	return out
}

func jsonStringBody(s string, escapeHTML bool) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(escapeHTML)
	if enc.Encode(s) != nil {
		return s
	}
	out := strings.TrimRight(buf.String(), "\n")
	return out[1 : len(out)-1]
}

// settle decides the exemption of every value against the corpus registered so
// far, in two batches: each new corpus text is scanned once against the values
// already known, and each new value once against the whole corpus. A value is
// exempt only when a spelling of its COMPLETE text occurs in a corpus text.
// An exemption invalidates the cached payload matcher the moment it is made, so
// an interrupted reconciliation can never leave the matcher stale.
func (g *privacyGuard) settle(ctx context.Context) error {
	var oldVals, newVals []int
	for i := range g.values {
		if g.values[i].exempt {
			continue
		}
		if i < g.valuesScanned {
			oldVals = append(oldVals, i)
		} else {
			newVals = append(newVals, i)
		}
	}
	scan := func(vals []int, texts []string) error {
		if len(vals) == 0 || len(texts) == 0 {
			return nil
		}
		a := newACAutomaton(ctx)
		for _, vi := range vals {
			for _, sp := range g.values[vi].spellings {
				a.add(sp, vi)
			}
		}
		a.build()
		if a.err != nil {
			return a.err
		}
		for _, text := range texts {
			if _, err := a.scan(text, true, func(id int) bool {
				if !g.values[id].exempt {
					g.values[id].exempt = true
					g.version++
				}
				return false
			}); err != nil {
				return err
			}
			if !a.tick(1) {
				return a.err
			}
		}
		return nil
	}
	if err := scan(oldVals, g.corpus[g.corpusScanned:]); err != nil {
		return err
	}
	if err := scan(newVals, g.corpus); err != nil {
		return err
	}
	g.valuesScanned, g.corpusScanned = len(g.values), len(g.corpus)
	return nil
}

// payloadAutomaton is the automaton over every non-exempt value, rebuilt only
// when that set changed. A reused automaton starts each check with a clean
// context and error, so one check's cancellation never reaches the next.
func (g *privacyGuard) payloadAutomaton(ctx context.Context) (*acAutomaton, error) {
	if g.auto != nil && g.autoVersion == g.version {
		g.auto.ctx, g.auto.err, g.auto.work = ctx, nil, 0
		return g.auto, nil
	}
	a := newACAutomaton(ctx)
	for i := range g.values {
		if g.values[i].exempt {
			continue
		}
		for _, sp := range g.values[i].spellings {
			a.add(sp, i)
		}
	}
	a.build()
	if a.err != nil {
		return nil, a.err
	}
	g.auto, g.autoVersion = a, g.version
	return a, nil
}

// CheckOutbound decodes payload and scans every string leaf for a
// guarded value in its raw, JSON-escaped and HTML-escaped spelling, ignoring
// case. A value that readable text carries is exempt. The error names the JSON
// path of the first hit and never the value. While the masker's sticky error is
// set it refuses outright.
func (p *TemplatePrivacy) CheckOutbound(ctx context.Context, payload []byte) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return &TemplatePrivacyLeakError{Limit: p.err}
	}
	// Cancellation is checked on entry and, inside reconciliation, automaton
	// construction and the payload walk, at bounded intervals counted across
	// entries (acAutomaton.tick), not per string.
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.guard.settle(ctx); err != nil {
		return err
	}
	auto, err := p.guard.payloadAutomaton(ctx)
	if err != nil {
		return err
	}
	if auto.empty() {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		// Not JSON: scan the body as one string, fail closed on a hit.
		root = string(payload)
	} else if _, err := dec.Token(); err != io.EOF {
		root = string(payload)
	}
	var walkErr error
	var walk func(v any, path string) bool
	walk = func(v any, path string) bool {
		switch x := v.(type) {
		case string:
			hit, err := auto.scan(strings.ToLower(x), false, func(int) bool { return true })
			if err != nil {
				walkErr = err
				return true
			}
			if hit {
				walkErr = &TemplatePrivacyLeakError{Path: leafPath(path)}
				return true
			}
		case map[string]any:
			for k, e := range x {
				child := k
				if path != "" {
					child = path + "." + k
				}
				if walk(e, child) {
					return true
				}
			}
		case []any:
			for i, e := range x {
				if walk(e, path+"["+strconv.Itoa(i)+"]") {
					return true
				}
			}
		}
		return false
	}
	walk(root, "")
	return walkErr
}

func leafPath(path string) string {
	if path == "" {
		return "$"
	}
	return path
}

// acAutomaton is an Aho-Corasick automaton over bytes, written here because the
// guard needs only this much of it: many patterns, one pass, "first hit" and
// "which patterns occur" queries.
type acAutomaton struct {
	ctx   context.Context
	work  int
	err   error
	root  [256]int32
	nodes []acNode
	pats  []acPat
	found []bool
	n     int
}

type acNode struct {
	child, sib, fail, dict int32
	patHead                int32 // 1-based index into pats, 0 = none
	live                   int32 // patterns here not yet found
	c                      byte
}

type acPat struct {
	id   int
	next int32
}

func newACAutomaton(ctx context.Context) *acAutomaton {
	return &acAutomaton{ctx: ctx, nodes: make([]acNode, 1)}
}

// acTickEvery is how many units of work (bytes scanned, nodes built) pass
// between context checks. The count runs across every entry the automaton
// handles, so many short strings are interrupted as promptly as one long one.
const acTickEvery = 1 << 12

// tick counts n units of work and reports whether the context is still live.
func (a *acAutomaton) tick(n int) bool {
	if a.err != nil {
		return false
	}
	a.work += n
	if a.work >= acTickEvery {
		a.work = 0
		if err := a.ctx.Err(); err != nil {
			a.err = err
			return false
		}
	}
	return true
}

func (a *acAutomaton) empty() bool { return a.n == 0 }

func (a *acAutomaton) childOf(n int32, c byte) int32 {
	if n == 0 {
		return a.root[c]
	}
	for k := a.nodes[n].child; k != 0; k = a.nodes[k].sib {
		if a.nodes[k].c == c {
			return k
		}
	}
	return 0
}

func (a *acAutomaton) add(pattern string, id int) {
	if pattern == "" || !a.tick(len(pattern)) {
		return
	}
	n := int32(0)
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		next := a.childOf(n, c)
		if next == 0 {
			a.nodes = append(a.nodes, acNode{c: c})
			next = int32(len(a.nodes) - 1)
			if n == 0 {
				a.root[c] = next
			} else {
				a.nodes[next].sib = a.nodes[n].child
				a.nodes[n].child = next
			}
		}
		n = next
	}
	a.pats = append(a.pats, acPat{id: id, next: a.nodes[n].patHead})
	a.nodes[n].patHead = int32(len(a.pats))
	a.nodes[n].live++
	a.n++
}

// build computes the failure and dictionary links breadth first.
func (a *acAutomaton) build() {
	a.found = make([]bool, len(a.pats)+1)
	queue := make([]int32, 0, len(a.nodes))
	for c := 0; c < 256; c++ {
		if k := a.root[c]; k != 0 {
			queue = append(queue, k)
		}
	}
	for head := 0; head < len(queue); head++ {
		if !a.tick(1) {
			return
		}
		u := queue[head]
		for k := a.nodes[u].child; k != 0; k = a.nodes[k].sib {
			queue = append(queue, k)
			c := a.nodes[k].c
			f := a.nodes[u].fail
			for {
				if w := a.childOf(f, c); w != 0 && w != k {
					a.nodes[k].fail = w
					break
				}
				if f == 0 {
					a.nodes[k].fail = 0
					break
				}
				f = a.nodes[f].fail
			}
			fl := a.nodes[k].fail
			if a.nodes[fl].patHead != 0 {
				a.nodes[k].dict = fl
			} else {
				a.nodes[k].dict = a.nodes[fl].dict
			}
		}
	}
}

// liveFrom is the first node at or below n on the dictionary chain that still
// has an unfound pattern.
func (a *acAutomaton) liveFrom(n int32) int32 {
	for n != 0 && a.nodes[n].live == 0 {
		n = a.nodes[n].dict
	}
	return n
}

// scan runs text through the automaton and calls visit for each pattern id that
// ends at a position. visit returning true stops the scan. The result says
// whether it stopped that way. With retire set, a pattern is visited once: it
// is marked found, and a node with nothing left to find drops off the
// dictionary chain, so a repetitive text over overlapping patterns stays linear.
func (a *acAutomaton) scan(text string, retire bool, visit func(id int) bool) (bool, error) {
	if a.n == 0 {
		return false, nil
	}
	if !a.tick(1) {
		return false, a.err
	}
	state := int32(0)
	for i := 0; i < len(text); i++ {
		if !a.tick(1) {
			return false, a.err
		}
		c := text[i]
		for {
			if w := a.childOf(state, c); w != 0 {
				state = w
				break
			}
			if state == 0 {
				break
			}
			state = a.nodes[state].fail
		}
		var n int32
		if a.nodes[state].patHead != 0 && a.nodes[state].live > 0 {
			n = state
		} else {
			n = a.liveFrom(a.nodes[state].dict)
			a.nodes[state].dict = n
		}
		for n != 0 {
			for k := a.nodes[n].patHead; k != 0; k = a.pats[k-1].next {
				if a.found[k] {
					continue
				}
				if retire {
					a.found[k] = true
					a.nodes[n].live--
				}
				if visit(a.pats[k-1].id) {
					return true, nil
				}
			}
			nxt := a.liveFrom(a.nodes[n].dict)
			a.nodes[n].dict = nxt
			n = nxt
		}
	}
	return false, nil
}
