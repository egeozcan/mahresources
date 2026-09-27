package plugin_system

import (
	"context"
	"strings"
	"testing"
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
