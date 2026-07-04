package shortcodes

import (
	"context"
	"fmt"
	"html"
	"reflect"
)

// unresolvedScopeSentinel mirrors mrql.UnresolvedScopeSentinel — the scope ID
// callers stamp when an entity's owning/parent/root group could not be
// resolved. [link] never emits a link to this sentinel. Kept as a local
// constant so the leaf shortcodes package stays free of an mrql import.
const unresolvedScopeSentinel = ^uint(0) >> 1

// RenderLinkShortcode expands a [link] shortcode into a detail-page URL.
// Inline form ([link to="…"]) renders just the HTML-escaped URL, so authors can
// write <a href="[link]" class="…">. Block form ([link]inner[/link]) renders a
// full <a href="URL">processed inner</a>. When the target cannot be resolved
// (unknown to=, unset category, or an unresolved scope sentinel), the inline
// form renders nothing and the block form renders its processed inner without a
// wrapping anchor.
func RenderLinkShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	url, ok := resolveLinkURL(sc.Attrs["to"], ctx)

	if sc.IsBlock {
		inner := processWithDepth(reqCtx, sc.InnerContent, ctx, renderer, executor, depth+1)
		if !ok {
			return inner
		}
		return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(url), inner)
	}

	if !ok {
		return ""
	}
	return html.EscapeString(url)
}

// resolveLinkURL maps the to= target to a detail-page URL using the entity
// context. Returns ok=false when no valid target exists.
func resolveLinkURL(to string, ctx MetaShortcodeContext) (string, bool) {
	switch to {
	case "", "self":
		switch ctx.EntityType {
		case "group":
			return fmt.Sprintf("/group?id=%d", ctx.EntityID), true
		case "resource":
			return fmt.Sprintf("/resource?id=%d", ctx.EntityID), true
		case "note":
			return fmt.Sprintf("/note?id=%d", ctx.EntityID), true
		}
		return "", false

	case "owner":
		var gid uint
		if ctx.EntityType == "group" {
			gid = ctx.ParentGroupID
		} else {
			gid = ctx.ScopeGroupID
		}
		return groupURL(gid)

	case "root":
		return groupURL(ctx.RootGroupID)

	case "category":
		return resolveCategoryURL(ctx)
	}
	return "", false
}

// groupURL builds a /group?id= URL for a resolved group ID, rejecting the
// zero and unresolved-sentinel values.
func groupURL(gid uint) (string, bool) {
	if gid == 0 || gid == unresolvedScopeSentinel {
		return "", false
	}
	return fmt.Sprintf("/group?id=%d", gid), true
}

// resolveCategoryURL reads the entity's category/type ID via reflection and
// builds the carrier's detail-page URL.
func resolveCategoryURL(ctx MetaShortcodeContext) (string, bool) {
	var field, path string
	switch ctx.EntityType {
	case "group":
		field, path = "CategoryId", "/category?id=%d"
	case "resource":
		field, path = "ResourceCategoryId", "/resourceCategory?id=%d"
	case "note":
		field, path = "NoteTypeId", "/noteType?id=%d"
	default:
		return "", false
	}
	id, ok := readUintField(ctx.Entity, field)
	if !ok || id == 0 {
		return "", false
	}
	return fmt.Sprintf(path, id), true
}

// readUintField returns the uint value of a named field on entity, following a
// pointer field or a nil-safe pointer entity. Handles both *uint and plain uint
// carrier fields.
func readUintField(entity any, name string) (uint, bool) {
	if entity == nil {
		return 0, false
	}
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return 0, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return 0, false
	}
	f := v.FieldByName(name)
	if !f.IsValid() {
		return 0, false
	}
	if f.Kind() == reflect.Ptr {
		if f.IsNil() {
			return 0, false
		}
		f = f.Elem()
	}
	switch f.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return uint(f.Uint()), true
	default:
		return 0, false
	}
}
