package plugin_system

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// renderUtil boots a one-plugin manager whose "page_bottom" slot runs the given Lua
// and returns what it rendered.
func renderUtil(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "util-test", `
plugin = { name = "util-test", version = "1.0", description = "util api test" }
function init()
    mah.inject("page_bottom", function(ctx)
`+body+`
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Close)
	if err := mgr.EnablePlugin("util-test"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	return mgr.RenderSlot(context.Background(), "page_bottom", map[string]any{}, nil)
}

// The sandbox opens no os library, so before mah.util a plugin could not read
// the clock at all — which means it could not timestamp anything or expire a
// cached value.
func TestUtilApi_Now(t *testing.T) {
	// Formatted in Lua: tostring() on a number this large gives scientific
	// notation, which is not what a plugin would print.
	got := renderUtil(t, `return string.format("%d", mah.util.now())`)
	secs, err := strconv.ParseInt(got, 10, 64)
	if err != nil {
		t.Fatalf("now() did not return a number: %q", got)
	}
	if delta := secs - time.Now().Unix(); delta < -60 || delta > 60 {
		t.Errorf("now() = %v, want within 60s of now", secs)
	}
}

func TestUtilApi_NowISOIsUTC(t *testing.T) {
	got := renderUtil(t, `return mah.util.now_iso()`)
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("now_iso() = %q, not RFC3339: %v", got, err)
	}
	// UTC, deliberately: this repo has been bitten by local-offset timestamps
	// comparing lexicographically against UTC bounds.
	if !strings.HasSuffix(got, "Z") {
		t.Errorf("now_iso() = %q, want a UTC timestamp ending in Z", got)
	}
	if time.Since(parsed) > time.Minute || time.Since(parsed) < -time.Minute {
		t.Errorf("now_iso() = %q, too far from now", got)
	}
}

// today() is the calendar date in the server's zone. A note's start and end
// dates are wall-clock values without a zone, so a plugin asking whether one has
// passed needs the local date; the date part of now_iso() is UTC's, a day away
// from it for hours of every day east or west of UTC. The server's zone is fixed
// at process start, so each zone runs in a child process. Between them the two
// zones differ from UTC's date at every hour.
func TestUtilApi_TodayIsTheServersCalendarDate(t *testing.T) {
	if os.Getenv("MAH_UTIL_TODAY_CHILD") == "1" {
		t.Logf("TODAY=%s", renderUtil(t, `return mah.util.today()`))
		return
	}
	marker := regexp.MustCompile(`TODAY=(\S*)`)
	for _, zone := range []string{"Etc/GMT-14", "Etc/GMT+12"} {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatalf("load %s: %v", zone, err)
		}
		before := time.Now().In(loc).Format("2006-01-02")
		cmd := exec.Command(os.Args[0], "-test.run=^TestUtilApi_TodayIsTheServersCalendarDate$", "-test.v", "-test.count=1")
		cmd.Env = append(os.Environ(), "MAH_UTIL_TODAY_CHILD=1", "TZ="+zone)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("child in %s: %v\n%s", zone, err, out)
		}
		after := time.Now().In(loc).Format("2006-01-02")
		found := marker.FindSubmatch(out)
		if found == nil {
			t.Fatalf("child in %s printed no date:\n%s", zone, out)
		}
		if got := string(found[1]); got != before && got != after {
			t.Errorf("today() in %s = %q, want %q (UTC's date is %s)", zone, got, before, time.Now().UTC().Format("2006-01-02"))
		}
	}
}

func TestUtilApi_Base64RoundTrip(t *testing.T) {
	got := renderUtil(t, `
        local encoded = mah.util.base64.encode("Hello, World!")
        local decoded, err = mah.util.base64.decode(encoded)
        if err ~= nil then return "err:" .. err end
        return encoded .. "|" .. decoded
`)
	want := "SGVsbG8sIFdvcmxkIQ==|Hello, World!"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// get_resource_data hands Lua base64; without a decoder a plugin had to
// hand-roll one (data-views does exactly that).
func TestUtilApi_Base64DecodeRejectsGarbage(t *testing.T) {
	got := renderUtil(t, `
        local decoded, err = mah.util.base64.decode("!!!not base64!!!")
        if decoded ~= nil then return "unexpectedly decoded" end
        if err == nil then return "NO ERROR" end
        return "rejected"
`)
	if got != "rejected" {
		t.Errorf("got %q, want %q", got, "rejected")
	}
}

func TestUtilApi_Sha256(t *testing.T) {
	// echo -n "abc" | shasum -a 256
	got := renderUtil(t, `return mah.util.sha256("abc")`)
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The point of hmac_sha256: a plugin exposing a mah.api endpoint can verify an
// inbound webhook signature instead of trusting any caller.
func TestUtilApi_HmacSha256(t *testing.T) {
	// Standard RFC 4231 test case 2.
	got := renderUtil(t, `return mah.util.hmac_sha256("Jefe", "what do ya want for nothing?")`)
	want := "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUtilApi_Hex(t *testing.T) {
	got := renderUtil(t, `
        local encoded = mah.util.hex.encode("abc")
        local decoded, err = mah.util.hex.decode(encoded)
        if err ~= nil then return "err:" .. err end
        return encoded .. "|" .. decoded
`)
	want := "616263|abc"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Constant-time comparison, so a plugin verifying a signature does not leak it
// through timing by using ==.
func TestUtilApi_SecureCompare(t *testing.T) {
	got := renderUtil(t, `
        local same = mah.util.secure_compare("abcdef", "abcdef")
        local diff = mah.util.secure_compare("abcdef", "abcdeg")
        return tostring(same) .. "|" .. tostring(diff)
`)
	if got != "true|false" {
		t.Errorf("got %q, want %q", got, "true|false")
	}
}

// The module must not smuggle in filesystem, process or network reach.
func TestUtilApi_SandboxPostureUnchanged(t *testing.T) {
	got := renderUtil(t, `
        local names = {}
        for k, _ in pairs(mah.util) do names[#names + 1] = k end
        table.sort(names)
        return table.concat(names, ",")
`)
	want := "base64,hex,hmac_sha256,now,now_iso,secure_compare,sha256,today"
	if got != want {
		t.Errorf("mah.util surface = %q, want %q", got, want)
	}
}
