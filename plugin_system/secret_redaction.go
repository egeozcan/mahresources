package plugin_system

import (
	"sort"
	"strings"
)

// A plugin's password-typed settings are operator secrets: an API key the
// operator entered on the plugin's settings page. Its handler receives them
// (ctx.settings) and can echo them, into an error above all, and the host
// publishes a handler's text where the Job's owner reads it: the Job's failure,
// its progress and completion messages, its result, the panel's entries and their
// SSE events, and the logs. Every one of those takes the text through
// redactSecrets first, with the values read before any lock the text is stored
// under (pluginSecrets takes pm.mu).

// secretRedactionMarker replaces a secret in published text.
const secretRedactionMarker = "[redacted]"

// pluginSecrets answers the current values of a plugin's password-typed
// settings, longest first so a value that contains another is replaced whole.
func (pm *PluginManager) pluginSecrets(pluginName string) []string {
	plugin := pm.GetDiscoveredPlugin(pluginName)
	if plugin == nil {
		return nil
	}
	values := pm.GetPluginSettings(pluginName)
	var secrets []string
	for _, def := range plugin.Settings {
		if def.Type != "password" {
			continue
		}
		if value, ok := values[def.Name].(string); ok && value != "" {
			secrets = append(secrets, value)
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	return secrets
}

// RedactPluginSecrets replaces every occurrence of a plugin's password-typed
// setting values in text.
func (pm *PluginManager) RedactPluginSecrets(pluginName, text string) string {
	if text == "" {
		return text
	}
	return redactSecrets(text, pm.pluginSecrets(pluginName))
}

// redactSecrets replaces every occurrence of each secret in text.
func redactSecrets(text string, secrets []string) string {
	for _, secret := range secrets {
		text = strings.ReplaceAll(text, secret, secretRedactionMarker)
	}
	return text
}

// redactSecretsIn is redactSecrets over a result table, at every depth and in
// keys as well as values: a key is as durable and as visible as a value.
func redactSecretsIn(value any, secrets []string) any {
	if len(secrets) == 0 {
		return value
	}
	switch typed := value.(type) {
	case string:
		return redactSecrets(typed, secrets)
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, nested := range typed {
			redacted[redactSecrets(key, secrets)] = redactSecretsIn(nested, secrets)
		}
		return redacted
	case []any:
		redacted := make([]any, len(typed))
		for i, item := range typed {
			redacted[i] = redactSecretsIn(item, secrets)
		}
		return redacted
	default:
		return value
	}
}

// redactResult is redactSecretsIn for a result table.
func redactResult(result map[string]any, secrets []string) map[string]any {
	if result == nil || len(secrets) == 0 {
		return result
	}
	redacted, _ := redactSecretsIn(result, secrets).(map[string]any)
	return redacted
}

// redactedError is a plugin Lua call's error as the host hands it on: its text
// has the plugin's secrets redacted, and it still unwraps to the error it
// carries, so a caller asking what kind of failure it was is answered as before.
type redactedError struct {
	text string
	err  error
}

func (e redactedError) Error() string { return e.text }
func (e redactedError) Unwrap() error { return e.err }

// pluginCallError wraps err, from a Lua call of pluginName, as prefix and the
// error's redacted text.
func (pm *PluginManager) pluginCallError(pluginName, prefix string, err error) error {
	return redactedError{text: prefix + ": " + pm.RedactPluginSecrets(pluginName, err.Error()), err: err}
}
