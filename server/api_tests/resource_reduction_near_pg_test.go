//go:build postgres

package api_tests

import (
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

// The aHash guard's SQL runs on Postgres too. This harness wires no runtime
// settings of its own, and the guard's threshold is one, so they are wired here
// as the server's boot wires them.
func TestAPairTheAHashGuardRejectsIsNotClusteredPG(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	if err := tc.DB.AutoMigrate(&models.RuntimeSetting{}); err != nil {
		t.Fatalf("migrate runtime settings: %v", err)
	}
	settings := application_context.NewRuntimeSettings(tc.DB, application_context.NewStdlibSettingsLogger(),
		application_context.BuildSpecsExported(), application_context.BuildDefaultsFromConfig(tc.AppCtx.Config))
	if err := settings.Load(); err != nil {
		t.Fatalf("load runtime settings: %v", err)
	}
	tc.AppCtx.SetSettings(settings)
	assertTheAHashGuardKeepsAPairOutOfAReduction(t, tc)
}
