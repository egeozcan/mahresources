package plugin_system

import (
	"strings"
	"sync"
	"testing"
)

// recordingFetcher stands in for the host-side downloader. It records the URLs
// it was asked for, so a test can tell "refused" from "fetched and failed".
type recordingFetcher struct {
	mockQuerier
	stubWriter
	mu         sync.Mutex
	urls       []string
	lastEgress bool
}

func (r *recordingFetcher) BindInvocation(inv *Invocation) (EntityQuerier, EntityWriter) {
	r.mu.Lock()
	r.lastEgress = inv != nil && inv.Egress != nil
	r.mu.Unlock()
	return r, r
}

func (r *recordingFetcher) CreateResourceFromURL(url string, _ map[string]any) (map[string]any, error) {
	r.record(url)
	return map[string]any{"id": float64(1)}, nil
}

func (r *recordingFetcher) AddResourceVersionFromURL(_ uint, url string, _ string) (map[string]any, error) {
	r.record(url)
	return map[string]any{"id": float64(1)}, nil
}

func (r *recordingFetcher) record(url string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.urls = append(r.urls, url)
}

func (r *recordingFetcher) fetched() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.urls...)
}

func (r *recordingFetcher) sawEgressPolicy() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastEgress
}

// enableWithFetcher loads a plugin with a recording downstream fetcher.
func enableWithFetcher(t *testing.T, code string) (*PluginManager, *recordingFetcher) {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "fetcher", code)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)

	fetcher := &recordingFetcher{}
	pm.SetEntityQuerier(fetcher)
	pm.SetEntityWriter(fetcher)
	pm.SetPrincipalBinder(fetcher)

	if err := pm.EnablePlugin("fetcher"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	return pm, fetcher
}

// TestDbWriteAloneDoesNotGrantAServerSideFetch is the fourth door.
//
// These two functions hand a URL to the application's own downloader rather
// than going through mah.http, so db:write on its own used to be a full
// server-side fetch primitive that the declared `network` list could not
// describe — the hole this package exists to close, reached through a
// different door.
func TestDbWriteAloneDoesNotGrantAServerSideFetch(t *testing.T) {
	pm, _ := enableWithFetcher(t, `
plugin = { name = "fetcher", version = "1.0", api_version = 1,
           capabilities = { "db:write" } }
function init() end
`)

	for _, fn := range []string{"mah.db.create_resource_from_url", "mah.db.add_resource_version_from_url"} {
		if mahHas(t, pm, "fetcher", fn) {
			t.Errorf("%s was installed for a plugin with db:write but no http", fn)
		}
	}
	// The rest of db:write is unaffected.
	if !mahHas(t, pm, "fetcher", "mah.db.create_tag") {
		t.Error("an ordinary writer was withheld; the co-requisite is too broad")
	}
	if mahHas(t, pm, "fetcher", "mah.http") {
		t.Error("mah.http was installed without the http capability")
	}
}

func TestDbWritePlusHttpGrantsTheFetchers(t *testing.T) {
	pm, _ := enableWithFetcher(t, `
plugin = { name = "fetcher", version = "1.0", api_version = 1,
           capabilities = { "db:write", "http" } }
function init() end
`)

	for _, fn := range []string{"mah.db.create_resource_from_url", "mah.db.add_resource_version_from_url"} {
		if !mahHas(t, pm, "fetcher", fn) {
			t.Errorf("%s was withheld from a plugin declaring both db:write and http", fn)
		}
	}
}

// TestTheFetchersRespectTheDeclaredAllowlist: the capability is only half of
// it. Having `http` does not mean reaching anywhere.
func TestTheFetchersRespectTheDeclaredAllowlist(t *testing.T) {
	pm, fetcher := enableWithFetcher(t, `
plugin = { name = "fetcher", version = "1.0", api_version = 1,
           capabilities = { "db:write", "http" },
           network = { "allowed.example" } }
result = ""
function init()
    local ok, err = mah.db.create_resource_from_url("http://blocked.example/thing")
    result = tostring(err)
end
`)
	_ = pm

	if urls := fetcher.fetched(); len(urls) != 0 {
		t.Fatalf("the downloader was asked for %v despite the host not being declared", urls)
	}
}

// TestTheFetchersCheckEveryURLNotJustTheFirst. AddRemoteResource splits its
// input on newlines and fetches every line, so a check on "the URL" gates the
// first and waves through the rest.
func TestTheFetchersCheckEveryURLNotJustTheFirst(t *testing.T) {
	_, fetcher := enableWithFetcher(t, `
plugin = { name = "fetcher", version = "1.0", api_version = 1,
           capabilities = { "db:write", "http" },
           network = { "allowed.example" } }
function init()
    mah.db.create_resource_from_url("http://allowed.example/one\nhttp://blocked.example/two")
end
`)

	if urls := fetcher.fetched(); len(urls) != 0 {
		t.Fatalf("a batch containing an undeclared host was fetched: %v", urls)
	}
}

func TestTheFetchersAllowADeclaredHost(t *testing.T) {
	_, fetcher := enableWithFetcher(t, `
plugin = { name = "fetcher", version = "1.0", api_version = 1,
           capabilities = { "db:write", "http" },
           network = { "allowed.example" } }
function init()
    mah.db.create_resource_from_url("http://allowed.example/thing")
end
`)

	urls := fetcher.fetched()
	if len(urls) != 1 || !strings.Contains(urls[0], "allowed.example") {
		t.Fatalf("the declared host was not fetched: %v", urls)
	}
}

// TestTheFetchersCarryThePolicyToTheDownloader. Layer (a) is checked in the
// plugin process, but layers (b) and (c) happen inside the host's own client —
// which only gets them if the policy rides along on the invocation.
func TestTheFetchersCarryThePolicyToTheDownloader(t *testing.T) {
	_, fetcher := enableWithFetcher(t, `
plugin = { name = "fetcher", version = "1.0", api_version = 1,
           capabilities = { "db:write", "http" },
           network = { "allowed.example" } }
function init()
    mah.db.create_resource_from_url("http://allowed.example/thing")
end
`)

	if !fetcher.sawEgressPolicy() {
		t.Fatal("the downloader was bound without a network policy, so its dial-time deny and " +
			"redirect re-check never run")
	}
}

// TestTheVersionFetcherRespectsTheDeclaredAllowlist. The two URL writers are
// separate code paths — the batch one splits on newlines and goes through
// AddRemoteResource, this one fetches a single URL through its own client — so
// covering only the first leaves half the door open.
func TestTheVersionFetcherRespectsTheDeclaredAllowlist(t *testing.T) {
	_, fetcher := enableWithFetcher(t, `
plugin = { name = "fetcher", version = "1.0", api_version = 1,
           capabilities = { "db:write", "http" },
           network = { "allowed.example" } }
function init()
    mah.db.add_resource_version_from_url(7, "http://blocked.example/v2", "note")
end
`)

	if urls := fetcher.fetched(); len(urls) != 0 {
		t.Fatalf("the version downloader was asked for %v despite the host not being declared", urls)
	}
}

func TestTheVersionFetcherAllowsADeclaredHost(t *testing.T) {
	_, fetcher := enableWithFetcher(t, `
plugin = { name = "fetcher", version = "1.0", api_version = 1,
           capabilities = { "db:write", "http" },
           network = { "allowed.example" } }
function init()
    mah.db.add_resource_version_from_url(7, "http://allowed.example/v2", "note")
end
`)

	urls := fetcher.fetched()
	if len(urls) != 1 || !strings.Contains(urls[0], "allowed.example") {
		t.Fatalf("the declared host was not fetched: %v", urls)
	}
}
