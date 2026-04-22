package application_context

import (
	"testing"
	"time"
)

// TestMRQLQueryTimeout_RuntimeOverride confirms the query timeout is read
// through appContext.Settings() per call, not captured at startup.
func TestMRQLQueryTimeout_RuntimeOverride(t *testing.T) {
	ctx := &MahresourcesContext{Config: &MahresourcesConfig{}}
	rs := NewRuntimeSettings(newTestDB(t), &stubLogger{}, buildSpecs(), defaults())
	if err := rs.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	ctx.SetSettings(rs)
	if got := ctx.mrqlQueryTimeout(); got != 10*time.Second {
		t.Fatalf("default: want 10s, got %v", got)
	}
	if err := rs.Set(KeyMRQLQueryTimeout, "2s", "", ""); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got := ctx.mrqlQueryTimeout(); got != 2*time.Second {
		t.Fatalf("override: want 2s, got %v", got)
	}
}
