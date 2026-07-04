package template_filters

import (
	"mahresources/application_context"
	"mahresources/shortcodes"
)

// BuildPartialResolver returns a shortcodes.PartialResolver backed by the
// application context, with a small in-closure cache so a page rendering many
// cards resolves each partial name from the DB at most once. Build it once per
// page render and attach it to the request context with
// shortcodes.WithPartialResolver, mirroring how the MRQL render cache is
// threaded. The cache holds nil for names that don't resolve, so repeated
// misses don't re-hit the DB. A page render is single-goroutine, so the plain
// map needs no locking.
func BuildPartialResolver(appCtx *application_context.MahresourcesContext) shortcodes.PartialResolver {
	if appCtx == nil {
		return nil
	}
	cache := map[string]*string{}
	return func(name string) (string, bool) {
		if cached, ok := cache[name]; ok {
			if cached == nil {
				return "", false
			}
			return *cached, true
		}
		partial, err := appCtx.GetTemplatePartialByName(name)
		if err != nil || partial == nil {
			cache[name] = nil
			return "", false
		}
		content := partial.Content
		cache[name] = &content
		return content, true
	}
}
