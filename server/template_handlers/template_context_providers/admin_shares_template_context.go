package template_context_providers

import (
	"net/http"
	"strings"

	"github.com/flosch/pongo2/v4"

	"mahresources/application_context"
)

// adminSharesRow is the view model the /admin/shares template iterates.
// Exposing a flat struct keeps the template free of type-shape assertions:
// ShareCreatedAtFormatted is the already-rendered date string (empty when the
// underlying pointer is nil, which is the legacy-row case) and the template
// only has to check a string's emptiness.
type adminSharesRow struct {
	ID                      uint
	Name                    string
	ShareToken              string
	ShareCreatedAtFormatted string
}

// AdminSharesContextProvider returns the Pongo2 context for /admin/shares —
// the centralized dashboard that lists every note currently holding a share
// token (BH-035). Columns the template renders: Name | Public URL | Created
// | Revoke. ShareCreatedAt is a nullable timestamp (newly added column);
// existing rows minted before this migration render "(unknown)" rather than
// being back-filled with an inaccurate NOW().
func AdminSharesContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := StaticTemplateCtx(request)

		// BH-035: "share_created_at DESC NULLS LAST" — freshest shares at the
		// top, legacy NULL rows fall to the bottom where the admin can triage
		// them. SQLite and Postgres both support NULLS LAST with an explicit
		// ASC/DESC. See GetSharedNotes in application_context/note_context.go.
		notes, err := context.GetSharedNotes()
		if err != nil {
			return addErrContext(err, baseContext)
		}

		rows := make([]adminSharesRow, 0, len(notes))
		for _, n := range notes {
			row := adminSharesRow{ID: n.ID, Name: n.Name}
			if n.ShareToken != nil {
				row.ShareToken = *n.ShareToken
			}
			// Existing rows minted before BH-035 have ShareCreatedAt == nil.
			// Render them as "(unknown)" in the template — back-filling with
			// NOW() would be misleading because the admin sees a freshly
			// created row for a share token that may be months old.
			if n.ShareCreatedAt != nil {
				row.ShareCreatedAtFormatted = n.ShareCreatedAt.Format("2006-01-02 15:04")
			}
			rows = append(rows, row)
		}

		// BH-035: every card carries the full share URL only if SHARE_PUBLIC_URL
		// is configured; otherwise the template renders the relative /s/<token>
		// path plus a link back to the BH-033 warning. shareBaseUrl keeps the
		// template's URL-building trivially simple (no string concatenation in
		// pongo2).
		shareBaseUrl := ""
		shareUrlConfigured := false
		if context.Config != nil && context.Config.SharePublicURL != "" {
			shareBaseUrl = strings.TrimRight(context.Config.SharePublicURL, "/")
			shareUrlConfigured = true
		}

		return pongo2.Context{
			"pageTitle":          "Shared Notes",
			"hideSidebar":        true,
			"shares":             rows,
			"shareBaseUrl":       shareBaseUrl,
			"shareUrlConfigured": shareUrlConfigured,
		}.Update(baseContext)
	}
}
