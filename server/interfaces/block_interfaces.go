package interfaces

import (
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
	"mahresources/models"
	"mahresources/models/query_models"
)

type BlockReader interface {
	GetBlock(id uint) (*models.NoteBlock, error)
	GetBlocksForNote(noteID uint) ([]models.NoteBlock, error)
}

type BlockWriter interface {
	CreateBlock(editor *query_models.NoteBlockEditor) (*models.NoteBlock, error)
	UpdateBlockContent(blockID uint, content json.RawMessage) (*models.NoteBlock, error)
	ReorderBlocks(noteID uint, positions map[uint]string) error
}

type BlockStateWriter interface {
	UpdateBlockState(blockID uint, state json.RawMessage) (*models.NoteBlock, error)
}

type BlockDeleter interface {
	DeleteBlock(blockID uint) error
}

type BlockRebalancer interface {
	RebalanceBlockPositions(noteID uint) error
}

// TableBlockQueryRunner combines block reading and query execution for table blocks.
type TableBlockQueryRunner interface {
	GetBlock(id uint) (*models.NoteBlock, error)
	RunReadOnlyQuery(queryId uint, params map[string]any) (*sqlx.Rows, error)
}

// CalendarBlockEventFetcher combines block reading and resource access for calendar blocks.
type CalendarBlockEventFetcher interface {
	GetBlock(id uint) (*models.NoteBlock, error)
	GetResource(id uint) (*models.Resource, error)
	GetCalendarEvents(blockID uint, start, end time.Time) (*CalendarEventsResponse, error)
}

// CalendarEventsResponse is the response for the calendar events API.
type CalendarEventsResponse struct {
	Events    []CalendarEvent `json:"events"`
	Calendars []CalendarInfo  `json:"calendars"`
	Errors    []CalendarError `json:"errors,omitempty"`
	CachedAt  string          `json:"cachedAt"`
}

// CalendarError represents an error that occurred while fetching a specific calendar.
type CalendarError struct {
	CalendarID string `json:"calendarId"`
	Error      string `json:"error"`
}

// CalendarEvent represents a single calendar event.
type CalendarEvent struct {
	ID          string    `json:"id"`
	CalendarID  string    `json:"calendarId"`
	Title       string    `json:"title"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AllDay      bool      `json:"allDay"`
	Location    string    `json:"location,omitempty"`
	Description string    `json:"description,omitempty"`
}

// CalendarInfo represents calendar metadata.
type CalendarInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}
