//go:build postgres

package api_tests

import "testing"

func TestTemplateMRQLCheckMatchesRenderingPostgres(t *testing.T) {
	runTemplateMRQLCheckMatchesRendering(t, SetupPostgresTestEnv(t))
}
