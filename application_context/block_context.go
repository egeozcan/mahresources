package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"time"

	"gorm.io/gorm"
	"mahresources/lib"
	"mahresources/models"
	"mahresources/models/block_types"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"mahresources/server/interfaces"
)

// Global ICS cache (shared across all calendar blocks)
var icsCache = NewICSCache(100, 30*time.Minute)

// CreateBlock creates a new block in a note
func (ctx *MahresourcesContext) CreateBlock(editor *query_models.NoteBlockEditor) (*models.NoteBlock, error) {
	// Validate block type
	bt := block_types.GetBlockType(editor.Type)
	if bt == nil {
		return nil, errors.New("unknown block type: " + editor.Type)
	}

	// Validate content or use default
	if len(editor.Content) == 0 {
		editor.Content = bt.DefaultContent()
	} else if err := bt.ValidateContent(editor.Content); err != nil {
		return nil, err
	}

	block := models.NoteBlock{
		NoteID:   editor.NoteID,
		Type:     editor.Type,
		Position: editor.Position,
		Content:  types.JSON(editor.Content),
		State:    types.JSON(bt.DefaultState()),
	}

	// Use transaction to ensure atomicity of block creation and description sync
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&block).Error; err != nil {
			return err
		}

		// Sync first text block to note description within the same transaction
		if editor.Type == "text" {
			if err := syncFirstTextBlockToDescriptionTx(tx, editor.NoteID); err != nil {
				log.Printf("Warning: failed to sync description for note %d: %v", editor.NoteID, err)
				// Don't fail the transaction for sync errors
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &block, nil
}

// GetBlock retrieves a single block by ID
func (ctx *MahresourcesContext) GetBlock(id uint) (*models.NoteBlock, error) {
	var block models.NoteBlock
	return &block, ctx.db.First(&block, id).Error
}

// GetBlocksForNote retrieves all blocks for a note, ordered by position
func (ctx *MahresourcesContext) GetBlocksForNote(noteID uint) (*[]models.NoteBlock, error) {
	var blocks []models.NoteBlock
	err := ctx.db.Where("note_id = ?", noteID).Order("position ASC").Find(&blocks).Error
	return &blocks, err
}

// UpdateBlockContent updates a block's content
func (ctx *MahresourcesContext) UpdateBlockContent(blockID uint, content json.RawMessage) (*models.NoteBlock, error) {
	var block models.NoteBlock
	if err := ctx.db.First(&block, blockID).Error; err != nil {
		return nil, err
	}

	// Validate content against block type
	bt := block_types.GetBlockType(block.Type)
	if bt == nil {
		return nil, errors.New("unknown block type: " + block.Type)
	}
	if err := bt.ValidateContent(content); err != nil {
		return nil, err
	}

	block.Content = types.JSON(content)

	// Use transaction to ensure atomicity of content update and description sync
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&block).Error; err != nil {
			return err
		}

		// Sync first text block to note description within the same transaction
		if block.Type == "text" {
			if err := syncFirstTextBlockToDescriptionTx(tx, block.NoteID); err != nil {
				log.Printf("Warning: failed to sync description for note %d: %v", block.NoteID, err)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &block, nil
}

// UpdateBlockState updates a block's state (for UI state like checked items)
func (ctx *MahresourcesContext) UpdateBlockState(blockID uint, state json.RawMessage) (*models.NoteBlock, error) {
	var block models.NoteBlock
	if err := ctx.db.First(&block, blockID).Error; err != nil {
		return nil, err
	}

	// Validate state against block type
	bt := block_types.GetBlockType(block.Type)
	if bt == nil {
		return nil, errors.New("unknown block type: " + block.Type)
	}
	if err := bt.ValidateState(state); err != nil {
		return nil, err
	}

	block.State = types.JSON(state)
	return &block, ctx.db.Save(&block).Error
}

// DeleteBlock removes a block
func (ctx *MahresourcesContext) DeleteBlock(blockID uint) error {
	var block models.NoteBlock
	if err := ctx.db.First(&block, blockID).Error; err != nil {
		return err
	}

	noteID := block.NoteID
	isText := block.Type == "text"

	// Use transaction to ensure atomicity of deletion and description sync
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&block).Error; err != nil {
			return err
		}

		// Sync first text block to note description within the same transaction
		if isText {
			if err := syncFirstTextBlockToDescriptionTx(tx, noteID); err != nil {
				log.Printf("Warning: failed to sync description for note %d: %v", noteID, err)
			}
		}
		return nil
	})
}

// ReorderBlocks updates positions for multiple blocks in a single transaction
func (ctx *MahresourcesContext) ReorderBlocks(noteID uint, positions map[uint]string) error {
	if len(positions) == 0 {
		return nil
	}

	// Collect block IDs for validation
	blockIDs := make([]uint, 0, len(positions))
	for blockID := range positions {
		blockIDs = append(blockIDs, blockID)
	}

	// Verify all blocks belong to the specified note
	var count int64
	if err := ctx.db.Model(&models.NoteBlock{}).Where("id IN ? AND note_id = ?", blockIDs, noteID).Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(positions) {
		return errors.New("one or more block IDs do not belong to the specified note")
	}

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		for blockID, position := range positions {
			if err := tx.Model(&models.NoteBlock{}).Where("id = ? AND note_id = ?", blockID, noteID).
				Update("position", position).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// syncFirstTextBlockToDescriptionTx syncs the first text block's content to the note's Description
// within an existing transaction. This ensures atomicity between block operations and description sync.
func syncFirstTextBlockToDescriptionTx(tx *gorm.DB, noteID uint) error {
	var blocks []models.NoteBlock
	if err := tx.Where("note_id = ? AND type = ?", noteID, "text").
		Order("position ASC").Limit(1).Find(&blocks).Error; err != nil {
		return err
	}

	if len(blocks) == 0 {
		return nil
	}

	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(blocks[0].Content, &content); err != nil {
		return err
	}

	return tx.Model(&models.Note{}).Where("id = ?", noteID).Update("description", content.Text).Error
}

// RebalanceBlockPositions normalizes block positions for a note to prevent position string growth.
// This reassigns positions using evenly distributed values (e.g., "d", "h", "l", "p", "t").
// Call this periodically or when positions become too long.
func (ctx *MahresourcesContext) RebalanceBlockPositions(noteID uint) error {
	var blocks []models.NoteBlock
	if err := ctx.db.Where("note_id = ?", noteID).Order("position ASC").Find(&blocks).Error; err != nil {
		return err
	}

	if len(blocks) == 0 {
		return nil
	}

	// Generate evenly distributed positions
	newPositions := lib.GenerateEvenPositions(len(blocks))

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		for i, block := range blocks {
			if err := tx.Model(&models.NoteBlock{}).Where("id = ?", block.ID).
				Update("position", newPositions[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// UpdateBlockStateFromRequest decodes a block state update from an HTTP request body
// and applies it using UpdateBlockState. This is used by the share server for
// allowing anonymous visitors to update todo checkboxes on shared notes.
func (ctx *MahresourcesContext) UpdateBlockStateFromRequest(blockId uint, r *http.Request) error {
	var stateUpdate json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&stateUpdate); err != nil {
		return err
	}

	_, err := ctx.UpdateBlockState(blockId, stateUpdate)
	return err
}

// GetCalendarEvents fetches and parses calendar events for a calendar block.
// It supports both URL-based and resource-based calendar sources with caching.
func (ctx *MahresourcesContext) GetCalendarEvents(blockID uint, start, end time.Time) (*interfaces.CalendarEventsResponse, error) {
	// Get the block
	block, err := ctx.GetBlock(blockID)
	if err != nil {
		return nil, err
	}

	// Verify block type
	if block.Type != "calendar" {
		return nil, errors.New("block is not a calendar type")
	}

	// Parse block content to get calendar sources
	var content struct {
		Calendars []block_types.CalendarSource `json:"calendars"`
	}
	if err := json.Unmarshal(block.Content, &content); err != nil {
		return nil, fmt.Errorf("failed to parse block content: %w", err)
	}

	var allEvents []interfaces.CalendarEvent
	var calendars []interfaces.CalendarInfo
	var cachedAt time.Time

	// Fetch events from each calendar source
	for _, cal := range content.Calendars {
		var icsContent []byte
		var fetchTime time.Time
		var fetchErr error

		switch cal.Source.Type {
		case "url":
			icsContent, fetchTime, fetchErr = ctx.fetchICSFromURL(cal.Source.URL)
		case "resource":
			if cal.Source.ResourceID == nil {
				log.Printf("Calendar %s: resource source missing resourceId", cal.ID)
				continue
			}
			icsContent, fetchTime, fetchErr = ctx.fetchICSFromResource(*cal.Source.ResourceID)
		default:
			log.Printf("Calendar %s: unknown source type %s", cal.ID, cal.Source.Type)
			continue
		}

		if fetchErr != nil {
			log.Printf("Calendar %s: failed to fetch ICS: %v", cal.ID, fetchErr)
			continue
		}

		// Track the most recent cache time
		if fetchTime.After(cachedAt) {
			cachedAt = fetchTime
		}

		// Parse events from ICS content
		events, parseErr := ParseICSEvents(icsContent, cal.ID, start, end)
		if parseErr != nil {
			log.Printf("Calendar %s: failed to parse ICS: %v", cal.ID, parseErr)
			continue
		}

		// Convert to interface type
		for _, evt := range events {
			allEvents = append(allEvents, interfaces.CalendarEvent{
				ID:          evt.ID,
				CalendarID:  evt.CalendarID,
				Title:       evt.Title,
				Start:       evt.Start,
				End:         evt.End,
				AllDay:      evt.AllDay,
				Location:    evt.Location,
				Description: evt.Description,
			})
		}

		// Add calendar info
		calendars = append(calendars, interfaces.CalendarInfo{
			ID:    cal.ID,
			Name:  cal.Name,
			Color: cal.Color,
		})
	}

	// Sort events by start time
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].Start.Before(allEvents[j].Start)
	})

	// Use current time if no calendars were fetched
	if cachedAt.IsZero() {
		cachedAt = time.Now()
	}

	return &interfaces.CalendarEventsResponse{
		Events:    allEvents,
		Calendars: calendars,
		CachedAt:  cachedAt.UTC().Format(time.RFC3339),
	}, nil
}

// fetchICSFromURL fetches ICS content from a URL with caching support.
// Returns the content, the time it was fetched, and any error.
func (ctx *MahresourcesContext) fetchICSFromURL(url string) ([]byte, time.Time, error) {
	// Check cache first
	if entry, ok := icsCache.Get(url); ok {
		if entry.IsFresh(30 * time.Minute) {
			return entry.Content, entry.FetchedAt, nil
		}
		// Entry exists but is stale - try conditional fetch
		content, fetchTime, err := ctx.fetchAndCacheICS(url, entry)
		if err != nil {
			// If conditional fetch fails, return stale data
			log.Printf("Conditional fetch failed for %s, using cached data: %v", url, err)
			return entry.Content, entry.FetchedAt, nil
		}
		return content, fetchTime, nil
	}

	// No cache entry - fetch fresh
	return ctx.fetchAndCacheICS(url, nil)
}

// fetchAndCacheICS performs the actual HTTP fetch with optional conditional headers.
func (ctx *MahresourcesContext) fetchAndCacheICS(url string, existingEntry *ICSCacheEntry) ([]byte, time.Time, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to create request: %w", err)
	}

	// Add conditional headers if we have a previous entry
	if existingEntry != nil {
		if existingEntry.ETag != "" {
			req.Header.Set("If-None-Match", existingEntry.ETag)
		}
		if existingEntry.LastModified != "" {
			req.Header.Set("If-Modified-Since", existingEntry.LastModified)
		}
	}

	// Use configured timeouts
	client := &http.Client{
		Timeout: ctx.Config.RemoteResourceConnectTimeout,
	}
	if client.Timeout == 0 {
		client.Timeout = 30 * time.Second
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	// Handle 304 Not Modified
	if resp.StatusCode == http.StatusNotModified && existingEntry != nil {
		// Refresh the cache entry timestamp
		icsCache.Set(url, existingEntry.Content, existingEntry.ETag, existingEntry.LastModified)
		return existingEntry.Content, time.Now(), nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to read response: %w", err)
	}

	// Cache the result
	etag := resp.Header.Get("ETag")
	lastModified := resp.Header.Get("Last-Modified")
	icsCache.Set(url, content, etag, lastModified)

	return content, time.Now(), nil
}

// fetchICSFromResource reads ICS content from a stored resource file.
func (ctx *MahresourcesContext) fetchICSFromResource(resourceID uint) ([]byte, time.Time, error) {
	resource, err := ctx.GetResource(resourceID)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to get resource: %w", err)
	}

	// Determine which filesystem to use
	var fs = ctx.fs
	if resource.StorageLocation != nil && *resource.StorageLocation != "" {
		if altFs, ok := ctx.altFileSystems[*resource.StorageLocation]; ok {
			fs = altFs
		}
	}

	// Open and read the file
	f, err := fs.Open(resource.GetCleanLocation())
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to open resource file: %w", err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to read resource file: %w", err)
	}

	// Use the resource's updated time as the fetch time
	return content, resource.UpdatedAt, nil
}
