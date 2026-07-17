package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/mrql"
)

func TestExecuteMRQLFindLogsTimeoutDiagnostics(t *testing.T) {
	ctx := setupTestContext(t)
	query := `type = "resource" AND name ~ "timeout-log-needle" LIMIT 7`
	parsed, err := mrql.Parse(query)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	parsed.EntityType = mrql.EntityResource

	deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	db, err := mrql.Translate(parsed, ctx.db.WithContext(deadlineCtx))
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}

	var resources []models.Resource
	err = ctx.executeMRQLFind(db, &resources, parsed, "test resource select")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}

	var entry models.LogEntry
	if err := ctx.db.Where("entity_type = ?", "mrql").Last(&entry).Error; err != nil {
		t.Fatalf("find timeout log: %v", err)
	}
	if entry.Level != models.LogLevelWarning {
		t.Fatalf("level = %q, want warning", entry.Level)
	}
	if !strings.Contains(entry.Message, "test resource select") {
		t.Fatalf("message %q does not identify execution phase", entry.Message)
	}

	var details map[string]any
	if err := json.Unmarshal([]byte(entry.Details), &details); err != nil {
		t.Fatalf("decode details: %v", err)
	}
	if details["mrql"] != query {
		t.Fatalf("mrql detail = %q, want %q", details["mrql"], query)
	}
	sql, _ := details["sql"].(string)
	if !strings.Contains(sql, "timeout-log-needle") || !strings.Contains(strings.ToUpper(sql), "LIMIT 7") {
		t.Fatalf("sql detail missing query value/limit: %q", sql)
	}
	for _, key := range []string{"phase", "entityType", "database", "configuredTimeout", "timeoutMs", "elapsedMs", "deadline", "error"} {
		if _, ok := details[key]; !ok {
			t.Errorf("details missing %q: %#v", key, details)
		}
	}
}

func TestExecuteMRQLFindDoesNotLogSuccessfulQuery(t *testing.T) {
	ctx := setupTestContext(t)
	parsed, err := mrql.Parse(`type = "resource" LIMIT 1`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	parsed.EntityType = mrql.EntityResource
	db, err := mrql.Translate(parsed, ctx.db.WithContext(context.Background()))
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}

	var resources []models.Resource
	if err := ctx.executeMRQLFind(db, &resources, parsed, "test success"); err != nil {
		t.Fatalf("executeMRQLFind: %v", err)
	}

	var count int64
	if err := ctx.db.Model(&models.LogEntry{}).Where("entity_type = ?", "mrql").Count(&count).Error; err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if count != 0 {
		t.Fatalf("got %d MRQL timeout logs for successful query", count)
	}
}
