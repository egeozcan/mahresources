// Package mcpserver serves the mahresources API over the Model Context
// Protocol (2026-07-28, stateless Streamable HTTP; older protocol versions are
// served by the same handler).
//
// Every tool call is dispatched as an ordinary HTTP request through the same
// middleware chain the REST API uses, as the caller, so authentication, roles,
// group scope, plugin permissions and rate limits apply unchanged. Destructive
// operations additionally need a confirmation the client showed its user; see
// confirm.go and tools.go.
package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpPath = "/mcp"

// Path is where the MCP endpoint is mounted.
const Path = mcpPath

// Options configures the MCP handler.
type Options struct {
	// Dispatch is the full REST handler chain (authentication, CSRF,
	// authorization, body limits, router). Tool calls are served through it.
	Dispatch http.Handler
	// Spec is the generated OpenAPI document the catalog is built from.
	Spec *openapi3.T
	// SigningKey is mixed into the confirmation key.
	SigningKey []byte
	// MaxRequestBody caps an MCP request body. Zero uses the SDK default.
	MaxRequestBody int64
	// MaxResponse caps a captured JSON or text response. Zero uses 8 MiB.
	MaxResponse int64
	// Version is reported as the server version.
	Version string
}

// Handler serves POST /mcp.
type Handler struct {
	opts      Options
	catalog   *Catalog
	confirmer *confirmer
	named     []namedTool
	sdk       http.Handler
}

// New builds the handler. It fails when the catalog cannot be built, which
// includes a non-GET operation with no safety class.
func New(opts Options) (*Handler, error) {
	if opts.Dispatch == nil || opts.Spec == nil {
		return nil, errors.New("mcpserver: Dispatch and Spec are required")
	}
	if opts.MaxResponse <= 0 {
		opts.MaxResponse = 8 << 20
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	catalog, err := BuildCatalog(opts.Spec)
	if err != nil {
		return nil, err
	}
	conf, err := newConfirmer(opts.SigningKey)
	if err != nil {
		return nil, err
	}
	named, err := buildNamedTools(catalog)
	if err != nil {
		return nil, err
	}
	h := &Handler{opts: opts, catalog: catalog, confirmer: conf, named: named}
	h.sdk = mcp.NewStreamableHTTPHandler(h.serverFor, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		MaxRequestBodyBytes:          opts.MaxRequestBody,
		PropagateRequestCancellation: true,
		// Admission (below) refuses every browser request, which is what the
		// SDK's loopback Host check guards against, and that check would
		// otherwise refuse a reverse proxy on localhost forwarding a public
		// Host header.
		DisableLocalhostProtection: true,
	})
	return h, nil
}

// Catalog returns the operation catalog.
func (h *Handler) Catalog() *Catalog { return h.catalog }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.sdk.ServeHTTP(w, r)
}

// serverFor builds the MCP server for one HTTP request. The server is
// stateless, so a request is a whole session, and building it per request
// lets every tool handler dispatch as that request's caller.
func (h *Handler) serverFor(r *http.Request) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "mahresources", Title: "Mahresources", Version: h.opts.Version}, &mcp.ServerOptions{
		Instructions: serverInstructions,
		SetCacheable: func(_ context.Context, _ mcp.Request, c *mcp.Cacheable) {
			// The tool list depends only on the binary, but answers are
			// scoped to the caller, so no shared cache may keep them.
			c.CacheScope = "private"
			c.TTLMs = 5 * 60 * 1000
		},
	})
	call := &caller{h: h, r: r}
	for _, t := range h.named {
		s.AddTool(t.tool, call.named(t))
	}
	for _, t := range genericTools() {
		s.AddTool(t, call.generic(t.Name))
	}
	return s
}

const serverInstructions = `Mahresources is a personal information manager for Resources (files), Notes and Groups, with Tags, Categories, Relations, Series, saved Queries and MRQL (its query language).

Start with the named tools (search, mrql_query, list_*, get_*). For anything else, find the operation with search_operations, read its arguments with describe_operation, and run it with call_operation. Every REST operation the caller may use is reachable this way.

Destructive operations (deletes, merges, mass edits, publishing, system settings) ask the user to confirm through the client before they run. If the client cannot show a confirmation, they are refused.

Long-running work (downloads, exports, imports, plugin actions) returns a job ID; poll it with get_job.`

// Admission refuses what the MCP endpoint must never serve. It has to wrap
// the handler chain from outside the CSRF middleware, so a cookie-only
// request is refused as unauthenticated (401) instead of reaching CSRF (403).
//
//   - Browser requests: any request carrying Origin or Sec-Fetch-Site, unless
//     its Origin is allowlisted. Non-browser MCP clients send neither. This
//     holds with authentication disabled too, and it covers DNS rebinding,
//     where Origin and Host agree and a same-origin check would pass.
//   - With authentication enabled, any request without a Bearer token.
//   - Methods other than POST: the server is stateless, so there is no
//     session to GET or DELETE.
func Admission(authEnabled bool, allowedOrigins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != mcpPath {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" || r.Header.Get("Sec-Fetch-Site") != "" {
			if origin == "" || !slices.Contains(allowedOrigins, origin) {
				http.Error(w, "Forbidden: browser requests to the MCP endpoint are refused", http.StatusForbidden)
				return
			}
		}
		if authEnabled && !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.Header().Set("WWW-Authenticate", `Bearer realm="mahresources"`)
			http.Error(w, "Unauthorized: the MCP endpoint requires an API token (Authorization: Bearer)", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}
