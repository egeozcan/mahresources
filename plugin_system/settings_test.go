package plugin_system

import (
	"testing"
)

func TestParseSettingsDeclaration(t *testing.T) {
	script := `
plugin = {
    name = "test-plugin",
    version = "1.0",
    settings = {
        {
            name     = "api_key",
            type     = "password",
            label    = "API Key",
            required = true,
        },
        {
            name     = "endpoint",
            type     = "string",
            label    = "Endpoint URL",
            required = true,
            default  = "https://api.example.com",
        },
        {
            name    = "theme",
            type    = "select",
            label   = "Theme",
            options = {"light", "dark", "auto"},
            default = "auto",
        },
        {
            name    = "enabled",
            type    = "boolean",
            label   = "Enabled",
            default = true,
        },
        {
            name    = "max_retries",
            type    = "number",
            label   = "Max Retries",
            default = 3,
        },
    },
}
`
	defs, err := parseSettingsFromLua(script)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 5 {
		t.Fatalf("expected 5 settings, got %d", len(defs))
	}

	// Setting 0: password, required, no default
	s := defs[0]
	if s.Name != "api_key" {
		t.Errorf("expected name 'api_key', got %q", s.Name)
	}
	if s.Type != "password" {
		t.Errorf("expected type 'password', got %q", s.Type)
	}
	if s.Label != "API Key" {
		t.Errorf("expected label 'API Key', got %q", s.Label)
	}
	if !s.Required {
		t.Error("expected required=true")
	}
	if s.DefaultValue != nil {
		t.Errorf("expected nil default, got %v", s.DefaultValue)
	}

	// Setting 1: string, required, with default
	s = defs[1]
	if s.Name != "endpoint" {
		t.Errorf("expected name 'endpoint', got %q", s.Name)
	}
	if s.Type != "string" {
		t.Errorf("expected type 'string', got %q", s.Type)
	}
	if s.Label != "Endpoint URL" {
		t.Errorf("expected label 'Endpoint URL', got %q", s.Label)
	}
	if !s.Required {
		t.Error("expected required=true")
	}
	if s.DefaultValue != "https://api.example.com" {
		t.Errorf("expected default 'https://api.example.com', got %v", s.DefaultValue)
	}

	// Setting 2: select with options
	s = defs[2]
	if s.Name != "theme" {
		t.Errorf("expected name 'theme', got %q", s.Name)
	}
	if s.Type != "select" {
		t.Errorf("expected type 'select', got %q", s.Type)
	}
	if s.Label != "Theme" {
		t.Errorf("expected label 'Theme', got %q", s.Label)
	}
	if s.Required {
		t.Error("expected required=false")
	}
	if len(s.Options) != 3 {
		t.Fatalf("expected 3 options, got %d", len(s.Options))
	}
	expectedOpts := []string{"light", "dark", "auto"}
	for i, want := range expectedOpts {
		if s.Options[i] != want {
			t.Errorf("option[%d]: expected %q, got %q", i, want, s.Options[i])
		}
	}
	if s.DefaultValue != "auto" {
		t.Errorf("expected default 'auto', got %v", s.DefaultValue)
	}

	// Setting 3: boolean with default true
	s = defs[3]
	if s.Name != "enabled" {
		t.Errorf("expected name 'enabled', got %q", s.Name)
	}
	if s.Type != "boolean" {
		t.Errorf("expected type 'boolean', got %q", s.Type)
	}
	if s.Label != "Enabled" {
		t.Errorf("expected label 'Enabled', got %q", s.Label)
	}
	if s.DefaultValue != true {
		t.Errorf("expected default true, got %v", s.DefaultValue)
	}

	// Setting 4: number with default
	s = defs[4]
	if s.Name != "max_retries" {
		t.Errorf("expected name 'max_retries', got %q", s.Name)
	}
	if s.Type != "number" {
		t.Errorf("expected type 'number', got %q", s.Type)
	}
	if s.Label != "Max Retries" {
		t.Errorf("expected label 'Max Retries', got %q", s.Label)
	}
	defVal, ok := s.DefaultValue.(float64)
	if !ok {
		t.Fatalf("expected default to be float64, got %T", s.DefaultValue)
	}
	if defVal != 3.0 {
		t.Errorf("expected default 3, got %v", defVal)
	}
}

func TestParseSettingsNoSettings(t *testing.T) {
	script := `
plugin = {
    name    = "bare-plugin",
    version = "1.0",
}
`
	defs, err := parseSettingsFromLua(script)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(defs) != 0 {
		t.Errorf("expected 0 settings, got %d", len(defs))
	}
}

func TestValidateSettings_AllTypes(t *testing.T) {
	defs := []SettingDefinition{
		{Name: "api_key", Type: "password", Label: "API Key", Required: true},
		{Name: "endpoint", Type: "string", Label: "Endpoint", Required: true},
		{Name: "theme", Type: "select", Label: "Theme", Options: []string{"light", "dark"}},
		{Name: "enabled", Type: "boolean", Label: "Enabled"},
		{Name: "retries", Type: "number", Label: "Retries"},
	}

	t.Run("all valid", func(t *testing.T) {
		values := map[string]any{
			"api_key":  "secret123",
			"endpoint": "https://example.com",
			"theme":    "dark",
			"enabled":  true,
			"retries":  float64(5),
		}
		errs := ValidateSettings(defs, values)
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("missing required", func(t *testing.T) {
		values := map[string]any{
			"theme":   "light",
			"enabled": true,
		}
		errs := ValidateSettings(defs, values)
		if len(errs) != 2 {
			t.Fatalf("expected 2 errors (api_key and endpoint), got %d: %v", len(errs), errs)
		}
		if errs[0].Field != "api_key" {
			t.Errorf("expected first error field 'api_key', got %q", errs[0].Field)
		}
		if errs[1].Field != "endpoint" {
			t.Errorf("expected second error field 'endpoint', got %q", errs[1].Field)
		}
	})

	t.Run("empty required", func(t *testing.T) {
		values := map[string]any{
			"api_key":  "",
			"endpoint": "https://example.com",
		}
		errs := ValidateSettings(defs, values)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if errs[0].Field != "api_key" {
			t.Errorf("expected error field 'api_key', got %q", errs[0].Field)
		}
	})

	t.Run("invalid select option", func(t *testing.T) {
		values := map[string]any{
			"api_key":  "key",
			"endpoint": "url",
			"theme":    "neon",
		}
		errs := ValidateSettings(defs, values)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if errs[0].Field != "theme" {
			t.Errorf("expected error field 'theme', got %q", errs[0].Field)
		}
	})

	t.Run("invalid boolean", func(t *testing.T) {
		values := map[string]any{
			"api_key":  "key",
			"endpoint": "url",
			"enabled":  "yes",
		}
		errs := ValidateSettings(defs, values)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if errs[0].Field != "enabled" {
			t.Errorf("expected error field 'enabled', got %q", errs[0].Field)
		}
	})

	t.Run("invalid number", func(t *testing.T) {
		values := map[string]any{
			"api_key":  "key",
			"endpoint": "url",
			"retries":  "five",
		}
		errs := ValidateSettings(defs, values)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		if errs[0].Field != "retries" {
			t.Errorf("expected error field 'retries', got %q", errs[0].Field)
		}
	})

	t.Run("int and int64 numbers accepted", func(t *testing.T) {
		values := map[string]any{
			"api_key":  "key",
			"endpoint": "url",
			"retries":  int(3),
		}
		errs := ValidateSettings(defs, values)
		if len(errs) != 0 {
			t.Errorf("expected 0 errors for int value, got %d: %v", len(errs), errs)
		}

		values["retries"] = int64(3)
		errs = ValidateSettings(defs, values)
		if len(errs) != 0 {
			t.Errorf("expected 0 errors for int64 value, got %d: %v", len(errs), errs)
		}
	})
}

func TestCheckRequiredSettings(t *testing.T) {
	defs := []SettingDefinition{
		{Name: "api_key", Type: "password", Label: "API Key", Required: true},
		{Name: "endpoint", Type: "string", Label: "Endpoint", Required: true},
		{Name: "theme", Type: "select", Label: "Theme"},
		{Name: "enabled", Type: "boolean", Label: "Enabled"},
	}

	t.Run("missing required", func(t *testing.T) {
		values := map[string]any{
			"theme":   "dark",
			"enabled": true,
		}
		missing := CheckRequiredSettings(defs, values)
		if len(missing) != 2 {
			t.Fatalf("expected 2 missing, got %d: %v", len(missing), missing)
		}
		if missing[0] != "API Key" {
			t.Errorf("expected 'API Key', got %q", missing[0])
		}
		if missing[1] != "Endpoint" {
			t.Errorf("expected 'Endpoint', got %q", missing[1])
		}
	})

	t.Run("all present", func(t *testing.T) {
		values := map[string]any{
			"api_key":  "secret",
			"endpoint": "https://example.com",
			"theme":    "dark",
		}
		missing := CheckRequiredSettings(defs, values)
		if len(missing) != 0 {
			t.Errorf("expected 0 missing, got %d: %v", len(missing), missing)
		}
	})

	t.Run("empty string counts as missing", func(t *testing.T) {
		values := map[string]any{
			"api_key":  "",
			"endpoint": "https://example.com",
		}
		missing := CheckRequiredSettings(defs, values)
		if len(missing) != 1 {
			t.Fatalf("expected 1 missing, got %d: %v", len(missing), missing)
		}
		if missing[0] != "API Key" {
			t.Errorf("expected 'API Key', got %q", missing[0])
		}
	})

	t.Run("nil value counts as missing", func(t *testing.T) {
		values := map[string]any{
			"api_key":  nil,
			"endpoint": "https://example.com",
		}
		missing := CheckRequiredSettings(defs, values)
		if len(missing) != 1 {
			t.Fatalf("expected 1 missing, got %d: %v", len(missing), missing)
		}
		if missing[0] != "API Key" {
			t.Errorf("expected 'API Key', got %q", missing[0])
		}
	})
}
