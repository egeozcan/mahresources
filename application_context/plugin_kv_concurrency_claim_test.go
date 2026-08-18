//go:build json1 && fts5

package application_context

import (
	"os"
	"strings"
	"testing"
)

// The compare-and-set is right. The reason given for it is not, in all four
// places it is given.
//
// Every one of them says some form of "two of a plugin's surfaces run at the
// same time, so a read-modify-write loses a write". They cannot. A plugin has
// one VM behind one exclusive vmMutex, and every entry into its Lua holds that
// lock for the whole call: injections and the other request-serving surfaces
// via lockVMForRequest, hooks via lockVMForHook, async action jobs and drained
// HTTP callbacks via LockVM. plugin_system.TestPluginSurfaces_CannotRunAtTheSameTime
// measures it. plugin-lua-api.md says it too, forty lines above the mah.kv
// section that denies it: "All calls (hooks, actions, page handlers, HTTP
// callbacks) acquire this mutex, ensuring single-threaded execution within a
// single plugin."
//
// This matters beyond tidiness, because the false version is not merely
// imprecise -- it points an author away from the arrangements that actually
// lose their write. Two are real and are named nowhere:
//
//   - The read and the write land in two separate calls into the plugin.
//     mah.start_job and a drained HTTP callback both run after the call that
//     registered them released the lock, so the lock never spans the pair. This
//     is the ordinary shape of both features, not an exotic one.
//   - The deployment runs more than one process. Each has its own VM and its
//     own mutex, and nothing serializes across them at all.
//
// An author who reads the shipped prose learns a concurrency model the platform
// does not have, and is wrong in both directions: they will expect their
// handlers to run in parallel, and they will not learn the one arrangement in
// which their read-modify-write is genuinely unsafe.

// kvProseSpan is one place the model is described, with the claim it makes
// today that the lock contradicts.
//
// The check is "this exact claim is gone" rather than a pattern over words like
// "in parallel" or "at the same time", because the correction is built from the
// same vocabulary negated -- a plugin's surfaces never run at the same time --
// so a pattern that catches the falsehood catches the fix with it. A
// negation-aware pattern does not survive either: the plugin_kv_context.go
// sentence carries a negation and the false claim in one sentence ("cannot run
// concurrently with itself, which two of its own surfaces served in parallel
// already disprove"), so a rule that excuses negated sentences excuses the
// worst of the four. Pinning the sentences and requiring the true exposures to
// be named bounds the rewrite from both sides instead.
type kvProseSpan struct {
	where      string
	text       string
	falseClaim string
}

func kvProseSpans(t *testing.T) []kvProseSpan {
	t.Helper()

	const (
		luaAPIPage = "../docs-site/docs/features/plugin-lua-api.md"
		systemPage = "../docs-site/docs/features/plugin-system.md"
		kvAPIFile  = "../plugin_system/kv_api.go"
		kvCtxFile  = "plugin_kv_context.go"
	)
	return []kvProseSpan{{
		where:      luaAPIPage + " (## mah.kv)",
		text:       docsSection(t, luaAPIPage, "## mah.kv"),
		falseClaim: "Two of a plugin's surfaces can be served at the same time",
	}, {
		where:      systemPage + " (## Key-Value Storage)",
		text:       docsSection(t, systemPage, "## Key-Value Storage"),
		falseClaim: "two of a plugin's surfaces may update at once",
	}, {
		where:      kvCtxFile + " (PluginKVCompareAndSet)",
		text:       goDocComment(t, kvCtxFile, "func (ctx *MahresourcesContext) PluginKVCompareAndSet("),
		falseClaim: "which two of its own surfaces served in parallel already disprove",
	}, {
		where:      kvAPIFile + " (compare_and_set)",
		text:       goDocComment(t, kvAPIFile, `kvMod.RawSetString("compare_and_set"`),
		falseClaim: "two of a plugin's own surfaces updating one key at once",
	}}
}

func TestPluginKVProse_ClaimsNoSimultaneousSurfaces(t *testing.T) {
	for _, span := range kvProseSpans(t) {
		if strings.Contains(span.text, span.falseClaim) {
			t.Errorf("%s still says %q. A plugin's surfaces queue behind one exclusive VM lock, "+
				"so they never run at the same time and this cannot be why a read-modify-write "+
				"loses a write", span.where, span.falseClaim)
		}
	}
}

// The two spans that carry the argument rather than a one-line summary have to
// name the arrangements that really do lose a write. The other two are left to
// the claim removal alone: plugin-system.md links straight to the mah.kv
// reference, and the comment at the Lua binding is a sentence about what the
// function answers, not the place to reproduce the model.
func TestPluginKVProse_NamesTheRealExposures(t *testing.T) {
	exposures := []struct {
		markers []string
		what    string
	}{{
		markers: []string{"start_job", "callback"},
		what: "a read and a write that land in two separate calls into the plugin, which is what " +
			"mah.start_job and a drained HTTP callback are: both run after the call that " +
			"registered them released the VM lock",
	}, {
		markers: []string{"process"},
		what: "a deployment running more than one process, where each process has its own VM and " +
			"its own lock and nothing serializes across them",
	}}

	for _, span := range kvProseSpans(t) {
		if !strings.Contains(span.where, "## mah.kv") && !strings.Contains(span.where, "PluginKVCompareAndSet") {
			continue
		}
		for _, exposure := range exposures {
			found := false
			for _, marker := range exposure.markers {
				if strings.Contains(span.text, marker) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s does not mention %v, so it never names %s", span.where, exposure.markers, exposure.what)
			}
		}
	}
}

// docsSection returns a page from the given heading up to the next one of the
// same level. It is luaAPISection from plugin_kv_cas_lua_test.go with the path
// as an argument; that one should fold into this.
func docsSection(t *testing.T, path, heading string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var (
		out   []string
		found bool
	)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, heading) {
			found = true
			continue
		}
		if found && strings.HasPrefix(line, "## ") {
			break
		}
		if found {
			out = append(out, line)
		}
	}
	if !found {
		t.Fatalf("%s has no %q heading", path, heading)
	}
	return strings.Join(out, "\n")
}

// goDocComment returns the comment block immediately above the line containing
// marker, as one line of prose.
//
// Rejoining it matters. These comments wrap at seventy-odd columns, so a
// sentence in the file is several lines each starting "// ", and a check for
// the sentence against the raw file would never match it -- passing while
// testing nothing, which is the failure mode a drift test can least afford.
func goDocComment(t *testing.T, path, marker string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	lines := strings.Split(string(data), "\n")
	at := -1
	for i, line := range lines {
		if strings.Contains(line, marker) {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatalf("%s has no line containing %q", path, marker)
	}

	var block []string
	for i := at - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "//") {
			break
		}
		text := strings.TrimSpace(strings.TrimPrefix(trimmed, "//"))
		if text == "" {
			// A paragraph break. Dropped rather than kept, so joining does not
			// leave the double spaces a sentence match would then miss on.
			continue
		}
		block = append([]string{text}, block...)
	}
	if len(block) == 0 {
		t.Fatalf("%s has no comment above %q", path, marker)
	}
	return strings.Join(block, " ")
}
