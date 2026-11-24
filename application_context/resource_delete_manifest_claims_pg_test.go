//go:build postgres

package application_context

import "testing"

func TestManifestHookRunsWhenFileStepNeverSucceedsPG(t *testing.T) {
	testManifestHookRunsWhenFileStepNeverSucceeds(t, newPostgresPluginContext(t, map[string]string{"hooktest": manifestClaimPlugin()}))
}

func TestManifestClaimIsRenewedPerRowPG(t *testing.T) {
	testManifestClaimIsRenewedPerRow(t, newPostgresPluginContext(t, map[string]string{"hooktest": manifestClaimPlugin()}))
}

func TestManifestFiredHookDoesNotReplayAfterStallPG(t *testing.T) {
	testManifestFiredHookDoesNotReplayAfterStall(t, newPostgresPluginContext(t, map[string]string{"hooktest": manifestClaimPlugin()}))
}

func TestManifestExpiredDrainDoesNotMarkSuccessorsFilesDonePG(t *testing.T) {
	testManifestExpiredDrainDoesNotMarkSuccessorsFilesDone(t, newPostgresPluginContext(t, map[string]string{"hooktest": manifestClaimPlugin()}))
}

func TestManifestLostClaimSkipsHookPG(t *testing.T) {
	testManifestLostClaimSkipsHook(t, newPostgresPluginContext(t, map[string]string{"hooktest": manifestClaimPlugin()}))
}

func TestManifestPeriodicReplayDrainsWithoutRestartPG(t *testing.T) {
	testManifestPeriodicReplayDrainsWithoutRestart(t, newPostgresPluginContext(t, map[string]string{"hooktest": manifestClaimPlugin()}))
}
