package block_types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalendar_Type(t *testing.T) {
	bt := CalendarBlockType{}
	assert.Equal(t, "calendar", bt.Type())
}

func TestCalendar_ValidateContent_EmptyCalendars(t *testing.T) {
	bt := CalendarBlockType{}
	content := json.RawMessage(`{"calendars": []}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestCalendar_ValidateContent_URLCalendar(t *testing.T) {
	bt := CalendarBlockType{}
	content := json.RawMessage(`{
		"calendars": [{
			"id": "cal1",
			"name": "Work Calendar",
			"color": "#ff0000",
			"source": {
				"type": "url",
				"url": "https://example.com/calendar.ics"
			}
		}]
	}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestCalendar_ValidateContent_URLCalendar_ShortHex(t *testing.T) {
	bt := CalendarBlockType{}
	content := json.RawMessage(`{
		"calendars": [{
			"id": "cal1",
			"name": "Work Calendar",
			"color": "#f00",
			"source": {
				"type": "url",
				"url": "https://example.com/calendar.ics"
			}
		}]
	}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestCalendar_ValidateContent_ResourceCalendar(t *testing.T) {
	bt := CalendarBlockType{}
	content := json.RawMessage(`{
		"calendars": [{
			"id": "cal2",
			"name": "Personal Calendar",
			"color": "#00ff00",
			"source": {
				"type": "resource",
				"resourceId": 123
			}
		}]
	}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestCalendar_ValidateContent_MultipleCalendars(t *testing.T) {
	bt := CalendarBlockType{}
	content := json.RawMessage(`{
		"calendars": [
			{
				"id": "cal1",
				"name": "Work",
				"color": "#ff0000",
				"source": {"type": "url", "url": "https://example.com/work.ics"}
			},
			{
				"id": "cal2",
				"name": "Personal",
				"color": "#00ff00",
				"source": {"type": "resource", "resourceId": 456}
			}
		]
	}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestCalendar_ValidateContent_InvalidColorFormat(t *testing.T) {
	bt := CalendarBlockType{}

	testCases := []struct {
		name  string
		color string
	}{
		{"no hash", "ff0000"},
		{"invalid chars", "#gggggg"},
		{"too short", "#ff"},
		{"too long", "#ff00001"},
		{"wrong length", "#ff000"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content := json.RawMessage(`{
				"calendars": [{
					"id": "cal1",
					"name": "Test",
					"color": "` + tc.color + `",
					"source": {"type": "url", "url": "https://example.com/cal.ics"}
				}]
			}`)
			err := bt.ValidateContent(content)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "color must be a valid hex color")
		})
	}
}

func TestCalendar_ValidateContent_InvalidSourceType(t *testing.T) {
	bt := CalendarBlockType{}
	content := json.RawMessage(`{
		"calendars": [{
			"id": "cal1",
			"name": "Test",
			"color": "#ff0000",
			"source": {
				"type": "invalid"
			}
		}]
	}`)
	err := bt.ValidateContent(content)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "source type must be 'url' or 'resource'")
}

func TestCalendar_ValidateContent_URLSourceMissingURL(t *testing.T) {
	bt := CalendarBlockType{}
	content := json.RawMessage(`{
		"calendars": [{
			"id": "cal1",
			"name": "Test",
			"color": "#ff0000",
			"source": {
				"type": "url"
			}
		}]
	}`)
	err := bt.ValidateContent(content)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "url source requires url field")
}

func TestCalendar_ValidateContent_ResourceSourceMissingResourceID(t *testing.T) {
	bt := CalendarBlockType{}
	content := json.RawMessage(`{
		"calendars": [{
			"id": "cal1",
			"name": "Test",
			"color": "#ff0000",
			"source": {
				"type": "resource"
			}
		}]
	}`)
	err := bt.ValidateContent(content)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resource source requires resourceId field")
}

func TestCalendar_ValidateContent_MissingID(t *testing.T) {
	bt := CalendarBlockType{}
	content := json.RawMessage(`{
		"calendars": [{
			"name": "Test",
			"color": "#ff0000",
			"source": {"type": "url", "url": "https://example.com/cal.ics"}
		}]
	}`)
	err := bt.ValidateContent(content)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must have an id")
}

func TestCalendar_ValidateContent_InvalidJSON(t *testing.T) {
	bt := CalendarBlockType{}
	content := json.RawMessage(`{invalid}`)
	err := bt.ValidateContent(content)
	assert.Error(t, err)
}

func TestCalendar_ValidateState_Empty(t *testing.T) {
	bt := CalendarBlockType{}
	state := json.RawMessage(`{}`)
	err := bt.ValidateState(state)
	assert.NoError(t, err)
}

func TestCalendar_ValidateState_MonthView(t *testing.T) {
	bt := CalendarBlockType{}
	state := json.RawMessage(`{"view": "month", "currentDate": "2024-01-15"}`)
	err := bt.ValidateState(state)
	assert.NoError(t, err)
}

func TestCalendar_ValidateState_AgendaView(t *testing.T) {
	bt := CalendarBlockType{}
	state := json.RawMessage(`{"view": "agenda"}`)
	err := bt.ValidateState(state)
	assert.NoError(t, err)
}

func TestCalendar_ValidateState_WeekView(t *testing.T) {
	bt := CalendarBlockType{}
	state := json.RawMessage(`{"view": "week"}`)
	err := bt.ValidateState(state)
	assert.NoError(t, err)
}

func TestCalendar_ValidateState_InvalidView(t *testing.T) {
	bt := CalendarBlockType{}
	state := json.RawMessage(`{"view": "invalid"}`)
	err := bt.ValidateState(state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "view must be 'month', 'week', or 'agenda'")
}

func TestCalendar_ValidateState_InvalidJSON(t *testing.T) {
	bt := CalendarBlockType{}
	state := json.RawMessage(`{invalid}`)
	err := bt.ValidateState(state)
	assert.Error(t, err)
}

func TestCalendar_DefaultContent(t *testing.T) {
	bt := CalendarBlockType{}
	content := bt.DefaultContent()

	// Should be valid JSON
	var c map[string]interface{}
	err := json.Unmarshal(content, &c)
	assert.NoError(t, err)

	// Should have calendars key
	_, ok := c["calendars"]
	assert.True(t, ok)

	// Default content should be valid
	err = bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestCalendar_DefaultState(t *testing.T) {
	bt := CalendarBlockType{}
	state := bt.DefaultState()

	// Should be valid JSON
	var s map[string]interface{}
	err := json.Unmarshal(state, &s)
	assert.NoError(t, err)

	// Default state should be valid
	err = bt.ValidateState(state)
	assert.NoError(t, err)
}

func TestCalendar_Registered(t *testing.T) {
	bt := GetBlockType("calendar")
	assert.NotNil(t, bt)
	assert.Equal(t, "calendar", bt.Type())
}
