package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/contracts"
	"mahresources/models"
)

// TestSharedNote_CalendarBlockDoesNotPublishSourceURL verifies that the
// anonymous share page never carries a calendar block's feed source
// (private ICS URLs are bearer credentials; resource IDs are internal).
func TestSharedNote_CalendarBlockDoesNotPublishSourceURL(t *testing.T) {
	tc := setupShareEnabledTestEnv(t)
	shareRouter := setupShareServer(t, tc)

	note := tc.CreateDummyNote("calendar-secrets-note")
	token := shareNote(t, tc, note.ID)

	content, _ := json.Marshal(map[string]interface{}{
		"calendars": []map[string]interface{}{
			{
				"id":    "cal-work",
				"name":  "Work Cal",
				"color": "#3b82f6",
				"source": map[string]interface{}{
					"type": "url",
					"url":  "https://feeds.example.invalid/private-SECRETPATH/basic.ics?token=QSECRET",
				},
			},
			{
				"id":   "cal-res",
				"name": "Res Cal",
				"source": map[string]interface{}{
					"type":       "resource",
					"resourceId": 987654,
				},
			},
		},
	})
	block := &models.NoteBlock{
		NoteID:   note.ID,
		Type:     "calendar",
		Position: "a",
		Content:  content,
		State:    []byte("{}"),
	}
	require.NoError(t, tc.DB.Create(block).Error)

	req, _ := http.NewRequest(http.MethodGet, "/s/"+token, nil)
	rr := httptest.NewRecorder()
	shareRouter.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "share page must render, body=%s", rr.Body.String())

	body := rr.Body.String()
	assert.Contains(t, body, "Work Cal", "calendar name must still render")
	assert.Contains(t, body, "Res Cal", "calendar name must still render")
	assert.Contains(t, body, "cal-work", "calendar id must still render")

	for _, secret := range []string{"private-SECRETPATH", "QSECRET", "feeds.example.invalid", "resourceId", "987654"} {
		assert.NotContains(t, body, secret, "share page must not publish calendar source detail %q", secret)
	}
}

// TestSharedCalendarEvents_FetchErrorDoesNotEchoURL verifies that a failed
// feed fetch on the anonymous proxy endpoint does not echo the private URL
// (the *url.Error text carries the full request URL).
func TestSharedCalendarEvents_FetchErrorDoesNotEchoURL(t *testing.T) {
	tc := setupShareEnabledTestEnv(t)
	shareRouter := setupShareServer(t, tc)

	// A server that is closed immediately, so the dial is refused.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	feed := dead.URL + "/private-SECRETPATH/basic.ics?token=QSECRET"
	dead.Close()

	note := tc.CreateDummyNote("calendar-fetch-error-note")
	token := shareNote(t, tc, note.ID)

	content, _ := json.Marshal(map[string]interface{}{
		"calendars": []map[string]interface{}{
			{
				"id":     "cal-work",
				"name":   "Work Cal",
				"color":  "#3b82f6",
				"source": map[string]interface{}{"type": "url", "url": feed},
			},
		},
	})
	block := &models.NoteBlock{
		NoteID:   note.ID,
		Type:     "calendar",
		Position: "a",
		Content:  content,
		State:    []byte("{}"),
	}
	require.NoError(t, tc.DB.Create(block).Error)

	url := fmt.Sprintf("/s/%s/block/%d/calendar/events?start=2026-01-01&end=2026-01-31", token, block.ID)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	shareRouter.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "body=%s", rr.Body.String())

	var resp contracts.CalendarEventsResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp), "body=%s", rr.Body.String())
	require.Len(t, resp.Errors, 1, "expected one per-calendar error, body=%s", rr.Body.String())
	assert.Equal(t, "cal-work", resp.Errors[0].CalendarID)

	msg := resp.Errors[0].Error
	host := strings.TrimPrefix(dead.URL, "http://")
	port := host[strings.LastIndex(host, ":")+1:]
	for _, leak := range []string{"private-SECRETPATH", "QSECRET", "127.0.0.1", port} {
		assert.NotContains(t, msg, leak, "fetch error must not echo the feed URL, got %q", msg)
	}
}

// TestSharedCalendarEvents_ResourceErrorIsGeneric verifies that a resource
// source pointing at a missing resource does not reveal whether the ID exists.
func TestSharedCalendarEvents_ResourceErrorIsGeneric(t *testing.T) {
	tc := setupShareEnabledTestEnv(t)
	shareRouter := setupShareServer(t, tc)

	note := tc.CreateDummyNote("calendar-resource-error-note")
	token := shareNote(t, tc, note.ID)

	content, _ := json.Marshal(map[string]interface{}{
		"calendars": []map[string]interface{}{
			{
				"id":     "cal-res",
				"name":   "Res Cal",
				"color":  "#10b981",
				"source": map[string]interface{}{"type": "resource", "resourceId": 987654},
			},
		},
	})
	block := &models.NoteBlock{
		NoteID:   note.ID,
		Type:     "calendar",
		Position: "a",
		Content:  content,
		State:    []byte("{}"),
	}
	require.NoError(t, tc.DB.Create(block).Error)

	url := fmt.Sprintf("/s/%s/block/%d/calendar/events?start=2026-01-01&end=2026-01-31", token, block.ID)
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	shareRouter.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "body=%s", rr.Body.String())

	var resp contracts.CalendarEventsResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp), "body=%s", rr.Body.String())
	require.Len(t, resp.Errors, 1, "body=%s", rr.Body.String())

	msg := resp.Errors[0].Error
	assert.NotContains(t, msg, "record not found", "resource error must be generic, got %q", msg)
	assert.NotContains(t, msg, "987654", "resource error must not echo the resource ID, got %q", msg)
}
