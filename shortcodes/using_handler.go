package shortcodes

import (
	"context"
	"math"
	"strconv"
	"strings"
)

// usingSources are the attributes that name a [using] block's target. Exactly
// one must be present.
var usingSources = []string{"id", "id-path", "id-prop", "rel", "query"}

// usingTypes are the entity types a [using] block can switch to: the ones with
// Meta, a MetaSchema and an owner chain, which are also the only ones MRQL
// knows. The type is whitelisted because it is the one piece of the lookup query
// that is not a bound parameter.
var usingTypes = map[string]bool{"group": true, "resource": true, "note": true}

// usingLookupQuery is the query every ID source runs. The ID is always bound as
// $id, never written into the text.
func usingLookupQuery(entityType string) string {
	return "type = " + entityType + " AND id = $id"
}

// RenderUsingShortcode expands a [using]…[else]…[/using] block. It loads one
// group, resource or note and renders the body with that entity as the context,
// so every shortcode inside reads the target rather than the carrier. The [else]
// branch renders, with the carrier's context, when there is no target.
//
// The lookup goes through the QueryExecutor rather than a direct read, which is
// what makes it safe: the executor runs as the request's principal, so a
// group-confined user gets an empty result for an entity outside their subtree,
// and the page query budget and its cache count the lookup like any [mrql]. A
// target that does not exist and one the viewer may not see both render [else],
// so the block cannot be used to probe for IDs.
func RenderUsingShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if !sc.IsBlock {
		return shortcodeErrorMarker("using", "[using] requires a closing [/using] tag")
	}

	body, elseBranch := SplitElse(sc.InnerContent)

	renderElse := func() string {
		if strings.TrimSpace(elseBranch) == "" {
			return ""
		}
		return processWithDepth(reqCtx, elseBranch, ctx, renderer, executor, depth+1)
	}

	query, opts, found, problem := resolveUsingTarget(sc.Attrs, ctx)
	if problem != "" {
		return shortcodeErrorMarker("using", problem)
	}
	if !found {
		return renderElse()
	}

	result, err := executor(reqCtx, query, opts)
	if err != nil {
		return mrqlErrorHTML(err, false)
	}
	if result == nil {
		return renderElse()
	}
	if result.Mode != "" && result.Mode != "flat" {
		return shortcodeErrorMarker("using", "[using] query must return entities; GROUP BY returns buckets")
	}
	if len(result.Items) == 0 {
		return renderElse()
	}

	child := contextFromItem(result.Items[0], ctx)
	outer := ctx
	child.outer = &outer
	return processWithDepth(reqCtx, body, child, renderer, executor, depth+1)
}

// resolveUsingTarget turns a [using] block's attributes into the query that
// loads its target. found is false when the source resolves to no ID (a missing
// Meta value, an ownerless entity for rel=owner), which renders [else]. problem
// is non-empty for an attribute mistake the author has to fix, which renders an
// error marker instead.
func resolveUsingTarget(attrs map[string]string, ctx MetaShortcodeContext) (query string, opts QueryOptions, found bool, problem string) {
	source := ""
	for _, name := range usingSources {
		if strings.TrimSpace(attrs[name]) == "" {
			continue
		}
		if source != "" {
			return "", QueryOptions{}, false, "[using] takes exactly one of id, id-path, id-prop, rel or query"
		}
		source = name
	}
	if source == "" {
		return "", QueryOptions{}, false, "[using] needs one of id, id-path, id-prop, rel or query"
	}

	if source == "query" {
		return attrs["query"], QueryOptions{
			Params:       collectShortcodeParams(attrs),
			Limit:        1,
			ScopeGroupID: resolveScopeKeyword(attrs["scope"], ctx),
		}, true, ""
	}

	entityType := "group"
	if source != "rel" {
		entityType = strings.TrimSpace(attrs["type"])
		if !usingTypes[entityType] {
			return "", QueryOptions{}, false, `[using] type must be "group", "resource" or "note"`
		}
	}

	var id uint
	var ok bool
	switch source {
	case "id":
		id, ok = parseEntityID(strings.TrimSpace(attrs["id"]))
		if !ok {
			return "", QueryOptions{}, false, "[using] id must be a positive whole number"
		}
	case "id-path":
		id, ok = entityIDFromValue(ctx.rawValueAtPath(attrs["id-path"]))
	case "id-prop":
		id, ok = readUintField(ctx.Entity, strings.TrimSpace(attrs["id-prop"]))
		ok = ok && id > 0
	case "rel":
		id, ok = relGroupID(strings.TrimSpace(attrs["rel"]), ctx)
	}
	if !ok {
		return "", QueryOptions{}, false, ""
	}

	// Scope 0 is global. The [mrql] default scope is a convenience filter, not
	// authorization; an explicit reference must not come back empty because the
	// target lives outside the carrier's group. What the viewer may see is
	// still decided by the principal the executor runs as.
	return usingLookupQuery(entityType), QueryOptions{
		Params: map[string]string{"id": strconv.FormatUint(uint64(id), 10)},
		Limit:  1,
	}, true, ""
}

// parseEntityID accepts a positive base-10 integer that fits in a uint.
func parseEntityID(s string) (uint, bool) {
	n, err := strconv.ParseUint(s, 10, 0)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint(n), true
}

// entityIDFromValue reads an entity ID out of a decoded Meta value: a JSON
// number with no fractional part, or a string holding one. Anything else —
// arrays included, since a [using] block names one entity — is no ID.
func entityIDFromValue(v any) (uint, bool) {
	switch x := v.(type) {
	case float64:
		// 2^53 is the largest integer a float64 holds exactly; beyond it the
		// value is not the ID that was written.
		if x < 1 || x != math.Trunc(x) || x > 1<<53 {
			return 0, false
		}
		return uint(x), true
	case string:
		return parseEntityID(strings.TrimSpace(x))
	}
	return 0, false
}
