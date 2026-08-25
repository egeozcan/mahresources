package api_tests

import (
	"bytes"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
	"mahresources/models/query_models"
	template_context_providers "mahresources/server/template_handlers/template_context_providers"
)

// addCompareVersion appends a version row and points the resource at it, which is
// what a real upload leaves behind. The bytes are irrelevant here — every
// assertion below is about what the provider makes of the metadata.
func addCompareVersion(t *testing.T, tc *TestContext, resourceID uint, number int, contentType, comment string) models.ResourceVersion {
	t.Helper()
	version := models.ResourceVersion{
		ResourceID:    resourceID,
		VersionNumber: number,
		Hash:          fmt.Sprintf("compare-hash-%d-%d", resourceID, number),
		HashType:      "SHA1",
		FileSize:      int64(100 * number),
		ContentType:   contentType,
		Location:      fmt.Sprintf("/fake/%d/v%d", resourceID, number),
		Comment:       comment,
	}
	assert.NoError(t, tc.DB.Create(&version).Error)
	assert.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", resourceID).
		Update("current_version_id", version.ID).Error)
	return version
}

func newCompareResource(t *testing.T, tc *TestContext, name, body string) *models.Resource {
	t.Helper()
	resource, err := tc.AppCtx.AddResource(io.NopCloser(bytes.NewReader([]byte(body))), name+".txt",
		&query_models.ResourceCreator{ResourceQueryBase: query_models.ResourceQueryBase{Name: name}})
	assert.NoError(t, err)
	return resource
}

// A compare URL naming only a resource used to render the empty state while both
// version dropdowns displayed a version: the redirect that fills the numbers in
// ran for cross-resource comparisons only. Picking a version from that state
// wrote v1=0 into the URL and the empty state came back with nothing said.
func TestCompareContextProvider_SameResourceFillsVersions(t *testing.T) {
	tc := SetupTestEnv(t)

	res := newCompareResource(t, tc, "Redirect Target", "compare-redirect-v1")
	addCompareVersion(t, tc, res.ID, 2, "text/plain", "second")

	provider := template_context_providers.CompareContextProvider(tc.AppCtx)

	ctx := provider(httptest.NewRequest("GET", fmt.Sprintf("/resource/compare?r1=%d", res.ID), nil))
	redirect, ok := ctx["_redirect"].(string)
	assert.True(t, ok, "a URL with no versions should redirect to one that names them")
	// Previous versus current: the comparison someone opening a version panel means.
	assert.Contains(t, redirect, "v1=1")
	assert.Contains(t, redirect, "v2=2")

	// The redirect target itself must be stable, or the browser loops.
	settled := provider(httptest.NewRequest("GET", redirect, nil))
	_, loops := settled["_redirect"]
	assert.False(t, loops, "the resolved URL must not redirect again")
}

// A single-version resource resolves both sides to the same number, which is the
// one case where the fill-in could produce the URL it was invoked on.
func TestCompareContextProvider_SingleVersionDoesNotLoop(t *testing.T) {
	tc := SetupTestEnv(t)

	res := newCompareResource(t, tc, "Single Version", "compare-single-version")

	provider := template_context_providers.CompareContextProvider(tc.AppCtx)
	ctx := provider(httptest.NewRequest("GET", fmt.Sprintf("/resource/compare?r1=%d", res.ID), nil))

	redirect, ok := ctx["_redirect"].(string)
	if !ok {
		return
	}
	settled := provider(httptest.NewRequest("GET", redirect, nil))
	_, loops := settled["_redirect"]
	assert.False(t, loops, "the resolved URL must not redirect again")
}

// Choosing the comparator from the left-hand version alone sent a JSON-versus-PNG
// comparison to the text diff, which fetched the image in full and printed its
// bytes as added lines.
func TestCompareContextProvider_MismatchedTypesUseTheBinaryPanel(t *testing.T) {
	tc := SetupTestEnv(t)

	textRes := newCompareResource(t, tc, "Config", "{\"a\":1}")
	addCompareVersion(t, tc, textRes.ID, 2, "application/json", "")

	imageRes := newCompareResource(t, tc, "Pixel", "pretend-png-bytes")
	addCompareVersion(t, tc, imageRes.ID, 2, "image/png", "")

	provider := template_context_providers.CompareContextProvider(tc.AppCtx)

	mixed := fmt.Sprintf("/resource/compare?r1=%d&v1=2&r2=%d&v2=2", textRes.ID, imageRes.ID)
	ctx := provider(httptest.NewRequest("GET", mixed, nil))
	assert.Equal(t, "binary", ctx["contentCategory"],
		"two different categories have no side-by-side rendering; the type change is the difference")

	matchedImages := fmt.Sprintf("/resource/compare?r1=%d&v1=2&r2=%d&v2=2", imageRes.ID, imageRes.ID)
	ctx = provider(httptest.NewRequest("GET", matchedImages, nil))
	assert.Equal(t, "image", ctx["contentCategory"])
}

// A version and a date alone cannot tell two uploads on the same day apart, and
// they drop the comment somebody wrote so the versions could be told apart.
func TestCompareContextProvider_VersionOptionsCarryTheirDetail(t *testing.T) {
	tc := SetupTestEnv(t)

	res := newCompareResource(t, tc, "Options", "option-label-v1")
	addCompareVersion(t, tc, res.ID, 2, "text/plain", "tightened the schema")

	provider := template_context_providers.CompareContextProvider(tc.AppCtx)
	ctx := provider(httptest.NewRequest("GET", fmt.Sprintf("/resource/compare?r1=%d&v1=1&v2=2", res.ID), nil))

	options, ok := ctx["versions1"].([]template_context_providers.CompareVersionOption)
	assert.True(t, ok, "version options should be published as CompareVersionOption values")
	assert.Len(t, options, 2)

	var current template_context_providers.CompareVersionOption
	for _, option := range options {
		if option.IsCurrent {
			current = option
		}
	}
	assert.Equal(t, 2, current.VersionNumber)
	assert.Contains(t, current.Label, "current")
	assert.Contains(t, current.Label, "tightened the schema")

	for _, option := range options {
		assert.True(t, strings.HasPrefix(option.Label, fmt.Sprintf("v%d", option.VersionNumber)),
			"every option should lead with its version number, got %q", option.Label)
	}
}

// Pane headers name the version when both sides belong to one resource, because
// the side label alone ("Older", "Current") does not say which version that is.
func TestCompareContextProvider_PanelTitlesNameTheVersion(t *testing.T) {
	tc := SetupTestEnv(t)

	res := newCompareResource(t, tc, "Panel Source", "panel-title-v1")
	addCompareVersion(t, tc, res.ID, 2, "text/plain", "")

	provider := template_context_providers.CompareContextProvider(tc.AppCtx)
	ctx := provider(httptest.NewRequest("GET", fmt.Sprintf("/resource/compare?r1=%d&v1=1&v2=2", res.ID), nil))

	assert.Contains(t, ctx["panelTitle1"], "v1")
	assert.Contains(t, ctx["panelTitle2"], "v2")

	// Across two resources the name is the whole label, with no version suffix.
	other := newCompareResource(t, tc, "Panel Other", "panel-other-v1")
	cross := fmt.Sprintf("/resource/compare?r1=%d&v1=1&r2=%d&v2=1", res.ID, other.ID)
	ctx = provider(httptest.NewRequest("GET", cross, nil))
	assert.Equal(t, "Panel Source", ctx["panelTitle1"])
	assert.Equal(t, "Panel Other", ctx["panelTitle2"])
}

// A resource whose versions have not been migrated has no version rows at all;
// GetVersions synthesises a v1 for it so a version panel has something to show.
// That row has no id, so no file route can serve it and no comparison can load
// it — defaulting to it redirected straight to "version 1 not found".
func TestCompareContextProvider_UnmigratedResourceHasNothingToCompare(t *testing.T) {
	tc := SetupTestEnv(t)

	// Inserted directly: AddResource writes an initial version row, which is the
	// state this test needs the absence of.
	legacy := models.Resource{
		Name:        "Never Migrated",
		Hash:        "compare-unmigrated-hash",
		HashType:    "SHA1",
		FileSize:    128,
		ContentType: "text/plain",
		Location:    "/fake/unmigrated",
	}
	assert.NoError(t, tc.DB.Create(&legacy).Error)

	provider := template_context_providers.CompareContextProvider(tc.AppCtx)
	ctx := provider(httptest.NewRequest("GET", fmt.Sprintf("/resource/compare?r1=%d", legacy.ID), nil))

	_, redirects := ctx["_redirect"]
	assert.False(t, redirects, "there is no version to redirect to")
	assert.Nil(t, ctx["errorMessage"], "a resource with no version history is not an error")
	assert.Nil(t, ctx["comparison"])

	options, ok := ctx["versions1"].([]template_context_providers.CompareVersionOption)
	assert.True(t, ok)
	assert.Empty(t, options, "a version no route can serve must not be offered")

	assert.NotEmpty(t, ctx["compareUnavailableReason"],
		"the empty state has to say why there is nothing to pick")

	// The same holds with the unmigrated resource on the far side of a
	// cross-resource comparison, which redirected into the error page too.
	other := newCompareResource(t, tc, "Migrated", "compare-unmigrated-other")
	cross := fmt.Sprintf("/resource/compare?r1=%d&r2=%d", other.ID, legacy.ID)
	ctx = provider(httptest.NewRequest("GET", cross, nil))
	_, redirects = ctx["_redirect"]
	assert.False(t, redirects, "half a comparison is not a comparison")
	assert.Nil(t, ctx["errorMessage"])
	assert.NotEmpty(t, ctx["compareUnavailableReason"])
}

// The route is also served as `.json` and `.body`. A redirect built from a
// literal path answered a client that asked for one of those with a page.
func TestCompareContextProvider_RedirectKeepsTheRequestedSuffix(t *testing.T) {
	tc := SetupTestEnv(t)

	res := newCompareResource(t, tc, "Suffix Carrier", "compare-suffix-v1")
	addCompareVersion(t, tc, res.ID, 2, "text/plain", "")

	provider := template_context_providers.CompareContextProvider(tc.AppCtx)

	for _, suffix := range []string{"", ".json", ".body"} {
		path := fmt.Sprintf("/resource/compare%s?r1=%d", suffix, res.ID)
		ctx := provider(httptest.NewRequest("GET", path, nil))

		redirect, ok := ctx["_redirect"].(string)
		assert.True(t, ok, "%s should redirect", path)
		assert.True(t, strings.HasPrefix(redirect, "/resource/compare"+suffix+"?"),
			"redirect from %s should stay on that path, got %q", path, redirect)
		// Relative, so a forwarded Host header cannot steer it off-site.
		assert.False(t, strings.Contains(redirect, "//"), "redirect should be relative, got %q", redirect)

		settled := provider(httptest.NewRequest("GET", redirect, nil))
		_, loops := settled["_redirect"]
		assert.False(t, loops, "the resolved URL must not redirect again")
	}
}

// A merge renumbers the loser's history above the winner's highest and leaves
// the winner's own current version where it was, so a winner whose current
// version is v1 can hold v2 and v3 as history. Answering "nothing is earlier"
// there defaulted the page to comparing v1 with itself.
func TestCompareContextProvider_CurrentIsLowestStillFindsAPartner(t *testing.T) {
	tc := SetupTestEnv(t)

	res := newCompareResource(t, tc, "Merge Winner", "compare-merge-winner-v1")

	// The versions a merge transfers: numbered above, and not current.
	for _, number := range []int{2, 3} {
		version := models.ResourceVersion{
			ResourceID:    res.ID,
			VersionNumber: number,
			Hash:          fmt.Sprintf("compare-transferred-%d", number),
			HashType:      "SHA1",
			FileSize:      int64(50 * number),
			ContentType:   "text/plain",
			Location:      fmt.Sprintf("/fake/transferred/v%d", number),
			Comment:       "Merged from: Loser",
		}
		assert.NoError(t, tc.DB.Create(&version).Error)
	}

	provider := template_context_providers.CompareContextProvider(tc.AppCtx)
	ctx := provider(httptest.NewRequest("GET", fmt.Sprintf("/resource/compare?r1=%d", res.ID), nil))

	redirect, ok := ctx["_redirect"].(string)
	assert.True(t, ok, "a resource with three versions has something to compare")
	assert.Contains(t, redirect, "v2=1", "the current version stays on the right")
	assert.Contains(t, redirect, "v1=2", "the nearest other version is the partner, not v1 again")

	settled := provider(httptest.NewRequest("GET", redirect, nil))
	_, loops := settled["_redirect"]
	assert.False(t, loops)
	assert.False(t, settled["sameVersion"].(bool), "the two sides must not name one version")
}

// A URL naming one version and leaving the other empty filled the empty side
// with the current version — and when the one named was the current version,
// that produced a comparison of it with itself.
func TestCompareContextProvider_NamingTheCurrentVersionStillFindsAPartner(t *testing.T) {
	tc := SetupTestEnv(t)

	res := newCompareResource(t, tc, "Partner Search", "compare-partner-v1")
	addCompareVersion(t, tc, res.ID, 2, "text/plain", "")
	addCompareVersion(t, tc, res.ID, 3, "text/plain", "")

	provider := template_context_providers.CompareContextProvider(tc.AppCtx)
	ctx := provider(httptest.NewRequest("GET", fmt.Sprintf("/resource/compare?r1=%d&v1=3", res.ID), nil))

	redirect, ok := ctx["_redirect"].(string)
	assert.True(t, ok, "the empty side has to be filled in")
	assert.Contains(t, redirect, "v1=3", "the version the URL named stays where it was put")
	assert.Contains(t, redirect, "v2=2", "the empty side takes the nearest other version, not v3 again")

	settled := provider(httptest.NewRequest("GET", redirect, nil))
	_, loops := settled["_redirect"]
	assert.False(t, loops)
	assert.False(t, settled["sameVersion"].(bool))
}
