package plugin_system

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A plugin must be able to tell a short body from a cut one.
//
// The host caps a response at maxHttpResponseBody and drops the rest. That is
// logged for the operator and documented, but neither reaches the plugin: what
// Lua receives is a body of exactly the limit, indistinguishable from a
// document that happened to be that long. Every author who decodes JSON,
// follows a pagination cursor or hashes a payload is then working from a
// silently mutilated string, and the failure surfaces somewhere else entirely.
//
// So the response table carries `truncated`, and these tests pin three things
// about it that are each easy to get wrong:
//
//   - It is present on *every* successful response, false when nothing was cut.
//     A field that only appears when truncation happened cannot be told from an
//     older host or from a bug, so authors would have to write
//     `resp.truncated == true` defensively forever.
//   - Both paths carry it. The synchronous call and the async callback build
//     their tables at two separate sites, and a flag that is true on one of them
//     is worse than no flag: it makes the absent case on the other path read as
//     "complete".
//   - The error tables do not carry it, on either path. That is a decision, not
//     an omission. An error table carries no `body` either -- it is a different
//     shape, documented as "error, url and method instead" -- and a field
//     asserting a nonexistent body was complete says something untrue. Lua's
//     nil is falsy, so `if resp.truncated` still reads correctly there.
//
// The boundary is where the meaning lives. Both paths read through
// io.LimitReader(resp.Body, maxHttpResponseBody+1) and compare
// len(bodyBytes) > maxHttpResponseBody, which is right, but only by one byte in
// each direction: a reader sized at the limit can never satisfy the comparison
// and nothing would ever be reported cut, and a >= comparison would report a
// body of exactly the limit as cut and hand back a string one byte short of
// what arrived. Neither is visible to a test using a body 1000 bytes over. The
// pair at limit and limit+1 is what separates them.

// luaReport is the plugin-side reporter these tests share. It turns a response
// table into one line naming the body length, the *type* of the truncated
// field, and its value.
//
// The type is in there because Lua cannot tell nil from false in a boolean
// test, and those are exactly the two answers this item is about: "the field is
// present and says nothing was cut" and "the field was never set". `if
// resp.truncated` reads them identically, so a test asserting only truthiness
// would pass against the unfixed host and prove nothing.
const luaReport = `
local function report(resp)
    if resp.error then
        return "ERR:" .. resp.error
    end
    return #resp.body .. "|" .. type(resp.truncated) .. "|" .. tostring(resp.truncated)
end

local function error_report(resp)
    return type(resp.error) .. "/" .. type(resp.truncated) .. "/" .. type(resp.body)
end
`

// sizedBodyServer answers a request for /<n> with a body of exactly n bytes.
//
// One server serving every size a test needs, rather than one per size: the
// egress manifest names a single host, and httptest falls back to [::1] when it
// cannot listen on 127.0.0.1. Two servers could land on different loopback
// addresses and the test would then fail on the network rule instead of on what
// it is asserting.
func sizedBodyServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil {
			http.Error(w, "the path names the body size", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, strings.Repeat("A", n))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// sizedBodyURL is the URL on srv that answers with a body of exactly n bytes.
func sizedBodyURL(srv *httptest.Server, n int) string {
	return fmt.Sprintf("%s/%d", srv.URL, n)
}

// truncationManager writes a plugin that may reach srvURL's host, enables it,
// and hands back the running manager. luaReport is always in scope.
func truncationManager(t *testing.T, srvURL, luaBody string) *PluginManager {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "http-test", strings.Join([]string{
		httpTestPlugin(t, "http-test", srvURL),
		luaReport,
		luaBody,
	}, "\n"))

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pm.Close() })

	if err := pm.EnablePlugin("http-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	return pm
}

// syncReportPlugin is the Lua for a test that makes one get_sync call and
// renders what came back.
func syncReportPlugin(url string) string {
	return fmt.Sprintf(`
function init()
    mah.inject("page_bottom", function(ctx)
        return report(mah.http.get_sync(%q))
    end)
end
`, url)
}

// asyncReportPlugin is the same for a callback-based get. The slot stays empty
// until the callback has landed, which is what pollSlot waits on.
func asyncReportPlugin(url string) string {
	return fmt.Sprintf(`
http_result = ""

function init()
    mah.http.get(%q, function(resp)
        http_result = report(resp)
    end)
    mah.inject("page_bottom", function(ctx)
        return http_result
    end)
end
`, url)
}

func TestHttpApi_SyncTruncatedFalseAtExactlyTheLimit(t *testing.T) {
	srv := sizedBodyServer(t)
	pm := truncationManager(t, srv.URL, syncReportPlugin(sizedBodyURL(srv, maxHttpResponseBody)))

	result := pm.RenderSlot(context.Background(), "page_bottom", map[string]any{}, nil)
	want := fmt.Sprintf("%d|boolean|false", maxHttpResponseBody)
	if result != want {
		t.Errorf("a body of exactly the limit reported %q, want %q", result, want)
	}
}

func TestHttpApi_SyncTruncatedTrueOneByteOverTheLimit(t *testing.T) {
	srv := sizedBodyServer(t)
	pm := truncationManager(t, srv.URL, syncReportPlugin(sizedBodyURL(srv, maxHttpResponseBody+1)))

	// The body is still cut to the limit, not to the limit+1 the reader was
	// allowed to fetch: the extra byte exists only to make the overflow
	// detectable and must never reach the plugin.
	result := pm.RenderSlot(context.Background(), "page_bottom", map[string]any{}, nil)
	want := fmt.Sprintf("%d|boolean|true", maxHttpResponseBody)
	if result != want {
		t.Errorf("a body one byte over the limit reported %q, want %q", result, want)
	}
}

func TestHttpApi_AsyncTruncatedFalseAtExactlyTheLimit(t *testing.T) {
	srv := sizedBodyServer(t)
	pm := truncationManager(t, srv.URL, asyncReportPlugin(sizedBodyURL(srv, maxHttpResponseBody)))

	result := pollSlot(t, pm, "page_bottom", 10*time.Second)
	want := fmt.Sprintf("%d|boolean|false", maxHttpResponseBody)
	if result != want {
		t.Errorf("a body of exactly the limit reported %q, want %q", result, want)
	}
}

func TestHttpApi_AsyncTruncatedTrueOneByteOverTheLimit(t *testing.T) {
	srv := sizedBodyServer(t)
	pm := truncationManager(t, srv.URL, asyncReportPlugin(sizedBodyURL(srv, maxHttpResponseBody+1)))

	result := pollSlot(t, pm, "page_bottom", 10*time.Second)
	want := fmt.Sprintf("%d|boolean|true", maxHttpResponseBody)
	if result != want {
		t.Errorf("a body one byte over the limit reported %q, want %q", result, want)
	}
}

// TestHttpApi_TruncatedFalseOnAnOrdinaryResponse is the requirement the
// boundary tests do not state: the field is on every successful response, not
// only on the ones large enough for the limit to be in play. An implementation
// that sets it inside the `if too long` branch alone satisfies both truncated
// cases above and fails here.
func TestHttpApi_TruncatedFalseOnAnOrdinaryResponse(t *testing.T) {
	const smallBody = 11
	srv := sizedBodyServer(t)
	url := sizedBodyURL(srv, smallBody)

	pm := truncationManager(t, srv.URL, fmt.Sprintf(`
async_result = ""

function init()
    mah.http.get(%q, function(resp)
        async_result = report(resp)
    end)
    mah.inject("page_bottom", function(ctx)
        if async_result == "" then
            return ""
        end
        return "sync=" .. report(mah.http.get_sync(%q)) .. " async=" .. async_result
    end)
end
`, url, url))

	result := pollSlot(t, pm, "page_bottom", 10*time.Second)
	want := fmt.Sprintf("sync=%d|boolean|false async=%d|boolean|false", smallBody, smallBody)
	if result != want {
		t.Errorf("a small complete body reported %q, want %q", result, want)
	}
}

// TestHttpApi_TruncatedIsPerResponse pins the flag to the response it describes
// rather than to the plugin or the manager. Two requests are in flight at once,
// one over the limit and one well under it, and their callbacks are delivered
// on the same VM in whatever order the network settles: a flag kept anywhere
// but on the response table would leak the truncated answer onto the complete
// one, or the other way round.
func TestHttpApi_TruncatedIsPerResponse(t *testing.T) {
	const smallBody = 11
	srv := sizedBodyServer(t)

	pm := truncationManager(t, srv.URL, fmt.Sprintf(`
big_result = ""
small_result = ""

function init()
    mah.http.get(%q, function(resp)
        big_result = report(resp)
    end)
    mah.http.get(%q, function(resp)
        small_result = report(resp)
    end)
    mah.inject("page_bottom", function(ctx)
        if big_result == "" or small_result == "" then
            return ""
        end
        return "big=" .. big_result .. " small=" .. small_result
    end)
end
`, sizedBodyURL(srv, maxHttpResponseBody+1), sizedBodyURL(srv, smallBody)))

	result := pollSlot(t, pm, "page_bottom", 15*time.Second)
	want := fmt.Sprintf("big=%d|boolean|true small=%d|boolean|false", maxHttpResponseBody, smallBody)
	if result != want {
		t.Errorf("two concurrent responses reported %q, want %q", result, want)
	}
}

// TestHttpApi_SyncErrorResponseOmitsTruncated pins the decided error shape on
// the synchronous path: `error`, `url` and `method`, and nothing describing a
// body that was never read. Both of buildSyncErrorResponse's populations are
// covered -- a request refused before it left (the scheme check) and one that
// failed on the wire -- because a fix editing the code beside the body read is
// most likely to reach for the second.
func TestHttpApi_SyncErrorResponseOmitsTruncated(t *testing.T) {
	srv := sizedBodyServer(t)
	dead := sizedBodyURL(srv, 1)
	// Closed before the plugin asks for it, so the connection is refused at a
	// host the manifest does list: the point is the transport failure, not the
	// egress rule, which produces a different table.
	srv.Close()

	pm := truncationManager(t, srv.URL, fmt.Sprintf(`
function init()
    mah.inject("page_bottom", function(ctx)
        local refused = mah.http.get_sync(%q)
        local scheme = mah.http.get_sync("ftp://example.com/file")
        return "refused=" .. error_report(refused) .. " scheme=" .. error_report(scheme)
    end)
end
`, dead))

	result := pm.RenderSlot(context.Background(), "page_bottom", map[string]any{}, nil)
	want := "refused=string/nil/nil scheme=string/nil/nil"
	if result != want {
		t.Errorf("sync error tables reported %q, want %q", result, want)
	}
}

// TestHttpApi_AsyncErrorResponseOmitsTruncated is the same decision on the
// async path, where the error tables are built at four separate sites. The two
// covered here are the ones a plugin can reach without shutting the manager
// down: the transport failure inside executeHttpRequest, which sits directly
// above the body read, and queueErrorCallback's refusal before dispatch.
func TestHttpApi_AsyncErrorResponseOmitsTruncated(t *testing.T) {
	srv := sizedBodyServer(t)
	dead := sizedBodyURL(srv, 1)
	srv.Close()

	pm := truncationManager(t, srv.URL, fmt.Sprintf(`
refused_result = ""
scheme_result = ""

function init()
    mah.http.get(%q, function(resp)
        refused_result = error_report(resp)
    end)
    mah.http.get("ftp://example.com/file", function(resp)
        scheme_result = error_report(resp)
    end)
    mah.inject("page_bottom", function(ctx)
        if refused_result == "" or scheme_result == "" then
            return ""
        end
        return "refused=" .. refused_result .. " scheme=" .. scheme_result
    end)
end
`, dead))

	result := pollSlot(t, pm, "page_bottom", 10*time.Second)
	want := "refused=string/nil/nil scheme=string/nil/nil"
	if result != want {
		t.Errorf("async error tables reported %q, want %q", result, want)
	}
}

// pluginLuaApiPath is the only page describing the response table, so it is the
// only place an author learns the field exists.
const pluginLuaApiPath = "../docs-site/docs/features/plugin-lua-api.md"

// TestPluginLuaApiDocumentsTruncatedField keeps the page honest about the
// field. A flag nobody is told about leaves every author still writing against
// a body they cannot trust, which is the defect this item is fixing.
func TestPluginLuaApiDocumentsTruncatedField(t *testing.T) {
	row, ok := docsTableRow(t, "`truncated`")
	if !ok {
		t.Fatalf("%s documents no `truncated` field in the response table", pluginLuaApiPath)
	}
	if !strings.Contains(row, "boolean") {
		t.Errorf("the `truncated` row does not give its type as boolean: %q", row)
	}
}

// TestPluginLuaApiDocumentsResponseBodyLimit pins the documented cap to the
// constant. The page already carries the right number; this stops the two from
// drifting once the flag sends authors looking for what the limit actually is.
func TestPluginLuaApiDocumentsResponseBodyLimit(t *testing.T) {
	row, ok := docsTableRow(t, "Maximum response body")
	if !ok {
		t.Fatalf("%s documents no maximum response body", pluginLuaApiPath)
	}
	want := fmt.Sprintf("%d MB", maxHttpResponseBody/(1024*1024))
	if !strings.Contains(row, want) {
		t.Errorf("the documented maximum response body is %q, want %q", row, want)
	}
}

// docsTableRow returns the markdown table row whose first cell is label.
func docsTableRow(t *testing.T, label string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(pluginLuaApiPath)
	if err != nil {
		t.Fatalf("reading %s: %v", pluginLuaApiPath, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 3 {
			continue
		}
		if strings.TrimSpace(cells[1]) == label {
			return strings.TrimSpace(line), true
		}
	}
	return "", false
}
