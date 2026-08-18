package server

import (
	"net/http"
	"testing"
)

// auth.PluginActionAccessFor refuses a caller that cannot write, because this is
// the capability withAuthorization demands of an action run. It cannot ask
// requiredCapability itself: the predicate is read from server/api_handlers,
// which this package imports, so the classification is the one part of the rule
// that exists twice.
//
// Pinned here rather than trusted. Reclassifying this endpoint downwards
// (isReadViaPost) would leave the offer hiding actions a guest could run;
// upwards (isEditorPath) would put the original defect back, with plain users
// offered buttons that answer 403. Either way PluginActionAccessFor is what has
// to move with it.
func TestActionRunRequiresTheWriteCapability(t *testing.T) {
	if got := requiredCapability(http.MethodPost, "/v1/jobs/action/run"); got != capWrite {
		t.Fatalf("requiredCapability(POST /v1/jobs/action/run) = %v, want capWrite; auth.PluginActionAccessFor mirrors this and must follow", got)
	}
}
