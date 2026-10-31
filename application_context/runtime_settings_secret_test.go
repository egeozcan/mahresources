package application_context

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type settingsAuditRecorder struct{ entries []string }

func (a *settingsAuditRecorder) Audit(action, entityType, entityName, message string, details map[string]any, actor string) {
	b, _ := json.Marshal(details)
	a.entries = append(a.entries, action+entityType+entityName+message+string(b)+actor)
}

func secretSettingsFixture(t *testing.T) (*RuntimeSettings, *stubLogger) {
	t.Helper()
	log := &stubLogger{}
	specs := map[string]SettingSpec{
		"api_key": {Key: "api_key", Type: SettingTypeString, Secret: true},
		"model":   {Key: "model", Type: SettingTypeString},
	}
	return NewRuntimeSettings(newTestDB(t), log, specs, map[string]any{"api_key": "boot-secret-value", "model": "boot-model"}), log
}

func TestRuntimeSettings_SecretsAreWriteOnlyAcrossSaveClearResetAndReload(t *testing.T) {
	rs, bootLog := secretSettingsFixture(t)
	audit := &settingsAuditRecorder{}
	rs.SetAuditor(audit)
	assertView := func(configured, overridden bool) {
		t.Helper()
		var secret SettingView
		for _, v := range rs.List() {
			if v.Key == "api_key" {
				secret = v
			}
		}
		if !secret.Secret || secret.Configured != configured || !secret.BootConfigured || secret.Overridden != overridden {
			t.Fatalf("unexpected secret status: %+v", secret)
		}
		if secret.Current != nil || secret.BootDefault != nil {
			t.Fatal("secret values must be absent from the public view")
		}
		b, err := json.Marshal(rs.List())
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{"boot-secret-value", "saved-secret-value"} {
			if strings.Contains(string(b), value) {
				t.Fatalf("settings JSON exposed %s", value)
			}
		}
	}
	assertView(true, false)
	if err := rs.Set("api_key", "saved-secret-value", "rotation", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	assertView(true, true)
	reloaded := NewRuntimeSettings(rs.db, bootLog, rs.specs, rs.defaults)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if got := reloaded.snapshot([]string{"api_key"})["api_key"]; got != "saved-secret-value" {
		t.Fatal("the saved key did not survive reload")
	}
	if err := rs.Set("api_key", "", "disable", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	assertView(false, true)
	if got := rs.snapshot([]string{"api_key"})["api_key"]; got != "" {
		t.Fatal("clearing the key must suppress the boot fallback")
	}
	if err := rs.Reset("api_key", "restore environment", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	assertView(true, false)
	for _, secret := range []string{"boot-secret-value", "saved-secret-value"} {
		if bootLog.contains(secret) || strings.Contains(strings.Join(audit.entries, "\n"), secret) {
			t.Fatal("startup or audit logging exposed a credential")
		}
	}
	if len(audit.entries) != 3 {
		t.Fatalf("want audits for save, clear and reset, got %d", len(audit.entries))
	}
}

func TestRuntimeSettings_SecretValidationAndDecodeErrorsDoNotEchoValues(t *testing.T) {
	rs, log := secretSettingsFixture(t)
	const secret = "invalid-secret-value"
	spec := rs.specs["api_key"]
	spec.StringValidator = func(value string) error { return fmt.Errorf("invalid credential %s", value) }
	rs.specs["api_key"] = spec
	if err := rs.Set("api_key", secret, "", ""); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("want a safe validation error, got %v", err)
	}
	encoded, _ := encodeSettingValue("string", secret)
	if err := rs.db.Create(&models.RuntimeSetting{Key: "api_key", ValueJSON: string(encoded)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := rs.Load(); err != nil {
		t.Fatal(err)
	}
	if log.contains(secret) {
		t.Fatal("load validation logged the stored credential")
	}
	if err := rs.db.Model(&models.RuntimeSetting{}).Where("key = ?", "api_key").Update("value_json", `{"type":"`+secret+`","value":"x"}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := rs.Load(); err != nil {
		t.Fatal(err)
	}
	if log.contains(secret) {
		t.Fatal("load decoding logged the stored credential")
	}
}

func TestRuntimeSettings_BatchRollsBackDatabaseAndCacheWithoutSQLSecretLogs(t *testing.T) {
	rs, _ := secretSettingsFixture(t)
	const secret = "sql-error-secret-value"
	if err := rs.db.Exec(`CREATE TRIGGER reject_model BEFORE INSERT ON runtime_settings WHEN NEW.key = 'model' BEGIN SELECT RAISE(ABORT, 'sql-error-secret-value'); END`).Error; err != nil {
		t.Fatal(err)
	}
	var sqlLog bytes.Buffer
	rs.db = rs.db.Session(&gorm.Session{Logger: logger.New(log.New(&sqlLog, "", 0), logger.Config{
		LogLevel: logger.Info, SlowThreshold: time.Nanosecond,
	})})
	audit := &settingsAuditRecorder{}
	rs.SetAuditor(audit)
	err := rs.Apply(map[string]string{"api_key": secret, "model": "new-model"}, nil, "", "")
	if !errors.Is(err, ErrSettingsStorage) || strings.Contains(err.Error(), secret) {
		t.Fatalf("want a safe storage error, got %v", err)
	}
	if strings.Contains(sqlLog.String(), secret) {
		t.Fatalf("credential escaped through the SQL logger: %s", sqlLog.String())
	}
	snapshot := rs.snapshot([]string{"api_key", "model"})
	if snapshot["api_key"] != "boot-secret-value" || snapshot["model"] != "boot-model" {
		t.Fatalf("a failed transaction changed the cache: %v", snapshot)
	}
	var rows int64
	if err := rs.db.Model(&models.RuntimeSetting{}).Count(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows != 0 || len(audit.entries) != 0 {
		t.Fatalf("failed batch left rows (%d) or audits (%d)", rows, len(audit.entries))
	}
}

func TestRuntimeSettings_BatchValidatesEveryMutationBeforeWriting(t *testing.T) {
	rs, _ := secretSettingsFixture(t)
	for _, tc := range []struct {
		name   string
		values map[string]string
		reset  []string
	}{
		{"unknown value", map[string]string{"api_key": "valid", "unknown": "invalid"}, nil},
		{"unknown reset", map[string]string{"api_key": "valid"}, []string{"unknown"}},
		{"set and reset", map[string]string{"api_key": "valid"}, []string{"api_key"}},
		{"duplicate reset", nil, []string{"api_key", "api_key"}},
		{"empty", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := rs.Apply(tc.values, tc.reset, "", ""); err == nil {
				t.Fatal("want invalid batch rejected")
			}
			var rows int64
			rs.db.Model(&models.RuntimeSetting{}).Count(&rows)
			if rows != 0 || rs.snapshot([]string{"api_key"})["api_key"] != "boot-secret-value" {
				t.Fatal("invalid batch changed database or cache")
			}
		})
	}
}

func TestRuntimeSettings_BatchSaveResetAndSnapshotsStayCoherent(t *testing.T) {
	rs, _ := secretSettingsFixture(t)
	if err := rs.Apply(map[string]string{"api_key": "same", "model": "same"}, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			value := fmt.Sprintf("revision-%d", i)
			if err := rs.Apply(map[string]string{"api_key": value, "model": value}, nil, "", ""); err != nil {
				t.Errorf("apply: %v", err)
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				values := rs.snapshot([]string{"api_key", "model"})
				if values["api_key"] != values["model"] {
					t.Errorf("mixed configuration: %v", values)
				}
			}
		}()
	}
	wg.Wait()
	reloaded := NewRuntimeSettings(rs.db, &stubLogger{}, rs.specs, rs.defaults)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if got, want := reloaded.snapshot([]string{"api_key", "model"}), rs.snapshot([]string{"api_key", "model"}); got["api_key"] != want["api_key"] || got["model"] != want["model"] {
		t.Fatal("database and live cache diverged")
	}
	if err := rs.Apply(nil, []string{"api_key", "model"}, "reset profile", ""); err != nil {
		t.Fatal(err)
	}
	values := rs.snapshot([]string{"api_key", "model"})
	if values["api_key"] != "boot-secret-value" || values["model"] != "boot-model" {
		t.Fatal("batch reset did not restore both defaults")
	}
}
