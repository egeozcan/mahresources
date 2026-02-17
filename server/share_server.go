package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/flosch/pongo2/v4"
	"github.com/gorilla/mux"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/server/template_handlers/loaders"
	_ "mahresources/server/template_handlers/template_filters"
)

// templateBlock represents a block with decoded Content and State for template rendering
type templateBlock struct {
	ID        uint
	Type      string
	Content   map[string]interface{}
	State     map[string]interface{}
	QueryData map[string]interface{} // For query-based tables: contains "columns" and "rows"
}

// groupInfo holds group data for template rendering in shared views
type groupInfo struct {
	Name         string
	Description  string
	CategoryName string
}

// ShareServer is a separate HTTP server for serving shared notes publicly.
// It runs on a different port from the main server and only exposes
// shared content through cryptographically secure tokens.
type ShareServer struct {
	server      *http.Server
	appContext  *application_context.MahresourcesContext
	templateSet *pongo2.TemplateSet
}

// NewShareServer creates a new ShareServer instance
func NewShareServer(appContext *application_context.MahresourcesContext) *ShareServer {
	// Initialize template set for shared templates
	templateSet := pongo2.NewSet("", loaders.MustNewLocalFileSystemLoader("./templates", make(map[string]string)))
	return &ShareServer{
		appContext:  appContext,
		templateSet: templateSet,
	}
}

// Start begins the share server on the specified address and port.
// If port is empty, the server is not started (share feature disabled).
// The server runs in a goroutine and returns immediately.
func (s *ShareServer) Start(bindAddress string, port string) error {
	if port == "" {
		return nil // Share server disabled
	}

	router := mux.NewRouter()
	s.registerShareRoutes(router)

	addr := fmt.Sprintf("%s:%s", bindAddress, port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Share server starting on %s", addr)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Share server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the share server with a 5 second timeout
func (s *ShareServer) Stop() error {
	if s.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// registerShareRoutes sets up all routes for the share server
func (s *ShareServer) registerShareRoutes(router *mux.Router) {
	// Shared note view
	router.Methods(http.MethodGet).Path("/s/{token}").HandlerFunc(s.handleSharedNote)

	// Block state update (for interactive todos)
	router.Methods(http.MethodPost).Path("/s/{token}/block/{blockId}/state").HandlerFunc(s.handleBlockStateUpdate)

	// Calendar events for calendar blocks
	router.Methods(http.MethodGet).Path("/s/{token}/block/{blockId}/calendar/events").HandlerFunc(s.handleCalendarEvents)

	// Resource serving (for gallery images)
	router.Methods(http.MethodGet).Path("/s/{token}/resource/{hash}").HandlerFunc(s.handleSharedResource)

	// Static assets
	router.PathPrefix("/public/").Handler(http.StripPrefix("/public/", mimeTypeHandler(http.FileServer(http.Dir("public")))))
}

// handleSharedNote serves a shared note by its token
func (s *ShareServer) handleSharedNote(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]

	note, err := s.appContext.GetNoteByShareToken(token)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Render the shared note template
	s.renderSharedNote(w, note, token)
}

// handleBlockStateUpdate updates a block's state (e.g., todo checkbox)
// It validates that the token is valid and the block belongs to the note
func (s *ShareServer) handleBlockStateUpdate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]
	blockIdStr := vars["blockId"]

	// Verify token and get note
	note, err := s.appContext.GetNoteByShareToken(token)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Parse block ID
	var blockId uint
	if _, err := fmt.Sscanf(blockIdStr, "%d", &blockId); err != nil {
		http.Error(w, "Invalid block ID", http.StatusBadRequest)
		return
	}

	// Verify block belongs to this note
	blockBelongsToNote := false
	for _, block := range note.Blocks {
		if block.ID == blockId {
			blockBelongsToNote = true
			break
		}
	}
	if !blockBelongsToNote {
		http.Error(w, "Block not found", http.StatusNotFound)
		return
	}

	// Update block state
	err = s.appContext.UpdateBlockStateFromRequest(blockId, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success": true}`))
}

// handleCalendarEvents returns calendar events for a calendar block in a shared note
func (s *ShareServer) handleCalendarEvents(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]
	blockIdStr := vars["blockId"]

	// Verify token and get note
	note, err := s.appContext.GetNoteByShareToken(token)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Parse block ID
	var blockId uint
	if _, err := fmt.Sscanf(blockIdStr, "%d", &blockId); err != nil {
		http.Error(w, "Invalid block ID", http.StatusBadRequest)
		return
	}

	// Verify block belongs to this note and is a calendar block
	blockBelongsToNote := false
	for _, block := range note.Blocks {
		if block.ID == blockId && block.Type == "calendar" {
			blockBelongsToNote = true
			break
		}
	}
	if !blockBelongsToNote {
		http.Error(w, "Block not found", http.StatusNotFound)
		return
	}

	// Parse date range from query params
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	if startStr == "" || endStr == "" {
		http.Error(w, "start and end dates required", http.StatusBadRequest)
		return
	}

	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		http.Error(w, "invalid start date", http.StatusBadRequest)
		return
	}

	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		http.Error(w, "invalid end date", http.StatusBadRequest)
		return
	}
	end = end.Add(24*time.Hour - time.Second)

	// Fetch calendar events
	response, err := s.appContext.GetCalendarEvents(blockId, start, end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// handleSharedResource serves a resource (image/file) that belongs to a shared note
// It validates that the token is valid and the resource is referenced in the note
// (either in note.Resources or in a gallery block)
func (s *ShareServer) handleSharedResource(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]
	hash := vars["hash"]

	// Verify token
	note, err := s.appContext.GetNoteByShareToken(token)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Check if resource is in note.Resources
	resourceAllowed := false
	for _, resource := range note.Resources {
		if resource.Hash == hash {
			resourceAllowed = true
			break
		}
	}

	// If not found in note.Resources, check gallery blocks
	if !resourceAllowed {
		// Collect resource IDs from gallery blocks
		resourceIdsSet := make(map[uint]bool)
		for _, block := range note.Blocks {
			if block.Type == "gallery" && len(block.Content) > 0 {
				var content map[string]interface{}
				if err := json.Unmarshal(block.Content, &content); err == nil {
					if resourceIds, ok := content["resourceIds"].([]interface{}); ok {
						for _, rId := range resourceIds {
							if id, ok := rId.(float64); ok {
								resourceIdsSet[uint(id)] = true
							}
						}
					}
				}
			}
		}

		// Load those resources and check if hash matches
		if len(resourceIdsSet) > 0 {
			resourceIds := make([]uint, 0, len(resourceIdsSet))
			for id := range resourceIdsSet {
				resourceIds = append(resourceIds, id)
			}
			if resources, err := s.appContext.GetResourcesWithIds(&resourceIds); err == nil {
				for _, resource := range resources {
					if resource.Hash == hash {
						resourceAllowed = true
						break
					}
				}
			}
		}
	}

	if !resourceAllowed {
		http.Error(w, "Resource not found", http.StatusNotFound)
		return
	}

	// Serve the resource
	s.appContext.ServeResourceByHash(w, r, hash)
}

// renderSharedNote renders a shared note using templates
func (s *ShareServer) renderSharedNote(w http.ResponseWriter, note *models.Note, shareToken string) {
	template := pongo2.Must(s.templateSet.FromFile("/shared/displayNote.tpl"))

	// Collect all group IDs from references blocks and resource IDs from gallery blocks
	groupIdsSet := make(map[uint]bool)
	resourceIdsSet := make(map[uint]bool)

	// Convert blocks to template-friendly format with decoded JSON
	blocks := make([]templateBlock, 0, len(note.Blocks))
	for _, block := range note.Blocks {
		tb := templateBlock{
			ID:      block.ID,
			Type:    block.Type,
			Content: make(map[string]interface{}),
			State:   make(map[string]interface{}),
		}

		// Decode Content JSON
		if len(block.Content) > 0 {
			if err := json.Unmarshal(block.Content, &tb.Content); err != nil {
				log.Printf("Error decoding block content: %v", err)
			}
		}

		// Decode State JSON
		if len(block.State) > 0 {
			if err := json.Unmarshal(block.State, &tb.State); err != nil {
				log.Printf("Error decoding block state: %v", err)
			}
		}

		// Collect group IDs from references blocks
		if block.Type == "references" {
			if groupIds, ok := tb.Content["groupIds"].([]interface{}); ok {
				for _, gId := range groupIds {
					if id, ok := gId.(float64); ok {
						groupIdsSet[uint(id)] = true
					}
				}
			}
		}

		// Collect resource IDs from gallery blocks
		if block.Type == "gallery" {
			if resourceIds, ok := tb.Content["resourceIds"].([]interface{}); ok {
				for _, rId := range resourceIds {
					if id, ok := rId.(float64); ok {
						resourceIdsSet[uint(id)] = true
					}
				}
			}
		}

		// Fetch query data for table blocks with queryId
		if block.Type == "table" {
			if queryIdFloat, ok := tb.Content["queryId"].(float64); ok {
				queryId := uint(queryIdFloat)
				// Get query params from content
				params := make(map[string]any)
				if queryParams, ok := tb.Content["queryParams"].(map[string]interface{}); ok {
					for k, v := range queryParams {
						params[k] = v
					}
				}
				// Execute query
				if queryData, err := s.fetchTableQueryData(queryId, params); err == nil {
					tb.QueryData = queryData
				} else {
					log.Printf("Error fetching table query data: %v", err)
				}
			}
		}

		blocks = append(blocks, tb)
	}

	// Build resource hash map from gallery block resources
	// Use float64 keys since JSON numbers come as float64
	resourceHashMap := make(map[float64]string)
	if len(resourceIdsSet) > 0 {
		resourceIds := make([]uint, 0, len(resourceIdsSet))
		for id := range resourceIdsSet {
			resourceIds = append(resourceIds, id)
		}
		if resources, err := s.appContext.GetResourcesWithIds(&resourceIds); err == nil {
			for _, resource := range resources {
				resourceHashMap[float64(resource.ID)] = resource.Hash
			}
		}
	}

	// Build group data map with full group info for tooltips
	groupDataMap := make(map[float64]any)
	if len(groupIdsSet) > 0 {
		groupIds := make([]uint, 0, len(groupIdsSet))
		for id := range groupIdsSet {
			groupIds = append(groupIds, id)
		}
		if groups, err := s.appContext.GetGroupsWithIds(&groupIds); err == nil {
			for _, group := range groups {
				// Use float64 key since JSON numbers come as float64
				info := groupInfo{
					Name:        group.Name,
					Description: group.Description,
				}
				if group.Category != nil {
					info.CategoryName = group.Category.Name
				}
				groupDataMap[float64(group.ID)] = info
			}
		}
	}

	ctx := pongo2.Context{
		"note":            note,
		"blocks":          blocks,
		"pageTitle":       note.Name,
		"shareToken":      shareToken,
		"resourceHashMap": resourceHashMap,
		"groupDataMap":    groupDataMap,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := template.ExecuteWriter(ctx, w); err != nil {
		log.Printf("Error rendering shared note template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// fetchTableQueryData executes a query and returns data formatted for table display
func (s *ShareServer) fetchTableQueryData(queryId uint, params map[string]any) (map[string]interface{}, error) {
	rows, err := s.appContext.RunReadOnlyQuery(queryId, params)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Get column names
	colNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// Build column definitions
	columns := make([]map[string]string, 0, len(colNames))
	for _, colName := range colNames {
		columns = append(columns, map[string]string{
			"id":    colName,
			"label": colName,
		})
	}

	// Scan rows into maps
	resultRows := make([]map[string]interface{}, 0)
	for rows.Next() {
		columnValues := make([]interface{}, len(colNames))
		columnPointers := make([]interface{}, len(colNames))
		for i := range columnValues {
			columnPointers[i] = &columnValues[i]
		}

		if err := rows.Scan(columnPointers...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{})
		for i, colName := range colNames {
			val := columnValues[i]
			// Convert []byte to string for display
			if b, ok := val.([]byte); ok {
				row[colName] = string(b)
			} else {
				row[colName] = val
			}
		}
		resultRows = append(resultRows, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"columns": columns,
		"rows":    resultRows,
	}, nil
}
