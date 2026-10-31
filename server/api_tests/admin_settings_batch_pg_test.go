//go:build postgres

package api_tests

import (
	"errors"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

func TestRuntimeSettings_AtomicSecretBatchPersistenceAndRollbackPG(t *testing.T) {
	db := pgContainer.CreateTestDB(t)
	if err := db.AutoMigrate(&models.RuntimeSetting{}); err != nil {
		t.Fatal(err)
	}
	defaults := application_context.BuildDefaultsFromConfig(&application_context.MahresourcesConfig{DeepSeekAPIKey: "postgres-boot-key"})
	newSettings := func() *application_context.RuntimeSettings {
		return application_context.NewRuntimeSettings(db, application_context.NewStdlibSettingsLogger(), application_context.BuildSpecsExported(), defaults)
	}
	rs := newSettings()
	before := map[string]string{
		"ai_deepseek_api_key": "postgres-saved-key", "ai_mrql_openai_model": "before-model", "ai_openai_api_key": "before-openai-key",
	}
	if err := rs.Apply(before, nil, "initial profile", ""); err != nil {
		t.Fatal(err)
	}
	// The last changed key fails after a deletion and a model update, proving
	// that a failed PostgreSQL statement rolls back the complete profile.
	if err := db.Exec(`ALTER TABLE runtime_settings ADD CONSTRAINT reject_blocked_key CHECK (key <> 'ai_openai_api_key' OR value_json NOT LIKE '%blocked-key%')`).Error; err != nil {
		t.Fatal(err)
	}
	err := rs.Apply(map[string]string{"ai_mrql_openai_model": "after-model", "ai_openai_api_key": "blocked-key"}, []string{"ai_deepseek_api_key"}, "rejected profile", "")
	if !errors.Is(err, application_context.ErrSettingsStorage) || strings.Contains(err.Error(), "blocked-key") {
		t.Fatalf("want sanitized storage error, got %v", err)
	}
	for _, settings := range []*application_context.RuntimeSettings{rs, newSettings()} {
		if err := settings.Load(); err != nil {
			t.Fatal(err)
		}
		for _, view := range settings.List() {
			if expected, ok := before[view.Key]; ok {
				if !view.Overridden || (view.Secret && (!view.Configured || view.Current != nil || view.BootDefault != nil)) || (!view.Secret && view.Current != expected) {
					t.Fatalf("failed batch changed effective value: %+v", view)
				}
			}
		}
	}
	for key, expected := range before {
		var row models.RuntimeSetting
		if err := db.First(&row, "key = ?", key).Error; err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(row.ValueJSON, expected) {
			t.Fatalf("database value changed for %s", key)
		}
	}
	if err := rs.Apply(nil, []string{"ai_deepseek_api_key", "ai_mrql_openai_model", "ai_openai_api_key"}, "reset profile", ""); err != nil {
		t.Fatal(err)
	}
	var rows int64
	if err := db.Model(&models.RuntimeSetting{}).Count(&rows).Error; err != nil || rows != 0 {
		t.Fatalf("reset left %d overrides: %v", rows, err)
	}
}
