package application_context

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	neturl "net/url"
	"sort"
	"time"

	"gorm.io/gorm"
	"mahresources/lib"
	"mahresources/models"
	"mahresources/models/block_types"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"mahresources/plugin_system"
	"mahresources/server/interfaces"
)

// maxICSFileSize is the maximum size for ICS calendar files (10MB).
// This prevents memory exhaustion from malicious or corrupted calendar URLs.
const maxICSFileSize = 10 * 1024 * 1024

// CreateBlock creates a new block in a note
func (ctx *MahresourcesContext) CreateBlock(editor *query_models.NoteBlockEditor) (*models.NoteBlock, error) {
	// Validate note exists
	var noteCheck models.Note
	if err := ctx.db.Select("id").First(&noteCheck, editor.NoteID).Error; err != nil {
		return nil, fmt.Errorf("note %d not found: %w", editor.NoteID, err)
	}

	// Validate block type
	bt := block_types.GetBlockType(editor.Type)
	if bt == nil {
		return nil, errors.New("unknown block type: " + editor.Type)
	}

	// Enforce plugin block type filters
	if pbt, ok := bt.(*plugin_system.PluginBlockType); ok {
		if len(pbt.Filters.NoteTypeIDs) > 0 {
			note, err := ctx.GetNote(editor.NoteID)
			if err != nil {
				return nil, fmt.Errorf("cannot verify block type filters: %w", err)
			}
			found := false
			for _, id := range pbt.Filters.NoteTypeIDs {
				if note.NoteTypeId != nil && *note.NoteTypeId == id {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("block type %q is not available for this note type", editor.Type)
			}
		}
	}

	// Validate content or use default
	if len(editor.Content) == 0 {
		editor.Content = bt.DefaultContent()
	} else if isNullJSON(editor.Content) {
		return nil, errors.New("block content cannot be null")
	} else if err := bt.ValidateContent(editor.Content); err != nil {
		return nil, err
	}

	// Auto-assign position after all existing blocks if none provided
	if editor.Position == "" {
		var lastPos string
		ctx.db.Model(&models.NoteBlock{}).
			Where("note_id = ?", editor.NoteID).
			Order("position DESC").
			Limit(1).
			Pluck("position", &lastPos)
		if lastPos == "" {
			editor.Position = "n" // middle of alphabet for first block
		} else {
			editor.Position = lib.PositionBetween(lastPos, "")
		}
	}

	// Seed the first text block from the note's existing description so that
	// adding an empty text block which becomes the FIRST text block migrates the
	// description into the block instead of wiping it. Runs after the position is
	// finalized, because "first" is decided by position: an empty block inserted
	// *before* the current first text block would otherwise clear the description
	// via the description<->first-text-block sync.
	if editor.Type == "text" {
		if err := seedFirstTextBlockFromDescription(ctx.db, editor); err != nil {
			return nil, err
		}
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

	// Sync mention relations after block creation
	if editor.Type == "text" {
		var note models.Note
		if err := ctx.db.First(&note, editor.NoteID).Error; err == nil {
			ctx.syncMentionsForNote(&note)
		}
	}

	ctx.maybeRebalanceBlockPositions(editor.NoteID)

	return &block, nil
}

// GetBlock retrieves a single block by ID
func (ctx *MahresourcesContext) GetBlock(id uint) (*models.NoteBlock, error) {
	var block models.NoteBlock
	if err := ctx.db.First(&block, id).Error; err != nil {
		return &block, err
	}
	// RBAC: a block is only visible if its owning note is in scope.
	if !ctx.NoteVisible(block.NoteID) {
		return nil, gorm.ErrRecordNotFound
	}
	return &block, nil
}

// GetBlocksForNote retrieves all blocks for a note, ordered by position
func (ctx *MahresourcesContext) GetBlocksForNote(noteID uint) ([]models.NoteBlock, error) {
	// RBAC: blocks are confined to notes the principal can see.
	if !ctx.NoteVisible(noteID) {
		return []models.NoteBlock{}, nil
	}
	var blocks []models.NoteBlock
	err := ctx.db.Where("note_id = ?", noteID).Order("position ASC").Find(&blocks).Error
	return blocks, err
}

// UpdateBlockContent updates a block's content
func (ctx *MahresourcesContext) UpdateBlockContent(blockID uint, content json.RawMessage) (*models.NoteBlock, error) {
	var block models.NoteBlock
	if err := ctx.db.First(&block, blockID).Error; err != nil {
		return nil, err
	}
	// RBAC: blocks are only mutable when their owning note is in scope.
	if !ctx.NoteVisible(block.NoteID) {
		return nil, gorm.ErrRecordNotFound
	}

	// Reject empty or literal-null content before it can blank out a block.
	if len(bytes.TrimSpace(content)) == 0 || isNullJSON(content) {
		return nil, errors.New("block content cannot be null")
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

	// Sync mention relations after block content changes
	if block.Type == "text" {
		var note models.Note
		if err := ctx.db.First(&note, block.NoteID).Error; err == nil {
			ctx.syncMentionsForNote(&note)
		}
	}

	return &block, nil
}

// UpdateBlockState updates a block's state (for UI state like checked items)
func (ctx *MahresourcesContext) UpdateBlockState(blockID uint, state json.RawMessage) (*models.NoteBlock, error) {
	var block models.NoteBlock
	if err := ctx.db.First(&block, blockID).Error; err != nil {
		return nil, err
	}
	if !ctx.NoteVisible(block.NoteID) {
		return nil, gorm.ErrRecordNotFound
	}

	// Reject empty or literal-null state. This centralizes the guard that the
	// JSON API handler already applies so the anonymous share-server path (which
	// also reaches UpdateBlockState) cannot wipe saved state with `null`.
	if len(bytes.TrimSpace(state)) == 0 || isNullJSON(state) {
		return nil, errors.New("state field is required")
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
	if !ctx.NoteVisible(block.NoteID) {
		return gorm.ErrRecordNotFound
	}

	noteID := block.NoteID
	isText := block.Type == "text"

	// Use transaction to ensure atomicity of deletion and description sync
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
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

	if err != nil {
		return err
	}

	// Sync mention relations after block deletion
	if isText {
		var note models.Note
		if e := ctx.db.First(&note, noteID).Error; e == nil {
			ctx.syncMentionsForNote(&note)
		}
	}

	return nil
}

// ReorderBlocks updates positions for multiple blocks in a single transaction
func (ctx *MahresourcesContext) ReorderBlocks(noteID uint, positions map[uint]string) error {
	if len(positions) == 0 {
		return nil
	}
	// RBAC: only reorder blocks of an in-scope note.
	if !ctx.NoteVisible(noteID) {
		return gorm.ErrRecordNotFound
	}

	// Check for empty or duplicate position values. An empty position sorts before
	// every real position and can silently steal "first text block" status,
	// wiping the note description through the description sync.
	seen := make(map[string]bool, len(positions))
	for _, pos := range positions {
		if pos == "" {
			return errors.New("position must not be empty")
		}
		if seen[pos] {
			return errors.New("duplicate position values are not allowed")
		}
		seen[pos] = true
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

	// Reject positions that collide with blocks of the same note that are NOT part
	// of this (possibly partial) reorder. There is no unique DB index on
	// (note_id, position), so without this a partial map can place two blocks at
	// the same position, producing a non-deterministic order and an ambiguous
	// first text block.
	var otherPositions []string
	if err := ctx.db.Model(&models.NoteBlock{}).
		Where("note_id = ? AND id NOT IN ?", noteID, blockIDs).
		Pluck("position", &otherPositions).Error; err != nil {
		return err
	}
	for _, pos := range otherPositions {
		if seen[pos] {
			return errors.New("position collides with an existing block not included in the reorder")
		}
	}

	hasTextBlock := false

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		for blockID, position := range positions {
			if err := tx.Model(&models.NoteBlock{}).Where("id = ? AND note_id = ?", blockID, noteID).
				Update("position", position).Error; err != nil {
				return err
			}
			if !hasTextBlock {
				var block models.NoteBlock
				if err := tx.Select("type").First(&block, blockID).Error; err == nil && block.Type == "text" {
					hasTextBlock = true
				}
			}
		}

		// Sync description if any text blocks were reordered (the first text block may have changed)
		if hasTextBlock {
			if err := syncFirstTextBlockToDescriptionTx(tx, noteID); err != nil {
				log.Printf("Warning: failed to sync description for note %d after reorder: %v", noteID, err)
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Sync mention relations after reorder
	if hasTextBlock {
		var note models.Note
		if e := ctx.db.First(&note, noteID).Error; e == nil {
			ctx.syncMentionsForNote(&note)
		}
	}

	ctx.maybeRebalanceBlockPositions(noteID)

	return nil
}

// isNullJSON reports whether raw is the literal JSON null token (ignoring
// surrounding whitespace).
func isNullJSON(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

// seedFirstTextBlockFromDescription copies the note's existing Description into
// editor.Content when the new block will be the note's FIRST text block (by
// position) and its text is empty. This preserves the description through the
// bidirectional description<->first-text-block sync instead of overwriting it
// with empty text. editor.Position must be finalized before this is called.
func seedFirstTextBlockFromDescription(db *gorm.DB, editor *query_models.NoteBlockEditor) error {
	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(editor.Content, &content); err != nil {
		// Content was already validated by the caller; nothing to seed.
		return nil
	}
	if content.Text != "" {
		return nil
	}

	// The new block becomes the first text block only if no existing text block
	// sorts at or before its position. Counting positions (not just existence)
	// is what makes inserting an empty block *before* the current first block
	// safe — it, too, must inherit the description rather than clear it.
	var earlierTextBlocks int64
	if err := db.Model(&models.NoteBlock{}).
		Where("note_id = ? AND type = ? AND position <= ?", editor.NoteID, "text", editor.Position).
		Count(&earlierTextBlocks).Error; err != nil {
		return err
	}
	if earlierTextBlocks > 0 {
		return nil
	}

	var note models.Note
	if err := db.Select("description").First(&note, editor.NoteID).Error; err != nil {
		return err
	}
	if note.Description == "" {
		return nil
	}

	seeded, err := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: note.Description})
	if err != nil {
		return err
	}
	editor.Content = seeded
	return nil
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
		return tx.Model(&models.Note{}).Where("id = ?", noteID).Update("description", "").Error
	}

	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(blocks[0].Content, &content); err != nil {
		return err
	}

	return tx.Model(&models.Note{}).Where("id = ?", noteID).Update("description", content.Text).Error
}

// rebalanceThreshold is the maximum position-string length tolerated before a
// note's block positions are auto-rebalanced. Kept well under the size:64
// Position column cap so growth never reaches the hard varchar(64) insert
// failure on Postgres (SQLite would otherwise silently keep growing).
const rebalanceThreshold = 8

// maybeRebalanceBlockPositions auto-rebalances a note's block positions when any
// position string has grown past rebalanceThreshold. Best-effort and called
// after a create/reorder commits; failures are logged, not propagated.
func (ctx *MahresourcesContext) maybeRebalanceBlockPositions(noteID uint) {
	var positions []string
	if err := ctx.db.Model(&models.NoteBlock{}).
		Where("note_id = ?", noteID).
		Pluck("position", &positions).Error; err != nil {
		log.Printf("Warning: failed to read positions for rebalance check on note %d: %v", noteID, err)
		return
	}
	if !lib.NeedsRebalancing(positions, rebalanceThreshold) {
		return
	}
	if err := ctx.RebalanceBlockPositions(noteID); err != nil {
		log.Printf("Warning: auto-rebalance failed for note %d: %v", noteID, err)
	}
}

// RebalanceBlockPositions normalizes block positions for a note to prevent position string growth.
// This reassigns positions using evenly distributed values (e.g., "d", "h", "l", "p", "t").
// Call this periodically or when positions become too long.
func (ctx *MahresourcesContext) RebalanceBlockPositions(noteID uint) error {
	if !ctx.NoteVisible(noteID) {
		return gorm.ErrRecordNotFound
	}
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
// It supports both URL-based and resource-based calendar sources.
//
// ## Caching Strategy (Backend Tier)
//
// URL-based calendars use a two-layer caching approach:
//   - LRU cache (ICSCache) stores fetched ICS content with configurable TTL (default 30 min)
//   - Conditional HTTP requests (ETag/Last-Modified) minimize bandwidth when refreshing
//   - Stale cache entries are returned if conditional fetch fails (resilience)
//
// Resource-based calendars read directly from storage (no HTTP caching needed).
//
// The frontend (blockCalendar.js) has its own shorter cache (5 min stale threshold)
// for instant UI feedback. This tiered approach balances responsiveness with efficiency.
//
// ## Limitations
//
// This parser does not support recurring events (RRULE). Events with RRULE will
// only show their first occurrence. Full RRULE expansion would require significant
// complexity to handle all recurrence patterns correctly.
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
	var calendarErrors []interfaces.CalendarError
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
				calendarErrors = append(calendarErrors, interfaces.CalendarError{
					CalendarID: cal.ID,
					Error:      "resource source missing resourceId",
				})
				log.Printf("Calendar %s: resource source missing resourceId", cal.ID)
				continue
			}
			icsContent, fetchTime, fetchErr = ctx.fetchICSFromResource(*cal.Source.ResourceID)
		default:
			calendarErrors = append(calendarErrors, interfaces.CalendarError{
				CalendarID: cal.ID,
				Error:      fmt.Sprintf("unknown source type: %s", cal.Source.Type),
			})
			log.Printf("Calendar %s: unknown source type %s", cal.ID, cal.Source.Type)
			continue
		}

		if fetchErr != nil {
			calendarErrors = append(calendarErrors, interfaces.CalendarError{
				CalendarID: cal.ID,
				Error:      fmt.Sprintf("failed to fetch: %v", fetchErr),
			})
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
			calendarErrors = append(calendarErrors, interfaces.CalendarError{
				CalendarID: cal.ID,
				Error:      fmt.Sprintf("failed to parse: %v", parseErr),
			})
			log.Printf("Calendar %s: failed to parse ICS: %v", cal.ID, parseErr)
			continue
		}

		allEvents = append(allEvents, events...)

		// Add calendar info
		calendars = append(calendars, interfaces.CalendarInfo{
			ID:    cal.ID,
			Name:  cal.Name,
			Color: cal.Color,
		})
	}

	// Parse and merge custom events from block state
	if len(block.State) > 0 {
		customEvents, hasCustom := ctx.parseCustomEventsFromState(block.State, start, end)
		if len(customEvents) > 0 {
			allEvents = append(allEvents, customEvents...)
		}
		// Add "custom" calendar info if there are any custom events defined
		if hasCustom {
			calendars = append(calendars, interfaces.CalendarInfo{
				ID:    "custom",
				Name:  "My Events",
				Color: "#6366f1", // Indigo - distinct from the palette
			})
		}
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
		Errors:    calendarErrors,
		CachedAt:  cachedAt.UTC().Format(time.RFC3339),
	}, nil
}

// fetchICSFromURL fetches ICS content from a URL with caching support.
// Returns the content, the time it was fetched, and any error.
func (ctx *MahresourcesContext) fetchICSFromURL(url string) ([]byte, time.Time, error) {
	// Get configured TTL or default
	cacheTTL := ctx.Config.ICSCacheTTL
	if cacheTTL == 0 {
		cacheTTL = 30 * time.Minute
	}

	// Check cache first
	if entry, ok := ctx.icsCache.Get(url); ok {
		if entry.IsFresh(cacheTTL) {
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

// allowedICSScheme reports whether rawURL uses an http(s) scheme. Restricting
// the scheme (and re-validating redirect targets) limits the calendar fetch from
// being used as an SSRF primitive against non-http internal services (e.g.
// file://, gopher://). Private-IP filtering is intentionally NOT applied so that
// legitimate internal calendar servers keep working on private-network deployments.
func allowedICSScheme(rawURL string) bool {
	u, err := neturl.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// fetchAndCacheICS performs the actual HTTP fetch with optional conditional headers.
func (ctx *MahresourcesContext) fetchAndCacheICS(url string, existingEntry *ICSCacheEntry) ([]byte, time.Time, error) {
	if !allowedICSScheme(url) {
		return nil, time.Time{}, fmt.Errorf("calendar URL must use http or https")
	}

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

	// Use configured timeouts. Re-validate every redirect target so a permitted
	// initial http(s) URL cannot redirect into a non-http(s) scheme.
	client := &http.Client{
		Timeout: ctx.Config.RemoteResourceConnectTimeout,
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if !allowedICSScheme(r.URL.String()) {
				return fmt.Errorf("redirect to non-http(s) URL blocked")
			}
			return nil
		},
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
		ctx.icsCache.Set(url, existingEntry.Content, existingEntry.ETag, existingEntry.LastModified)
		return existingEntry.Content, time.Now(), nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Limit read size to prevent memory exhaustion from large responses.
	// Read one extra byte beyond maxICSFileSize to distinguish "exactly max size" from "too large".
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxICSFileSize+1))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to read response: %w", err)
	}

	// Check if we exceeded the size limit
	if int64(len(content)) > maxICSFileSize {
		return nil, time.Time{}, fmt.Errorf("ICS file exceeds maximum size of %d bytes", maxICSFileSize)
	}

	// Cache the result
	etag := resp.Header.Get("ETag")
	lastModified := resp.Header.Get("Last-Modified")
	ctx.icsCache.Set(url, content, etag, lastModified)

	return content, time.Now(), nil
}

// fetchICSFromResource reads ICS content from a stored resource file.
// Like URL fetches, this enforces maxICSFileSize to prevent memory exhaustion.
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

	// Open and read the file with size limit (consistent with URL fetch)
	f, err := fs.Open(resource.GetCleanLocation())
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to open resource file: %w", err)
	}
	defer f.Close()

	content, err := io.ReadAll(io.LimitReader(f, maxICSFileSize+1))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to read resource file: %w", err)
	}

	// Check if we exceeded the size limit
	if int64(len(content)) > maxICSFileSize {
		return nil, time.Time{}, fmt.Errorf("ICS resource file exceeds maximum size of %d bytes", maxICSFileSize)
	}

	// Use the resource's updated time as the fetch time
	return content, resource.UpdatedAt, nil
}

// parseCustomEventsFromState parses custom events from block state and filters to the date range.
// Returns the filtered events and whether any custom events exist in the state (for calendar info).
func (ctx *MahresourcesContext) parseCustomEventsFromState(stateJSON []byte, start, end time.Time) ([]interfaces.CalendarEvent, bool) {
	var state struct {
		CustomEvents []block_types.CustomCalendarEvent `json:"customEvents"`
	}
	if err := json.Unmarshal(stateJSON, &state); err != nil {
		log.Printf("Failed to parse custom events from state: %v", err)
		return nil, false
	}

	if len(state.CustomEvents) == 0 {
		return nil, false
	}

	var filtered []interfaces.CalendarEvent
	for _, ce := range state.CustomEvents {
		eventStart, err := time.Parse(time.RFC3339, ce.Start)
		if err != nil {
			log.Printf("Custom event %s: failed to parse start time: %v", ce.ID, err)
			continue
		}
		eventEnd, err := time.Parse(time.RFC3339, ce.End)
		if err != nil {
			log.Printf("Custom event %s: failed to parse end time: %v", ce.ID, err)
			continue
		}

		// Filter to date range (same logic as ICS events)
		if eventStart.Before(end) && eventEnd.After(start) {
			filtered = append(filtered, interfaces.CalendarEvent{
				ID:          ce.ID,
				CalendarID:  ce.CalendarID, // "custom"
				Title:       ce.Title,
				Start:       eventStart,
				End:         eventEnd,
				AllDay:      ce.AllDay,
				Location:    ce.Location,
				Description: ce.Description,
			})
		}
	}

	return filtered, true
}
