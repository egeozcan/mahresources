package shortcodes

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"strconv"
	"strings"
)

// The two [relations] directions: out lists the edges whose from_group_id is
// the carrier, in the edges whose to_group_id is.
const (
	RelationsDirectionOut = "out"
	RelationsDirectionIn  = "in"
)

// MaxRelationsLimit caps limit=; defaultRelationsLimit is the unset value.
const (
	MaxRelationsLimit     = 100
	defaultRelationsLimit = 20
)

// RelationsRequest asks for one group's typed relations in one direction.
type RelationsRequest struct {
	GroupID   uint
	Type      string // exact GroupRelationType name; "" is every type
	Direction string // RelationsDirectionOut or RelationsDirectionIn
	Limit     int    // 1..MaxRelationsLimit
	// Details asks for the far group's Description and Meta too. Only a block
	// body with an item path into those fields needs them, and they cost a
	// statement of their own.
	Details bool
}

// GroupRelationEntry is one edge as the carrier sees it: the relation, its
// type, and the group at the far end.
type GroupRelationEntry struct {
	Direction     string
	Type          string
	Name          string
	Description   string
	GroupID       uint
	GroupName     string
	GroupCategory string
	// Set only when the request asked for Details.
	GroupDescription string
	GroupMeta        json.RawMessage
}

// RelationLister lists a group's relations as the request's principal sees
// them, ordered by type name and then edge id. It is injected per render with
// WithRelationLister, like PartialResolver, so this package stays free of DB
// imports. A surface that installs none renders [relations] as an
// "unavailable" comment.
type RelationLister func(ctx context.Context, req RelationsRequest) ([]GroupRelationEntry, error)

// ErrRelationsBudget is returned, wrapped with a message naming the limit, by
// a lister whose page query budget refused a statement.
var ErrRelationsBudget = errors.New("relations: budget")

// ErrRelationsUnavailable is returned by a lister whose handle cannot confine
// the viewer, such as a subtree-limited principal bound for MRQL only. It
// renders the same comment as a surface with no lister at all.
var ErrRelationsUnavailable = errors.New("relations: unavailable for this principal binding")

type relationListerKey struct{}

// WithRelationLister returns a context carrying l for [relations]. A nil l
// makes [relations] render its "unavailable" comment.
func WithRelationLister(ctx context.Context, l RelationLister) context.Context {
	return context.WithValue(ctx, relationListerKey{}, l)
}

func relationListerFrom(ctx context.Context) RelationLister {
	if ctx == nil {
		return nil
	}
	l, _ := ctx.Value(relationListerKey{}).(RelationLister)
	return l
}

const relationsUnavailable = "relations unavailable in this context"

// RenderRelationsShortcode renders [relations]: the carrier group's typed
// relations as nested lists, one per relation type, each entry a link to the
// group at the far end. With a block body each entry renders the body instead,
// against the carrier's context, with the edge's values spliced in at its
// [item] tokens as [each] does. No visible edge renders the [else] branch, or
// nothing.
func RenderRelationsShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	list := relationListerFrom(reqCtx)
	if list == nil {
		return shortcodeComment(relationsUnavailable)
	}
	// A <button> takes phrasing content only; nested lists are not.
	if withinReloadFace(reqCtx) {
		return shortcodeErrorMarker("relations", "[relations] cannot be used inside a [reload] button face")
	}
	direction := RelationsDirectionOut
	if raw, ok := sc.Attrs["direction"]; ok {
		direction = strings.TrimSpace(raw)
		if direction != RelationsDirectionOut && direction != RelationsDirectionIn {
			return shortcodeErrorMarker("relations", `direction must be "out" or "in"`)
		}
	}
	if ctx.EntityType != "group" || ctx.EntityID == 0 {
		return shortcodeComment("relations: only groups have relations")
	}

	req := RelationsRequest{
		GroupID:   ctx.EntityID,
		Type:      strings.TrimSpace(sc.Attrs["type"]),
		Direction: direction,
		Limit:     min(parseIntAttr(sc.Attrs["limit"], defaultRelationsLimit), MaxRelationsLimit),
	}

	m := meterFrom(reqCtx)
	var marked, elseBranch string
	var items []Shortcode
	if sc.IsBlock {
		var itemBranch string
		itemBranch, elseBranch = splitElseM(m, sc.InnerContent)
		marked, items = markItems(m, itemBranch)
		if m.Err() != nil {
			return ""
		}
		req.Details = readsFarGroupDetails(items)
	}

	entries, err := list(reqCtx, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrRelationsUnavailable):
			return shortcodeComment(relationsUnavailable)
		case errors.Is(err, ErrRelationsBudget):
			return shortcodeErrorMarker("relations", strings.TrimPrefix(err.Error(), ErrRelationsBudget.Error()+": "))
		case errors.Is(err, context.DeadlineExceeded):
			return shortcodeErrorMarker("relations", "lookup timed out")
		default:
			// The lister logged the full error; SQL text stays off the page.
			return shortcodeErrorMarker("relations", "could not load relations")
		}
	}
	if !m.alive() {
		return ""
	}
	if len(entries) == 0 {
		if strings.TrimSpace(elseBranch) == "" {
			return ""
		}
		return processWithDepth(reqCtx, elseBranch, ctx, renderer, executor, depth+1)
	}

	tooltip := withinTooltip(reqCtx)
	var b strings.Builder
	b.WriteString(`<div class="relations-shortcode" data-direction="`)
	b.WriteString(direction)
	b.WriteString(`"><ul class="relations-shortcode__types" role="list">`)
	for i, e := range entries {
		if i == 0 || e.Direction != entries[i-1].Direction || e.Type != entries[i-1].Type {
			if i > 0 {
				b.WriteString(`</ul></li>`)
			}
			b.WriteString(`<li class="relations-shortcode__type"><span class="relations-shortcode__label">`)
			b.WriteString(html.EscapeString(relationsLabel(e)))
			b.WriteString(`</span><ul class="relations-shortcode__list" role="list">`)
		}
		b.WriteString(`<li class="relations-shortcode__entry">`)
		if sc.IsBlock {
			if !m.alive() {
				return ""
			}
			rendered := processWithDepth(reqCtx, marked, ctx, renderer, executor, depth+1)
			if m.Err() != nil {
				return ""
			}
			elem := relationElement(e, req.Details)
			spliced, ok := spliceLazy(m, rendered, len(items), func(idx int) string {
				switch items[idx].Name {
				case "calc":
					return stripNUL(renderCalc(ctx, items[idx], elem, true))
				case "inspect":
					if i != 0 {
						return ""
					}
					return renderInspectGated(reqCtx, items[idx], ctx, elem, true)
				}
				return stripNUL(renderItemValueObserved(ctx, items[idx], elem, i+1))
			})
			if !ok {
				return ""
			}
			b.WriteString(spliced)
		} else {
			name := html.EscapeString(relationsGroupName(e))
			if tooltip {
				// A hover card is a tooltip the keyboard cannot enter, so a link
				// there could never be reached by it.
				b.WriteString(name)
			} else {
				b.WriteString(`<a href="/group?id=`)
				b.WriteString(strconv.FormatUint(uint64(e.GroupID), 10))
				b.WriteString(`">`)
				b.WriteString(name)
				b.WriteString(`</a>`)
			}
			if e.GroupCategory != "" {
				b.WriteString(` <span class="relations-shortcode__category">(`)
				b.WriteString(html.EscapeString(e.GroupCategory))
				b.WriteString(`)</span>`)
			}
		}
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul></li></ul></div>`)
	return b.String()
}

// relationsLabel is the visible label of a type run. Direction is spelled out
// rather than left to styling.
func relationsLabel(e GroupRelationEntry) string {
	label := e.Type
	if strings.TrimSpace(label) == "" {
		label = "Relation"
	}
	if e.Direction == RelationsDirectionIn {
		return "Incoming: " + label
	}
	return label
}

// relationsGroupName keeps a link to a blank-named group from having no
// accessible name.
func relationsGroupName(e GroupRelationEntry) string {
	if strings.TrimSpace(e.GroupName) == "" {
		return "Group " + strconv.FormatUint(uint64(e.GroupID), 10)
	}
	return e.GroupName
}

// relationElement is the value [item] reads for one edge. GroupID is a float64
// because the [item] formatter expects JSON-decoded scalars.
func relationElement(e GroupRelationEntry, details bool) map[string]any {
	elem := map[string]any{
		"Name":          e.Name,
		"Description":   e.Description,
		"Type":          e.Type,
		"Direction":     e.Direction,
		"GroupID":       float64(e.GroupID),
		"GroupName":     e.GroupName,
		"GroupCategory": e.GroupCategory,
		"GroupURL":      "/group?id=" + strconv.FormatUint(uint64(e.GroupID), 10),
	}
	if details {
		elem["GroupDescription"] = e.GroupDescription
		var meta any
		if len(e.GroupMeta) > 0 && json.Unmarshal(e.GroupMeta, &meta) == nil {
			elem["GroupMeta"] = meta
		}
	}
	return elem
}

func readsFarGroupDetails(items []Shortcode) bool {
	for _, sc := range items {
		switch sc.Name {
		case "item":
			if sc.Attrs["index"] != "true" && isFarGroupDetailPath(sc.Attrs["path"]) {
				return true
			}
		case "calc":
			prog, err := parseCalcExpr(sc.Attrs["expr"])
			if err != nil {
				continue
			}
			for _, ref := range prog.refs {
				if ref.root == "item" && isFarGroupDetailPath(ref.path) {
					return true
				}
			}
		}
	}
	return false
}

func isFarGroupDetailPath(path string) bool {
	field, _, _ := strings.Cut(path, ".")
	return field == "GroupDescription" || field == "GroupMeta"
}
