package plugin_system

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A bundled plugin's `network` list is a security boundary the operator never
// wrote and cannot easily audit: it ships in a file we author, and it is enforced
// at request time, on a worker goroutine, in a plugin nobody runs in CI. So the
// failure mode is a plugin that works everywhere except at the one moment it
// reaches the network — which is exactly how fal-ai shipped with a declaration
// covering fal's queue API and not the CDN its results are served from.
//
// These tests are the static half of that check. They cannot follow a URL that
// is built at runtime (see TestFalAIAllowsResultMediaHosts for what is done
// instead), but everything visible at rest is checked here.

// bundledPluginSource reads one bundled plugin's Lua.
func bundledPluginSource(t *testing.T, dir, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, name, "plugin.lua"))
	if err != nil {
		t.Fatalf("reading %s/plugin.lua: %v", name, err)
	}
	return string(body)
}

// urlLiteral finds an absolute http(s) URL in Lua source. It stops at the first
// character that cannot appear unescaped in a URL, which is what ends the match
// at the closing quote of a Lua string.
var urlLiteral = regexp.MustCompile(`https?://[A-Za-z0-9._~:/?#\[\]@!$&'()*+,;=%-]+`)

// sourceWithoutComments drops whole-line Lua comments.
//
// Deliberately only whole-line `--`: it is enough to cover the case that matters
// (example-plugin documents mah.http by commenting out a call to a fictional
// api.example.com), and a real Lua comment stripper has to track long-bracket
// levels and string literals to avoid eating a `--` that is inside a string —
// which is the failure mode where this quietly stops checking anything. NOT
// handled, and knowingly so: trailing comments after code, and `--[[ ]]` block
// comments. A URL in either is checked as though it were live, which errs
// towards a declaration that is too broad rather than a check that is too weak.
func sourceWithoutComments(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// TestBundledPluginLiteralURLsAreDeclared checks every absolute URL written into
// a bundled plugin's source against that plugin's own egress policy, through the
// same matcher the request path uses.
//
// A plugin that declares no `network` list is Unrestricted, so its literals pass
// vacuously — correctly, since nothing confines it. The test bites the moment a
// plugin declares a list, which is the moment the list can be wrong.
func TestBundledPluginLiteralURLsAreDeclared(t *testing.T) {
	dir := bundledPluginDir(t)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()

	for _, dp := range pm.DiscoveredPlugins() {
		policy := dp.Manifest.NetworkPolicy()
		src := sourceWithoutComments(bundledPluginSource(t, dir, dp.Name))

		seen := map[string]bool{}
		for _, raw := range urlLiteral.FindAllString(src, -1) {
			host, err := hostFromURL(raw)
			if err != nil || seen[host] {
				continue
			}
			seen[host] = true
			// XML namespaces are identifiers that happen to be spelled as URLs.
			// Nothing dereferences the xmlns of an inline <svg>.
			if host == "www.w3.org" {
				continue
			}
			if !policy.Allows(host) {
				t.Errorf("plugin %q writes a URL to %s but its manifest `network` does not cover that host: %s",
					dp.Name, host, raw)
			}
		}
	}
}

// fetchingDBFunctions are the mah.db functions that make an outbound request.
//
// They are the reason a `network` list and an `http` capability are not
// separable concerns: a plugin can reach the network without ever naming
// mah.http, by handing a URL to the application's own downloader.
var fetchingDBFunctions = []string{
	"mah.db.create_resource_from_url",
	"mah.db.add_resource_version_from_url",
}

// TestBundledPluginCapabilitiesCoverNetworkUse asserts that a bundled plugin
// which fetches declares what fetching needs.
//
// The two mah.db fetchers are installed behind `canWrite && grants.Has(CapHTTP)`
// (db_api.go), so a plugin calling them with `db:write` and no `http` gets a nil
// where it expected a function — at the end of a job it has already paid fal for.
// That co-requisite is invisible from the call site and is what this pins.
func TestBundledPluginCapabilitiesCoverNetworkUse(t *testing.T) {
	dir := bundledPluginDir(t)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()

	for _, dp := range pm.DiscoveredPlugins() {
		if !dp.Manifest.Declared {
			continue // covered by TestBundledPluginsDeclareManifests
		}
		src := sourceWithoutComments(bundledPluginSource(t, dir, dp.Name))
		granted := dp.Manifest.Capabilities()

		if strings.Contains(src, "mah.http.") && !granted.Has(CapHTTP) {
			t.Errorf("plugin %q calls mah.http but does not declare the %q capability", dp.Name, CapHTTP)
		}
		for _, fn := range fetchingDBFunctions {
			if !strings.Contains(src, fn) {
				continue
			}
			if !granted.Has(CapHTTP) {
				t.Errorf("plugin %q calls %s, which fetches a URL, but does not declare the %q capability; "+
					"the host installs it only for a plugin holding db:write AND http, so the call would be nil",
					dp.Name, fn, CapHTTP)
			}
			if !granted.Has(CapDBWrite) {
				t.Errorf("plugin %q calls %s but does not declare the %q capability", dp.Name, fn, CapDBWrite)
			}
		}
	}
}

// TestFalAIAllowsResultMediaHosts pins the half of fal-ai's egress surface that
// no static check can see.
//
// The URL a generated image is downloaded from is read out of fal's API response
// at runtime (get_result_url reads result.image.url or result.images[1].url) and
// then handed to create_resource_from_url / add_resource_version_from_url. It
// appears nowhere in the source, so TestBundledPluginLiteralURLsAreDeclared
// cannot see it — and that is precisely the host the manifest was missing.
//
// What is checked instead is that the policy admits fal's documented CDN. The
// hosts below are from fal's own docs, which give the format as
// `https://v3b.fal.media/files/b/{prefix}/{filename}` and name `v3.fal.media`
// and `fal.media` in the same table's fallback chain
// (https://fal.ai/docs/documentation/model-apis/fal-cdn), and from live output
// schemas, which return images[].url on both v3 and v3b today.
//
// vnext.fal.media is synthetic. It is here because the exact host is NOT stable
// — the prefix has already rolled v3 -> v3b — so a declaration that only covered
// today's spellings would be a re-run of this bug on fal's next roll. It asserts
// the wildcard, not a hostname.
func TestFalAIAllowsResultMediaHosts(t *testing.T) {
	pm, err := NewPluginManager(bundledPluginDir(t))
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()

	var policy NetworkPolicy
	var found bool
	for _, dp := range pm.DiscoveredPlugins() {
		if dp.Name == "fal-ai" {
			policy, found = dp.Manifest.NetworkPolicy(), true
		}
	}
	if !found {
		t.Fatal("fal-ai was not discovered")
	}
	if policy.Unrestricted {
		t.Fatal("fal-ai declares no `network` list; this test asserts what that list covers")
	}

	for _, host := range []string{
		"v3b.fal.media", // documented CDN URL format
		"v3.fal.media",  // documented fallback, and live in imagen4/recraft output schemas
		"fal.media",     // documented final fallback; the apex needs its own rule, as *.fal.media requires a label
		"vnext.fal.media",
		"queue.fal.run", // the API half, so a fix to one family cannot drop the other
	} {
		if err := CheckEgressURL(policy, "https://"+host+"/files/b/abc/out.png"); err != nil {
			t.Errorf("fal-ai may not download its own results from %s: %v", host, err)
		}
	}

	// storage.googleapis.com is where fal serves the curated example assets in
	// its documentation, and it has never appeared in a real result payload.
	// Declaring it would make every Google Cloud Storage bucket reachable, so
	// the omission is a decision rather than an oversight.
	if policy.Allows("storage.googleapis.com") {
		t.Error("fal-ai's `network` list admits storage.googleapis.com, which reaches every GCS bucket; " +
			"fal serves real results from its own CDN")
	}
}
