package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"mahresources/application_context"
)

// ShareServer is a separate HTTP server for serving shared notes publicly.
// It runs on a different port from the main server and only exposes
// shared content through cryptographically secure tokens.
type ShareServer struct {
	server     *http.Server
	appContext *application_context.MahresourcesContext
}

// NewShareServer creates a new ShareServer instance
func NewShareServer(appContext *application_context.MahresourcesContext) *ShareServer {
	return &ShareServer{
		appContext: appContext,
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
	router.Methods(http.MethodPut).Path("/s/{token}/block/{blockId}/state").HandlerFunc(s.handleBlockStateUpdate)

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
	s.renderSharedNote(w, note)
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
// This is a placeholder - will be updated to use Pongo2 templates in Task 16
func (s *ShareServer) renderSharedNote(w http.ResponseWriter, note interface{}) {
	// Placeholder - will be implemented with template rendering
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("<html><body><h1>Shared Note</h1><p>Template rendering coming soon</p></body></html>"))
}
