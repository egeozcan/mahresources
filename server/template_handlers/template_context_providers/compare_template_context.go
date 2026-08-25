package template_context_providers

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/c2h5oh/datasize"

	"github.com/flosch/pongo2/v4"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
)

func CompareContextProvider(context ComparePageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		// Set on the base rather than on the success path alone, because the layout
		// reserves the sidebar for every state this provider can return: an error
		// page has no more to put in it than a comparison does. This page has
		// nothing to put there, and it is the page that most wants the width — two
		// panes side by side. On mobile the empty aside would otherwise render as a
		// "Filters and details" disclosure that reveals nothing.
		baseContext := StaticTemplateCtx(request).Update(pongo2.Context{"hideSidebar": true})

		var query query_models.CrossVersionCompareQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, baseContext)
		}

		// Validate required params
		if query.Resource1ID == 0 {
			return baseContext.Update(pongo2.Context{
				"pageTitle":    "Compare Versions",
				"errorMessage": "Pick a resource to compare versions of — open one and use Compare from its version panel.",
				"query":        query,
			})
		}

		// Default r2 to r1 if not provided
		if query.Resource2ID == 0 {
			query.Resource2ID = query.Resource1ID
		}

		// Get resource 1 and its versions for the picker
		resource1, err := context.GetResource(query.Resource1ID)
		if err != nil {
			return addErrContext(err, baseContext)
		}
		versions1, err := context.GetVersions(query.Resource1ID)
		if err != nil {
			return addErrContext(err, baseContext)
		}
		versions1 = persistedVersions(versions1)

		// Get resource 2 and its versions
		resource2, err := context.GetResource(query.Resource2ID)
		if err != nil {
			return addErrContext(err, baseContext)
		}
		versions2, err := context.GetVersions(query.Resource2ID)
		if err != nil {
			return addErrContext(err, baseContext)
		}
		versions2 = persistedVersions(versions2)

		// Fill in missing version numbers and redirect, so the URL always names what is
		// on screen — otherwise the selects render a version the Alpine state does not
		// hold, and the next pick writes v1=0 back into the URL.
		//
		// The current version (matching CurrentVersionID) is used rather than the highest
		// number, since merge-transferred versions can carry higher numbers while
		// representing different files. Same-resource defaults to previous-vs-current.
		if query.Version1 == 0 || query.Version2 == 0 {
			v1 := query.Version1
			v2 := query.Version2

			if query.Resource1ID == query.Resource2ID {
				cur := currentVersionNumber(resource1, versions1)
				if v2 == 0 {
					v2 = cur
					// A URL naming the current version and nothing else would
					// otherwise fill the empty side with the version already on the
					// other one, and answer "there is nothing to compare" for a
					// resource with two partners available.
					if v2 == v1 {
						v2 = previousVersionNumber(versions1, v1)
					}
				}
				if v1 == 0 {
					v1 = previousVersionNumber(versions1, v2)
				}
			} else {
				if v1 == 0 {
					v1 = currentVersionNumber(resource1, versions1)
				}
				if v2 == 0 {
					v2 = currentVersionNumber(resource2, versions2)
				}
			}

			// Redirect only when both versions resolved and the URL actually changes:
			// a single-version resource resolves v1 and v2 to the same number, and
			// redirecting to the URL we are already on would loop.
			if v1 > 0 && v2 > 0 && (v1 != query.Version1 || v2 != query.Version2) {
				// Built from the request's own URL rather than a literal path, so a
				// `.json` or `.body` suffix survives the hop and a client that asked
				// for one is not answered with a page. RequestURI, not String: the
				// absolute form would let a proxied Host header steer it off-site.
				target := &url.URL{}
				*target = *request.URL
				params := target.Query()
				params.Set("r1", fmt.Sprintf("%d", query.Resource1ID))
				params.Set("v1", fmt.Sprintf("%d", v1))
				params.Set("r2", fmt.Sprintf("%d", query.Resource2ID))
				params.Set("v2", fmt.Sprintf("%d", v2))
				target.RawQuery = params.Encode()
				return baseContext.Update(pongo2.Context{
					"_redirect": target.RequestURI(),
				})
			}
		}

		// Perform comparison if both versions specified
		var comparison *models.VersionComparison
		if query.Version1 > 0 && query.Version2 > 0 {
			comparison, err = context.CompareVersionsCross(
				query.Resource1ID, query.Version1,
				query.Resource2ID, query.Version2,
			)
			if err != nil {
				return addErrContext(err, baseContext)
			}
		}

		// Both sides have to agree on the category. Deciding from Version1 alone sends a
		// JSON-versus-PNG comparison to the text diff, which fetches the PNG in full and
		// prints its bytes as added lines. When they disagree the binary panel is right:
		// the type change is the difference, and the metadata already states it.
		contentCategory := "binary"
		if comparison != nil && comparison.Version1 != nil && comparison.Version2 != nil {
			c1 := contentCategoryFor(comparison.Version1.ContentType)
			if c1 == contentCategoryFor(comparison.Version2.ContentType) {
				contentCategory = c1
			}
		}

		// Merging is offered only between two resources, each at its current version;
		// merging an older one would silently promote it. The section renders either way
		// and carries the reason — a control that vanishes teaches nothing, which is the
		// argument bulkCompareAction.tpl already makes for its own two-selection rule.
		crossResource := query.Resource1ID != query.Resource2ID
		canMerge := false
		mergeBlockedReason := ""
		switch {
		case !crossResource:
			mergeBlockedReason = "Merging joins two different resources. Pick a different resource on one side to merge."
		default:
			cv1 := currentVersionNumber(resource1, versions1)
			cv2 := currentVersionNumber(resource2, versions2)
			switch {
			case cv1 == 0 || cv2 == 0:
				mergeBlockedReason = "One of these resources has no current version, so there is nothing to merge into."
			case query.Version1 != cv1 && query.Version2 != cv2:
				mergeBlockedReason = fmt.Sprintf("Both sides are showing older versions. Merging acts on the current versions, v%d and v%d.", cv1, cv2)
			case query.Version1 != cv1:
				mergeBlockedReason = fmt.Sprintf("The left side is showing v%d, not the current v%d. Merging acts on the current version.", query.Version1, cv1)
			case query.Version2 != cv2:
				mergeBlockedReason = fmt.Sprintf("The right side is showing v%d, not the current v%d. Merging acts on the current version.", query.Version2, cv2)
			default:
				canMerge = true
			}
		}

		// Side labels. Across two resources the name is the only thing that tells the
		// panes apart, so it is the label; "Left"/"Right" only restate which side you are
		// looking at. Truncated because it heads a pane, not a paragraph.
		label1, label2 := shortResourceName(resource1), shortResourceName(resource2)
		// Two names sharing a long prefix truncate to one string, and then every
		// control that carries a name — the pane headers, the breadcrumb, the
		// merge buttons — says the same thing on both sides. The id is what
		// distinguishes them when the names no longer do.
		if crossResource && label1 == label2 {
			label1 = fmt.Sprintf("%s (#%d)", label1, resource1.ID)
			label2 = fmt.Sprintf("%s (#%d)", label2, resource2.ID)
		}
		if !crossResource && query.Version1 > 0 && query.Version2 > 0 {
			if query.Version1 == query.Version2 {
				label1 = fmt.Sprintf("v%d", query.Version1)
				label2 = fmt.Sprintf("v%d", query.Version2)
			} else {
				curVer := currentVersionNumber(resource1, versions1)
				v1IsCurrent := query.Version1 == curVer
				v2IsCurrent := query.Version2 == curVer

				switch {
				case v1IsCurrent:
					label1 = "Current"
					label2 = fmt.Sprintf("v%d", query.Version2)
				case v2IsCurrent:
					label1 = fmt.Sprintf("v%d", query.Version1)
					label2 = "Current"
				default:
					if query.Version1 > query.Version2 {
						label1 = "Newer"
						label2 = "Older"
					} else {
						label1 = "Older"
						label2 = "Newer"
					}
				}
			}
		}

		// Built here rather than assembled inside an attribute, so the sentence
		// reaches the page as one JSON string: pongo2's escapejs emits a bare
		// `\u1F600` for an astral character, which JavaScript reads as `\u1F60`
		// followed by a stray "0".
		mergeConfirm1 := fmt.Sprintf(
			"Merge %s into %s? %s will no longer exist as its own resource. This cannot be undone.",
			resource2.Name, resource1.Name, resource2.Name)
		mergeConfirm2 := fmt.Sprintf(
			"Merge %s into %s? %s will no longer exist as its own resource. This cannot be undone.",
			resource1.Name, resource2.Name, resource1.Name)

		// Two names for one file. The comparators would fetch it twice to diff it
		// against itself, and the PDF panel would offer two identical viewers
		// directly under a notice saying there is nothing to compare.
		sameVersion := comparison != nil && !crossResource && query.Version1 == query.Version2

		current1 := currentVersionNumber(resource1, versions1)
		current2 := currentVersionNumber(resource2, versions2)

		compareUnavailableReason := ""
		if comparison == nil {
			switch {
			case len(versions1) == 0 && len(versions2) == 0:
				compareUnavailableReason = "Neither of these resources has a version history yet, so there is nothing to compare."
			case len(versions1) == 0:
				compareUnavailableReason = fmt.Sprintf("%s has no version history yet, so there is nothing to compare it against.", shortResourceName(resource1))
			case len(versions2) == 0:
				compareUnavailableReason = fmt.Sprintf("%s has no version history yet, so there is nothing to compare it against.", shortResourceName(resource2))
			}
		}

		return baseContext.Update(pongo2.Context{
			// A constant title left every tab, bookmark and history entry saying the
			// same thing whatever was being compared.
			"pageTitle": compareTitle(resource1, resource2, crossResource, query.Version1, query.Version2),
			"resource1": resource1,
			"resource2": resource2,
			"name1":     shortResourceName(resource1),
			"name2":     shortResourceName(resource2),
			// The shared autocompleter takes its initial selection as a list. Passing
			// the model itself would serialise every association it preloaded into the
			// page for a control that only ever shows a name.
			"resource1Picker": comparePickerItems(resource1),
			"resource2Picker": comparePickerItems(resource2),
			"versions1":       versionOptions(versions1, current1),
			"versions2":       versionOptions(versions2, current2),
			// Set when a side has nothing to offer, so the empty state says why
			// rather than inviting a pick from a dropdown with no options in it.
			"compareUnavailableReason": compareUnavailableReason,
			"comparison":               comparison,
			"query":                    query,
			"contentCategory":          contentCategory,
			"crossResource":            crossResource,
			"sameVersion":              sameVersion,
			"canMerge":                 canMerge,
			"mergeBlockedReason":       mergeBlockedReason,
			"mergeConfirm1":            mergeConfirm1,
			"mergeConfirm2":            mergeConfirm2,
			"label1":                   label1,
			"label2":                   label2,
			// Pane headers. Every comparator renders the same two strings, so they are
			// built once rather than assembled from label + version in four templates.
			"panelTitle1": comparePanelTitle(label1, comparison, crossResource, true),
			"panelTitle2": comparePanelTitle(label2, comparison, crossResource, false),
		})
	}
}

// persistedVersions drops the version GetVersions synthesises for a resource
// whose history has not been migrated. That row stands in for the current file
// in a version panel, but it has no id, so no file route can serve it and
// GetVersionByNumber — which reads the table — cannot find it. Offering it here
// fills a dropdown with an option that answers "version 1 not found", and
// defaulting to it redirects straight there.
func persistedVersions(versions []models.ResourceVersion) []models.ResourceVersion {
	kept := make([]models.ResourceVersion, 0, len(versions))
	for _, v := range versions {
		if v.ID != 0 {
			kept = append(kept, v)
		}
	}
	return kept
}

// currentVersionNumber returns the version number that matches the resource's
// CurrentVersionID. Falls back to the latest version number if CurrentVersionID
// is not set or not found in the versions list.
func currentVersionNumber(resource *models.Resource, versions []models.ResourceVersion) int {
	if resource.CurrentVersionID != nil {
		for _, v := range versions {
			if v.ID == *resource.CurrentVersionID {
				return v.VersionNumber
			}
		}
	}
	if len(versions) > 0 {
		return versions[0].VersionNumber
	}
	return 0
}

// previousVersionNumber returns the version to set the current one against: the
// highest number below `than`, or the lowest above it when there is nothing
// below. `than` itself only when the resource has no other version.
//
// The second case is not a curiosity. A merge renumbers the loser's history
// above the winner's highest, and leaves the winner's own current version where
// it was — so a winner whose current version is v1 can hold v2 and v3 as
// history. Answering "there is nothing earlier" there defaulted the page to
// comparing v1 with itself while two other versions sat in the dropdown.
//
// `versions` is ordered by version number descending (see GetVersions).
func previousVersionNumber(versions []models.ResourceVersion, than int) int {
	for _, v := range versions {
		if v.VersionNumber < than {
			return v.VersionNumber
		}
	}
	// Descending, so the last one above `than` is the nearest one above it.
	nearestAbove := 0
	for _, v := range versions {
		if v.VersionNumber > than {
			nearestAbove = v.VersionNumber
		}
	}
	if nearestAbove > 0 {
		return nearestAbove
	}
	return than
}

// contentCategoryFor maps a content type onto the comparator that can render it.
//
// Parsed first, because a stored type is whatever was sniffed or whatever an
// imported manifest declared: "Application/PDF" and "application/pdf; charset=binary"
// name the same thing, while a malformed value names nothing and belongs in the
// binary panel. The version file route reads it the same way, so the comparator a
// version is rendered in and the disposition its bytes are served with agree.
func contentCategoryFor(contentType string) string {
	base, ok := models.ParseContentType(contentType)
	if !ok {
		return "binary"
	}
	switch {
	case strings.HasPrefix(base, "image/") && base != "image/svg+xml":
		return "image"
	case strings.HasPrefix(base, "text/"), base == "application/json", base == "application/xml":
		return "text"
	case base == "application/pdf":
		return "pdf"
	default:
		return "binary"
	}
}

// shortResourceName is a resource's name trimmed to something that fits a pane
// header. It never returns an empty string, because the header would then be a
// coloured bar with nothing in it.
func shortResourceName(resource *models.Resource) string {
	if resource == nil || strings.TrimSpace(resource.Name) == "" {
		return "Untitled"
	}
	name := strings.TrimSpace(resource.Name)
	runes := []rune(name)
	if len(runes) <= 42 {
		return name
	}
	return strings.TrimSpace(string(runes[:41])) + "\u2026"
}

// compareTitle names the comparison in the browser tab and the page heading.
func compareTitle(resource1, resource2 *models.Resource, crossResource bool, v1, v2 int) string {
	if crossResource {
		return fmt.Sprintf("%s vs %s", shortResourceName(resource1), shortResourceName(resource2))
	}
	if v1 > 0 && v2 > 0 {
		return fmt.Sprintf("%s \u00b7 v%d \u2194 v%d", shortResourceName(resource1), v1, v2)
	}
	return fmt.Sprintf("Compare %s", shortResourceName(resource1))
}

// CompareVersionOption is one row of a version dropdown. A version and a date
// alone cannot tell two uploads on the same day apart, and they drop the comment
// somebody wrote precisely so the versions could be told apart.
type CompareVersionOption struct {
	VersionNumber int
	Label         string
	IsCurrent     bool
}

func versionOptions(versions []models.ResourceVersion, current int) []CompareVersionOption {
	options := make([]CompareVersionOption, 0, len(versions))
	sameDay := versionDatesCollide(versions)
	for _, v := range versions {
		// The time is added when a date alone stops identifying a version. The
		// year stays either way: a resource can hold uploads from several years
		// and two from one day, and dropping the year to make room for the clock
		// loses more than it gains.
		layout := "Jan 02, 2006"
		if sameDay {
			layout = "Jan 02, 2006 15:04"
		}
		label := fmt.Sprintf("v%d", v.VersionNumber)
		if v.VersionNumber == current {
			label += " \u00b7 current"
		}
		label += " \u00b7 " + v.CreatedAt.Format(layout)
		label += " \u00b7 " + versionSizeLabel(v.FileSize)
		if comment := strings.TrimSpace(v.Comment); comment != "" {
			label += " \u00b7 " + truncateRunes(comment, 40)
		}
		options = append(options, CompareVersionOption{
			VersionNumber: v.VersionNumber,
			Label:         label,
			IsCurrent:     v.VersionNumber == current,
		})
	}
	return options
}

// versionDatesCollide reports whether any two versions fall on the same calendar
// day, which is when a date alone stops identifying a version.
func versionDatesCollide(versions []models.ResourceVersion) bool {
	seen := make(map[string]struct{}, len(versions))
	for _, v := range versions {
		day := v.CreatedAt.Format("2006-01-02")
		if _, dup := seen[day]; dup {
			return true
		}
		seen[day] = struct{}{}
	}
	return false
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "\u2026"
}

// versionSizeLabel matches the humanReadableSize template filter, so a dropdown
// option — which cannot run a filter over a struct field — reads the same as the
// metadata card beside it.
func versionSizeLabel(size int64) string {
	if size < 0 {
		return "-" + datasize.ByteSize(-size).HumanReadable()
	}
	return datasize.ByteSize(size).HumanReadable()
}

// comparePickerItems shapes a resource as the shared autocompleter's initial
// selection: a one-element list, or an empty one when there is nothing selected.
func comparePickerItems(resource *models.Resource) []map[string]any {
	if resource == nil {
		return []map[string]any{}
	}
	return []map[string]any{{
		"ID":   resource.ID,
		"Name": resource.Name,
	}}
}

// comparePanelTitle heads one pane: the side label, plus the version number when
// both sides are versions of the same resource and the label alone would not say
// which one.
func comparePanelTitle(label string, comparison *models.VersionComparison, crossResource, left bool) string {
	if crossResource || comparison == nil {
		return label
	}
	version := comparison.Version1
	if !left {
		version = comparison.Version2
	}
	if version == nil {
		return label
	}
	if label == fmt.Sprintf("v%d", version.VersionNumber) {
		return label
	}
	return fmt.Sprintf("%s \u2014 v%d", label, version.VersionNumber)
}
