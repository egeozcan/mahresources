package template_filters

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flosch/pongo2/v4"
	"mahresources/models"
	"mahresources/models/types"
)

func init() {
	if err := pongo2.RegisterFilter("cardEntity", cardEntityFilter); err != nil {
		fmt.Println("error when registering cardEntity filter", err)
	}
}

// cardEntityFilter renders the entity a list card exposes as its Alpine `entity`
// scope, for a single-quoted data-entity attribute.
//
// A card used to carry the whole model: Meta, ownMeta, the series (with a second
// copy of the shared Meta) and the category with its schema and templates. On a
// list of resources with large metadata that was hundreds of kilobytes per card,
// sixteen megabytes for one page, all of it parsed by the browser and then again
// by JSON.parse before the page could respond.
//
// The full entity is a documented contract only for the category's own card
// templates: they can bind `entity.Meta.*` directly. So a card whose templates can
// read `entity` keeps it whole. Every other card gets the fields the list code
// itself reads (scalars, tags for Edit Tags, the category id for bulk-action
// filters, UpdatedAt for deferred shortcodes) with Meta left as an empty object.
//
// The JSON is escaped for the attribute rather than autoescaped: Go already
// writes <, > and & as \u escapes, so only the single quote needs an entity.
// Autoescape turned every double quote into six characters.
func cardEntityFilter(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	encoded, err := json.Marshal(cardEntityProjection(in.Interface()))
	if err != nil {
		return nil, &pongo2.Error{Sender: "filter:cardEntity", OrigError: err}
	}
	return pongo2.AsSafeValue(strings.ReplaceAll(string(encoded), "'", "&#39;")), nil
}

// cardTemplatesReadEntity reports whether text rendered into a card could reach
// the Alpine entity: directly, or through markup this filter cannot see, which a
// partial, a plugin shortcode or a raw=true value can all emit. The entity's own
// description counts too: the full-text group list renders it with shortcodes.
func cardTemplatesReadEntity(templates ...string) bool {
	for _, t := range templates {
		for _, marker := range []string{"entity", "[partial", "[plugin:", "raw="} {
			if strings.Contains(t, marker) {
				return true
			}
		}
	}
	return false
}

var emptyMeta = types.JSON("{}")

// cardRef is how a slim card names a related record: enough to link or label
// it, without the zero-valued remainder of its model.
type cardRef struct {
	ID   uint
	Name string
}

func cardEntityProjection(entity any) any {
	// List pages range over model values ([]models.Resource), MRQL over pointers.
	switch e := entity.(type) {
	case models.Resource:
		entity = &e
	case models.Group:
		entity = &e
	case models.Note:
		entity = &e
	}
	var slim any
	refs := map[string]any{}
	switch e := entity.(type) {
	case *models.Resource:
		if c := e.ResourceCategory; cardTemplatesReadEntity(e.Description) || c != nil && cardTemplatesReadEntity(c.CustomAvatar, c.CustomSummary, c.CustomMRQLResult) {
			return e
		}
		r := *e
		r.Meta, r.OwnMeta = emptyMeta, emptyMeta
		r.ResourceCategory, r.Series, r.Owner, r.Tags = nil, nil, nil, nil
		r.CurrentVersion, r.ImageHash = nil, nil
		r.Notes, r.Groups, r.Previews, r.Versions = nil, nil, nil, nil
		if c := e.ResourceCategory; c != nil {
			refs["resourceCategory"] = cardRef{c.ID, c.Name}
		}
		addOwnerAndTags(refs, e.Owner, e.Tags)
		slim = &r
	case *models.Group:
		if c := e.Category; cardTemplatesReadEntity(e.Description) || c != nil && cardTemplatesReadEntity(c.CustomAvatar, c.CustomSummary, c.CustomMRQLResult) {
			return e
		}
		g := *e
		g.Meta = emptyMeta
		g.Category, g.Owner, g.Tags = nil, nil, nil
		g.RelatedResources, g.RelatedNotes, g.RelatedGroups = nil, nil, nil
		g.OwnResources, g.OwnNotes, g.OwnGroups = nil, nil, nil
		g.Relationships, g.BackRelations = nil, nil
		if c := e.Category; c != nil {
			refs["Category"] = cardRef{c.ID, c.Name}
		}
		addOwnerAndTags(refs, e.Owner, e.Tags)
		slim = &g
	case *models.Note:
		if c := e.NoteType; cardTemplatesReadEntity(e.Description) || c != nil && cardTemplatesReadEntity(c.CustomAvatar, c.CustomSummary, c.CustomMRQLResult) {
			return e
		}
		n := *e
		n.Meta = emptyMeta
		n.NoteType, n.Owner, n.Tags = nil, nil, nil
		n.Resources, n.Groups, n.Blocks = nil, nil, nil
		if c := e.NoteType; c != nil {
			refs["NoteType"] = cardRef{c.ID, c.Name}
		}
		addOwnerAndTags(refs, e.Owner, e.Tags)
		slim = &n
	default:
		return entity
	}
	// The scalars go through the model's own JSON tags, so a renamed or added
	// field reaches the card unchanged; only the related records are swapped
	// for references.
	encoded, err := json.Marshal(slim)
	if err != nil {
		return entity
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return entity
	}
	for key, value := range refs {
		fields[key] = value
	}
	return fields
}

func addOwnerAndTags(refs map[string]any, owner *models.Group, tags []*models.Tag) {
	if owner != nil {
		refs["Owner"] = cardRef{owner.ID, owner.Name}
	}
	if tags != nil {
		slimTags := make([]cardRef, len(tags))
		for i, tag := range tags {
			slimTags[i] = cardRef{tag.ID, tag.Name}
		}
		refs["Tags"] = slimTags
	}
}
