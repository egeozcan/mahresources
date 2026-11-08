package arch

import (
	"regexp"
	"testing"
)

// The generate and preview routes read entity values (samples, the previewed
// entity, [mrql] rows) on the caller's behalf, so they must run against the
// request-scoped context. Mounted on the bare context, a principal confined to
// a subtree reads entities outside it through them. The path prefixes, and so
// the role gate, stay as they are.
func TestTemplateGenerationRoutesRunScoped(t *testing.T) {
	routes := readRepoFile(t, "server/routes.go")
	carriers := map[string]string{"category": "group", "resourceCategory": "resource", "noteType": "note"}
	for carrier, entityType := range carriers {
		for _, route := range []struct{ path, handler string }{
			{"generateTemplate", "GetGenerateTemplateHandler"},
			{"previewTemplate", "GetPreviewTemplateHandler"},
		} {
			want := regexp.MustCompile(`Path\("/v1/` + carrier + `/` + route.path + `"\)\.HandlerFunc\(scopedAPI\(appContext, func\([^)]*\) func\(http\.ResponseWriter, \*http\.Request\) \{\s*return api_handlers\.` + route.handler + `\([a-zA-Z]+, "` + entityType + `"[,)]`)
			if !want.MatchString(routes) {
				t.Errorf("/v1/%s/%s is not mounted through scopedAPI with the %q carrier", carrier, route.path, entityType)
			}
			bare := regexp.MustCompile(`Path\("/v1/` + carrier + `/` + route.path + `"\)\.HandlerFunc\(api_handlers\.`)
			if bare.MatchString(routes) {
				t.Errorf("/v1/%s/%s is mounted on the unscoped context", carrier, route.path)
			}
		}
	}
}

// The finish route must sit under each carrier's prefix, so it inherits the
// generate route's role gate (admin for category and resourceCategory, editor
// for noteType) instead of falling through to the default branch of the policy.
func TestTemplateRunFinishRoutesAreMountedUnderTheCarrierPrefix(t *testing.T) {
	routes := readRepoFile(t, "server/routes.go")
	if !regexp.MustCompile(`for _, carrier := range \[\]string\{"category", "resourceCategory", "noteType"\} \{\s*router\.Methods\(http\.MethodPost\)\.Path\("/v1/" \+ carrier \+ "/generateTemplate/finish"\)\.HandlerFunc\(api_handlers\.GetFinishTemplateRunHandler\(\)\)`).MatchString(routes) {
		t.Error("the finish route is not mounted under /v1/{category,resourceCategory,noteType}/generateTemplate/finish")
	}
}
