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
	ID      uint
	Type    string
	Content map[string]interface{}
	State   map[string]interface{}
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
	fmt.Sscanf(blockIdStr, "%d", &blockId)

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

// handleSharedResource serves a resource (image/file) that belongs to a shared note
// It validates that the token is valid and the resource belongs to the note
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

	// Verify resource belongs to this note
	resourceBelongsToNote := false
	for _, resource := range note.Resources {
		if resource.Hash == hash {
			resourceBelongsToNote = true
			break
		}
	}
	if !resourceBelongsToNote {
		http.Error(w, "Resource not found", http.StatusNotFound)
		return
	}

	// Serve the resource
	s.appContext.ServeResourceByHash(w, r, hash)
}

// renderSharedNote renders a shared note using templates
func (s *ShareServer) renderSharedNote(w http.ResponseWriter, note *models.Note, shareToken string) {
	template := pongo2.Must(s.templateSet.FromFile("/shared/displayNote.tpl"))

	// Build a map of resource ID to hash for gallery blocks
	resourceHashMap := make(map[uint]string)
	for _, resource := range note.Resources {
		resourceHashMap[resource.ID] = resource.Hash
	}

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

		blocks = append(blocks, tb)
	}

	ctx := pongo2.Context{
		"note":            note,
		"blocks":          blocks,
		"pageTitle":       note.Name,
		"shareToken":      shareToken,
		"resourceHashMap": resourceHashMap,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := template.ExecuteWriter(ctx, w); err != nil {
		log.Printf("Error rendering shared note template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
