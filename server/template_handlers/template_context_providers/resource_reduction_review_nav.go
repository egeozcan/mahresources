package template_context_providers

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/flosch/pongo2/v4"

	"mahresources/models"
)

// reviewNav builds the links of the review page's own navigation: the status
// tabs, the filter chips and the hidden fields of the sort form.
//
// In Go rather than in the template because every one of them is "the current
// query with one thing changed", and pongo2 cannot take a query string apart. A
// link never carries `page`: changing what is shown starts from the first page.
//
// Tabs keep the sort and the page size and drop every filter, since the tab counts
// ignore filters and "Skipped (3)" must not open onto a list the chips emptied.
// Chips keep everything else, the status tab included.
func reviewNav(values url.Values, reductionID uint, statusTab string, tabCounts map[string]int) pongo2.Context {
	id := strconv.FormatUint(uint64(reductionID), 10)

	// The parameters a link always carries. Lower-cased keys are the canonical
	// spelling; the decoder also accepts the capitalised ones older links used.
	get := func(key string) []string {
		var out []string
		for k, v := range values {
			if strings.EqualFold(k, key) {
				out = append(out, v...)
			}
		}
		return out
	}
	first := func(key string) string {
		if v := get(key); len(v) > 0 {
			return v[0]
		}
		return ""
	}

	base := func() url.Values {
		q := url.Values{}
		q.Set("id", id)
		for _, s := range get("status") {
			q.Add("status", s)
		}
		for _, t := range get("tier") {
			q.Add("tier", t)
		}
		if a := first("approval"); a != "" {
			q.Set("approval", a)
		}
		if first("attention") != "" {
			q.Set("attention", first("attention"))
		}
		if s := first("sort"); s != "" {
			q.Set("sort", s)
		}
		if p := first("pageSize"); p != "" {
			q.Set("pageSize", p)
		}
		return q
	}
	href := func(q url.Values) string { return "/reduction?" + q.Encode() }

	tabs := []pongo2.Context{}
	for _, tab := range []struct{ key, label string }{
		{models.ReductionClusterOpen, "Open"},
		{models.ReductionClusterSkipped, "Skipped"},
		{models.ReductionClusterStale, "Stale"},
		{models.ReductionClusterApplied, "Applied"},
	} {
		q := url.Values{}
		q.Set("id", id)
		q.Set("status", tab.key)
		if s := first("sort"); s != "" {
			q.Set("sort", s)
		}
		if p := first("pageSize"); p != "" {
			q.Set("pageSize", p)
		}
		tabs = append(tabs, pongo2.Context{
			"key":     tab.key,
			"label":   tab.label,
			"count":   tabCounts[tab.key],
			"href":    href(q),
			"current": tab.key == statusTab,
		})
	}

	tierActive := map[string]bool{}
	for _, t := range get("tier") {
		tierActive[t] = true
	}
	tiers := []pongo2.Context{}
	for _, tier := range []struct{ key, label string }{
		{models.ReductionTierIdentical, "Identical"},
		{models.ReductionTierNear, "Near-Identical"},
	} {
		q := base()
		q.Del("tier")
		for _, t := range []string{models.ReductionTierIdentical, models.ReductionTierNear} {
			if t == tier.key {
				if !tierActive[t] {
					q.Add("tier", t)
				}
			} else if tierActive[t] {
				q.Add("tier", t)
			}
		}
		tiers = append(tiers, pongo2.Context{"label": tier.label, "href": href(q), "active": tierActive[tier.key]})
	}

	current := normalizedApproval(first("approval"))
	approvals := []pongo2.Context{}
	for _, ap := range []struct{ key, label string }{{"approved", "Approved"}, {"unapproved", "Not approved"}} {
		q := base()
		q.Del("approval")
		if current != ap.key {
			q.Set("approval", ap.key)
		}
		approvals = append(approvals, pongo2.Context{"label": ap.label, "href": href(q), "active": current == ap.key})
	}

	attentionOn := first("attention") == "true" || first("attention") == "1" || first("attention") == "on"
	aq := base()
	aq.Del("attention")
	if !attentionOn {
		aq.Set("attention", "true")
	}

	// The sort form carries everything the select does not set.
	hidden := []pongo2.Context{{"name": "id", "value": id}}
	for _, s := range get("status") {
		hidden = append(hidden, pongo2.Context{"name": "status", "value": s})
	}
	for _, t := range get("tier") {
		hidden = append(hidden, pongo2.Context{"name": "tier", "value": t})
	}
	if current != "" {
		hidden = append(hidden, pongo2.Context{"name": "approval", "value": current})
	}
	if attentionOn {
		hidden = append(hidden, pongo2.Context{"name": "attention", "value": "true"})
	}

	clear := url.Values{}
	clear.Set("id", id)
	if statusTab != "" {
		clear.Set("status", statusTab)
	}
	if s := first("sort"); s != "" {
		clear.Set("sort", s)
	}
	if p := first("pageSize"); p != "" {
		clear.Set("pageSize", p)
	}

	// Where the bulk notice sends a reviewer who has Clusters needing a decision of
	// their own: the open Clusters that need attention, nothing else filtered.
	needs := url.Values{}
	needs.Set("id", id)
	needs.Set("status", models.ReductionClusterOpen)
	needs.Set("attention", "true")

	return pongo2.Context{
		"tabs":          tabs,
		"tiers":         tiers,
		"approvals":     approvals,
		"attention":     pongo2.Context{"label": "Needs attention", "href": href(aq), "active": attentionOn},
		"hidden":        hidden,
		"clearHref":     href(clear),
		"attentionHref": href(needs),
	}
}
