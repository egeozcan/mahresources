package block_types

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// hexColorRegex matches valid hex colors in #rgb or #rrggbb format.
var hexColorRegex = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// SourceConfig defines the source configuration for a calendar.
type SourceConfig struct {
	Type       string `json:"type"`       // "url" or "resource"
	URL        string `json:"url"`        // Required when Type is "url"
	ResourceID *uint  `json:"resourceId"` // Required when Type is "resource"
}

// CalendarSource represents a single calendar source configuration.
type CalendarSource struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Color  string       `json:"color"`
	Source SourceConfig `json:"source"`
}

// calendarContent represents the content schema for calendar blocks.
type calendarContent struct {
	Calendars []CalendarSource `json:"calendars"`
}

// calendarState represents the state schema for calendar blocks.
type calendarState struct {
	View        string `json:"view"`        // "month", "week", or "agenda"
	CurrentDate string `json:"currentDate"` // ISO date string
}

// CalendarBlockType implements BlockType for calendar content.
type CalendarBlockType struct{}

func (c CalendarBlockType) Type() string {
	return "calendar"
}

func (c CalendarBlockType) ValidateContent(content json.RawMessage) error {
	var cal calendarContent
	if err := json.Unmarshal(content, &cal); err != nil {
		return err
	}

	for i, source := range cal.Calendars {
		if source.ID == "" {
			return fmt.Errorf("calendar at index %d must have an id", i)
		}

		// Validate color if provided
		if source.Color != "" && !hexColorRegex.MatchString(source.Color) {
			return fmt.Errorf("calendar '%s': color must be a valid hex color (#rgb or #rrggbb)", source.ID)
		}

		// Validate source type
		if source.Source.Type != "url" && source.Source.Type != "resource" {
			return fmt.Errorf("calendar '%s': source type must be 'url' or 'resource'", source.ID)
		}

		// Validate source fields based on type
		if source.Source.Type == "url" && source.Source.URL == "" {
			return fmt.Errorf("calendar '%s': url source requires url field", source.ID)
		}
		if source.Source.Type == "resource" && source.Source.ResourceID == nil {
			return fmt.Errorf("calendar '%s': resource source requires resourceId field", source.ID)
		}
	}

	return nil
}

func (c CalendarBlockType) ValidateState(state json.RawMessage) error {
	var s calendarState
	if err := json.Unmarshal(state, &s); err != nil {
		return err
	}

	if s.View != "" && s.View != "month" && s.View != "week" && s.View != "agenda" {
		return errors.New("view must be 'month', 'week', or 'agenda'")
	}

	return nil
}

func (c CalendarBlockType) DefaultContent() json.RawMessage {
	return json.RawMessage(`{"calendars": []}`)
}

func (c CalendarBlockType) DefaultState() json.RawMessage {
	return json.RawMessage(`{"view": "month"}`)
}

func init() {
	RegisterBlockType(CalendarBlockType{})
}
