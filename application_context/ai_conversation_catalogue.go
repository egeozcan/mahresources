package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
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
	// tools maps every listed model ID to whether its record says it supports
	// "tools". known is false for a record with no supported_parameters list.
	tools map[string]bool
	known map[string]bool
	at    time.Time
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

// SupportsTools reads OpenRouter's model metadata. The tools-filtered list is
// the quick positive answer. A model it does not name is looked up in the full
// catalogue, and refused only when that record exists and its
// supported_parameters lacks "tools": the two lists are cached separately, so
// the filtered one can be older than the full one. A model in neither (a
// rolling alias, a new ID) or any lookup failure is unknown, never a refusal.
// One timeout covers the whole lookup, both fetches included.
func (p *generationConversationProvider) SupportsTools(ctx context.Context, model string) (bool, error) {
	if p.transport.provider != "openrouter" {
		return true, nil
	}
	ctx, cancel := context.WithTimeout(ctx, openRouterCatalogueTimeout)
	defer cancel()
	withTools, err := p.catalogue(ctx, openRouterToolsCatalogueURL, true)
	if err != nil {
		return true, err
	}
	if withTools.tools[model] {
		return true, nil
	}
	all, err := p.catalogue(ctx, openRouterFullCatalogueURL, false)
	if err != nil {
		return true, err
	}
	if _, listed := all.tools[model]; !listed || !all.known[model] {
		return true, ErrToolSupportUnknown
	}
	return all.tools[model], nil
}

func (p *generationConversationProvider) catalogue(ctx context.Context, url string, filtered bool) (*openRouterCatalogueEntry, error) {
	slot := func() **openRouterCatalogueEntry {
		if filtered {
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
	var decoded struct {
		Error json.RawMessage `json:"error"`
		Data  json.RawMessage `json:"data"`
	}
	// A plain GET: the catalogue is public, so the API key is not sent.
	if err := p.transport.get(ctx, url, &decoded); err != nil {
		return nil, err
	}
	// An envelope that is not a list of models (HTTP 200 with {}, null or an
	// error object) must not become an empty catalogue.
	var models []struct {
		ID                  string    `json:"id"`
		SupportedParameters *[]string `json:"supported_parameters"`
	}
	if nonEmptyRaw(decoded.Error) || !nonEmptyRaw(decoded.Data) || json.Unmarshal(decoded.Data, &models) != nil {
		return nil, newGenerationFailure("provider returned invalid response JSON", GenerationErrorClassDecode, 0)
	}
	entry := &openRouterCatalogueEntry{tools: make(map[string]bool, len(models)), known: make(map[string]bool, len(models)), at: openRouterCatalogueNow()}
	for _, m := range models {
		hasList := m.SupportedParameters != nil
		hasTools := hasList && slices.Contains(*m.SupportedParameters, "tools")
		// The filtered list names only tool-capable models, whatever a record
		// carries; the full list's record is the authority for a refusal.
		entry.known[m.ID] = hasList || filtered
		entry.tools[m.ID] = hasTools || filtered
	}
	openRouterCatalogue.Lock()
	*slot() = entry
	openRouterCatalogue.Unlock()
	return entry, nil
}
