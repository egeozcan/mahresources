package application_context

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrToolSupportUnknown is returned beside a true answer when the provider
// could not say whether a model supports Tools. It never refuses a run.
var ErrToolSupportUnknown = errors.New("tool support could not be determined")

const (
	openRouterToolsCatalogueURL = "https://openrouter.ai/api/v1/models?supported_parameters=tools"
	openRouterFullCatalogueURL  = "https://openrouter.ai/api/v1/models"
)

// Variables so tests can shorten them; production values are the plan's.
var (
	openRouterCatalogueTimeout = 5 * time.Second
	openRouterCatalogueTTL     = 10 * time.Minute
	openRouterCatalogueNow     = time.Now
)

type openRouterCatalogueEntry struct {
	set map[string]bool
	at  time.Time
}

// The catalogue is public and identical for every account, so one process-wide
// cache serves all of them. Only successful fetches are cached.
var openRouterCatalogue struct {
	sync.Mutex
	tools, all *openRouterCatalogueEntry
}

func resetOpenRouterCatalogue() {
	openRouterCatalogue.Lock()
	openRouterCatalogue.tools, openRouterCatalogue.all = nil, nil
	openRouterCatalogue.Unlock()
}

// SupportsTools reads OpenRouter's model metadata. A model the tools-filtered
// list names supports them; one that only the full list names does not; one in
// neither (a rolling alias, a new ID) or any lookup failure is unknown.
func (p *generationConversationProvider) SupportsTools(ctx context.Context, model string) (bool, error) {
	if p.transport.provider != "openrouter" {
		return true, nil
	}
	withTools, err := p.catalogue(ctx, openRouterToolsCatalogueURL, true)
	if err != nil {
		return true, err
	}
	if withTools.set[model] {
		return true, nil
	}
	all, err := p.catalogue(ctx, openRouterFullCatalogueURL, false)
	if err != nil {
		return true, err
	}
	if all.set[model] {
		return false, nil
	}
	return true, ErrToolSupportUnknown
}

func (p *generationConversationProvider) catalogue(ctx context.Context, url string, tools bool) (*openRouterCatalogueEntry, error) {
	slot := func() **openRouterCatalogueEntry {
		if tools {
			return &openRouterCatalogue.tools
		}
		return &openRouterCatalogue.all
	}
	openRouterCatalogue.Lock()
	cached := *slot()
	openRouterCatalogue.Unlock()
	if cached != nil && openRouterCatalogueNow().Sub(cached.at) < openRouterCatalogueTTL {
		return cached, nil
	}
	ctx, cancel := context.WithTimeout(ctx, openRouterCatalogueTimeout)
	defer cancel()
	var decoded struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	// A plain GET: the catalogue is public, so the API key is not sent.
	if err := p.transport.get(ctx, url, &decoded); err != nil {
		return nil, err
	}
	entry := &openRouterCatalogueEntry{set: make(map[string]bool, len(decoded.Data)), at: openRouterCatalogueNow()}
	for _, m := range decoded.Data {
		entry.set[m.ID] = true
	}
	openRouterCatalogue.Lock()
	*slot() = entry
	openRouterCatalogue.Unlock()
	return entry, nil
}
