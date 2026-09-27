package plugin_system

import (
	"context"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

const secretSettingPlugin = `
plugin = { name = "secret-holder", version = "1.0", api_version = 1, capabilities = { "actions", "pages" },
           settings = { { name = "api_key", type = "password", label = "API key" },
                        { name = "region", type = "string", label = "Region" } } }

function init()
    mah.action({ id = "breaks", label = "Breaks", entity = "resource", handler = function(ctx)
        error("refused key " .. ctx.settings.api_key)
    end })
    mah.action({ id = "aborts", label = "Aborts", entity = "resource", handler = function(ctx)
        mah.abort("refused key " .. ctx.settings.api_key)
    end })
    mah.action({ id = "answers", label = "Answers", entity = "resource", handler = function(ctx)
        return { success = true, message = "used " .. ctx.settings.api_key,
                 data = { [ctx.settings.api_key] = ctx.settings.api_key, region = ctx.settings.region } }
    end })
    mah.page("broken", function(ctx)
        error("page refused key " .. mah.get_setting("api_key"))
    end)
end
`

// TestAPluginSecretIsRedactedFromWhatItsSynchronousCallsReturn pins the plugin
// calls that answer a person directly, rather than through a Job: an action's
// error, its abort reason and its result, and a page's error, all carry the
// plugin's password-typed setting as "[redacted]" wherever it appears. A setting
// that is not a password is not a secret and is left as it is.
func TestAPluginSecretIsRedactedFromWhatItsSynchronousCallsReturn(t *testing.T) {
	const secret = "sk-live-0b1c2d3e4f"
	dir := t.TempDir()
	writePlugin(t, dir, "secret-holder", secretSettingPlugin)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("plugin manager: %v", err)
	}
	t.Cleanup(pm.Close)
	pm.SetPluginSettings("secret-holder", map[string]any{"api_key": secret, "region": "eu-west"})
	if err := pm.EnablePlugin("secret-holder"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	_, err = pm.RunAction(context.Background(), "secret-holder", "breaks", 1, map[string]any{}, "")
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "refused key [redacted]") {
		t.Fatalf("the action's error is %v, want it with the key redacted", err)
	}
	aborted, err := pm.RunAction(context.Background(), "secret-holder", "aborts", 1, map[string]any{}, "")
	if err != nil || strings.Contains(aborted.Message, secret) || !strings.Contains(aborted.Message, "[redacted]") {
		t.Fatalf("the abort answered %+v, %v, want its reason with the key redacted", aborted, err)
	}
	answered, err := pm.RunAction(context.Background(), "secret-holder", "answers", 1, map[string]any{}, "")
	if err != nil {
		t.Fatalf("run the answering action: %v", err)
	}
	if strings.Contains(answered.Message, secret) {
		t.Fatalf("the result's message carries the key: %q", answered.Message)
	}
	for key, value := range answered.Data {
		if strings.Contains(key, secret) || value == secret {
			t.Fatalf("the result's data carries the key: %v", answered.Data)
		}
	}
	if answered.Data["region"] != "eu-west" {
		t.Fatalf("a setting that is not a password was redacted: %v", answered.Data)
	}

	_, err = pm.HandlePage(context.Background(), "secret-holder", "broken", PageContext{})
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("the page's error is %v, want it with the key redacted", err)
	}
}

// A lowercase key, so it can also stand as a metric key.
const lowerSecret = "sklive0b1c2d3e4f"

const secretWorkerPlugin = `
plugin = { name = "secret-worker", version = "1.0", api_version = 1,
           capabilities = { "actions", "jobs", "http", "render" },
           settings = { { name = "api_key", type = "password", label = "API key" } } }

function init()
    mah.action({ id = "reports", label = "Reports", entity = "resource", async = true, handler = function(ctx)
        local key = ctx.settings.api_key
        mah.job_progress(ctx.job_id, { completed = 1, total = 2, unit = key, message = "unit is " .. key,
            metrics = { { key = key, label = "keyed", value = 1 }, { key = "rows", label = "rows of " .. key, value = 2 } } })
        mah.start_job("titled " .. key, function(job_id) end)
        mah.sleep(0.3)
    end })
    mah.action({ id = "calls", label = "Calls", entity = "resource", handler = function(ctx)
        local key = ctx.settings.api_key
        mah.http.get_sync("http://127.0.0.1:1/refused?k=" .. key)
        mah.http.get("http://127.0.0.1:1/queued?k=" .. key, function(resp) error("callback saw " .. key) end)
        return { success = true }
    end })
    mah.shortcode({ name = "leaky", label = "Leaky", render = function(ctx)
        error("shortcode saw " .. mah.get_setting("api_key"))
    end })
end
`

const secretInitPlugin = `
plugin = { name = "secret-init", version = "1.0", api_version = 1,
           settings = { { name = "api_key", type = "password", label = "API key" } } }

function init()
    error("init saw " .. mah.get_setting("api_key"))
end
`

// TestAPluginSecretIsRedactedFromTheJobPlaneAndTheHostsLogs pins the rest of the
// scope: the Job plane's title, unit and metrics (a metric whose key is the
// secret is dropped), and the log lines the host writes from a plugin error or a
// refused request: an egress refusal, an HTTP callback's error, a shortcode
// preview's error and a failing init(), whose error leaves EnablePlugin redacted.
func TestAPluginSecretIsRedactedFromTheJobPlaneAndTheHostsLogs(t *testing.T) {
	var logged strings.Builder
	var logMu sync.Mutex
	previous := log.Writer()
	log.SetOutput(writerFunc(func(p []byte) (int, error) {
		logMu.Lock()
		defer logMu.Unlock()
		logged.Write(p)
		return previous.Write(p)
	}))
	t.Cleanup(func() { log.SetOutput(previous) })
	readLog := func() string {
		logMu.Lock()
		defer logMu.Unlock()
		return logged.String()
	}

	dir := t.TempDir()
	writePlugin(t, dir, "secret-worker", secretWorkerPlugin)
	writePlugin(t, dir, "secret-init", secretInitPlugin)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("plugin manager: %v", err)
	}
	t.Cleanup(pm.Close)
	pm.SetPluginSettings("secret-worker", map[string]any{"api_key": lowerSecret})
	pm.SetPluginSettings("secret-init", map[string]any{"api_key": lowerSecret})

	if err := pm.EnablePlugin("secret-init"); err == nil || strings.Contains(err.Error(), lowerSecret) ||
		!strings.Contains(err.Error(), "init saw [redacted]") {
		t.Fatalf("a failing init() answered %v, want its error with the key redacted", err)
	}
	if err := pm.EnablePlugin("secret-worker"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	events := pm.SubscribeActionJobs()
	t.Cleanup(func() { pm.UnsubscribeActionJobs(events) })
	if _, err := pm.RunActionAsyncForHost(&HostJobRef{JobID: "reports", Handle: "reports", Sink: &recordingSink{}},
		nil, "secret-worker", "reports", 1, nil, ""); err != nil {
		t.Fatalf("run: %v", err)
	}
	var sawUnit, sawTitle bool
	waitUntil(t, "the report and the started job to be published", 5*time.Second, func() bool {
		for {
			select {
			case event := <-events:
				snap := event.Job
				if strings.Contains(snap.Unit, lowerSecret) || strings.Contains(snap.Message, lowerSecret) ||
					strings.Contains(snap.Label, lowerSecret) {
					t.Fatalf("a %s event carries the key: unit %q message %q label %q", event.Type, snap.Unit, snap.Message, snap.Label)
				}
				for _, metric := range snap.Metrics {
					if strings.Contains(metric.Key, lowerSecret) || strings.Contains(metric.Label, lowerSecret) {
						t.Fatalf("a %s event carries the key in a metric: %+v", event.Type, metric)
					}
				}
				if snap.Unit == "[redacted]" {
					sawUnit = true
				}
				if snap.Label == "titled [redacted]" {
					sawTitle = true
				}
			default:
				return sawUnit && sawTitle
			}
		}
	})

	if _, err := pm.RunAction(context.Background(), "secret-worker", "calls", 1, map[string]any{}, ""); err != nil {
		t.Fatalf("run the calling action: %v", err)
	}
	waitUntil(t, "the refusals and the callback's error to be logged", 5*time.Second, func() bool {
		text := readLog()
		return strings.Contains(text, "refused GET") && strings.Contains(text, "HTTP callback error")
	})

	if _, err := pm.renderShortcodeForDocs(context.Background(), "secret-worker", "plugin:secret-worker:leaky",
		nil, nil, "", false); err == nil || strings.Contains(err.Error(), lowerSecret) {
		t.Fatalf("a shortcode preview's error is %v, want it with the key redacted", err)
	}

	if text := readLog(); strings.Contains(text, lowerSecret) {
		t.Fatalf("the server log carries the key:\n%s", text)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
