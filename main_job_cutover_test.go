package main

import (
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/jobs"
)

func TestJobCenterCutoverRequiresRetirementAndCompleteKinds(t *testing.T) {
	var ctx application_context.MahresourcesContext
	service := jobs.NewService()
	ctx.SetJobService(service)
	ready := application_context.JobMigrationReadiness{Ready: true, WriterEpoch: 2, Phase: "complete"}
	if err := validateJobCenterCutover(service, ready); err != nil {
		t.Fatalf("complete Kind inventory and retirement were refused: %v", err)
	}
	ready.Ready = false
	if err := validateJobCenterCutover(service, ready); err == nil || !strings.Contains(err.Error(), "migration") {
		t.Fatalf("incomplete migration was admitted: %v", err)
	}
	ready.Ready = true
	ready.WriterEpoch = 1
	if err := validateJobCenterCutover(service, ready); err == nil {
		t.Fatal("epoch 1 report was admitted despite ready flag")
	}
	if err := validateJobCenterCutover(jobs.NewService(), application_context.JobMigrationReadiness{Ready: true}); err == nil {
		t.Fatal("missing Kind inventory was admitted")
	}
}
