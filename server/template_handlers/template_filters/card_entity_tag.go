package template_filters

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/flosch/pongo2/v4"
	"mahresources/models"
	"mahresources/models/types"
)

func init() {
	if err := pongo2.RegisterTag("card_entity", cardEntityTagParser); err != nil {
		fmt.Println("error when registering card_entity tag", err)
	}
	if err := pongo2.RegisterTag("card_entity_attr", cardEntityAttrTagParser); err != nil {
		fmt.Println("error when registering card_entity_attr tag", err)
	}
}

// cardEntityMarkerKey holds, for the card being rendered, the marker that
// {% card_entity_attr %} writes where the data-entity value goes. It is random
// per card: user content (a name in an aria-label) renders before the attribute,
// and a fixed marker typed into a name would receive the JSON inside a
// double-quoted attribute instead.
const cardEntityMarkerKey = "_cardEntityMarker"

// cardEntityScope is the wrapper's own reading of the attribute. It names the
// entity by definition, so it is left out when the card is scanned.
const cardEntityScope = `x-data="{ entity: JSON.parse($el.dataset.entity) }"`

// cardEntityNode renders a list card, then fills in the entity its Alpine scope
// parses from data-entity.
//
// A card used to carry the whole model: Meta, ownMeta, the series (with a second
// copy of the shared Meta) and the category with its schema and templates. On a
// library with large metadata that was hundreds of kilobytes per card, sixteen
// megabytes for one page, all parsed by the browser and again by JSON.parse
// before the page could respond.
//
// The full entity is documented for the card's own templates, which can bind
// `entity.Meta.*`. Whether one does can only be known from what the card renders:
// a partial, a plugin shortcode or a raw value can each emit a binding that the
// template text does not show. So the card is rendered first, and keeps the full
// entity only when its markup reads `entity`. Every other card gets the fields
// the list code itself reads (scalars, tags for Edit Tags, the category id for
// bulk-action filters, UpdatedAt for deferred shortcodes) with Meta left as an
// empty object. A [lazy] or [reload] body rendered later still binds to the full
// entity: its response merges the entity into the scope it lands in.
type cardEntityNode struct {
	entity  pongo2.IEvaluator
	wrapper *pongo2.NodeWrapper
}

func cardEntityTagParser(doc *pongo2.Parser, _ *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	entity, err := arguments.ParseExpression()
	if err != nil {
		return nil, err
	}
	if arguments.Remaining() > 0 {
		return nil, arguments.Error("card_entity takes one entity expression.", nil)
	}
	wrapper, _, err := doc.WrapUntilTag("endcard_entity")
	if err != nil {
		return nil, err
	}
	return &cardEntityNode{entity: entity, wrapper: wrapper}, nil
}

func (node *cardEntityNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	value, err := node.entity.Evaluate(ctx)
	if err != nil {
		return err
	}
	var nonce [16]byte
	if _, randErr := rand.Read(nonce[:]); randErr != nil {
		return ctx.Error(randErr.Error(), nil)
	}
	marker := "card-entity-" + hex.EncodeToString(nonce[:])
	outer, hadOuter := ctx.Private[cardEntityMarkerKey]
	ctx.Private[cardEntityMarkerKey] = marker
	var body strings.Builder
	err = node.wrapper.Execute(ctx, &body)
	if hadOuter {
		ctx.Private[cardEntityMarkerKey] = outer
	} else {
		delete(ctx.Private, cardEntityMarkerKey)
	}
	if err != nil {
		return err
	}
	card := body.String()
	entity := value.Interface()
	// Alpine reads attribute values after the HTML parser has decoded them, so
	// an expression written as &#101;ntity is a read of entity.
	if !markupReadsEntity(html.UnescapeString(strings.ReplaceAll(card, cardEntityScope, ""))) {
		entity = cardEntityProjection(entity)
	}
	encoded, jsonErr := json.Marshal(entity)
	if jsonErr != nil {
		return ctx.Error(jsonErr.Error(), nil)
	}
	// Go writes <, > and & as \u escapes, so in a single-quoted attribute only
	// the quote itself needs an entity. Autoescape spent six characters on
	// every double quote instead.
	attr := strings.ReplaceAll(string(encoded), "'", "&#39;")
	_, writeErr := writer.WriteString(strings.Replace(card, marker, attr, 1))
	if writeErr != nil {
		return ctx.Error(writeErr.Error(), nil)
	}
	return nil
}

type cardEntityAttrNode struct{}

func cardEntityAttrTagParser(_ *pongo2.Parser, _ *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	if arguments.Remaining() > 0 {
		return nil, arguments.Error("card_entity_attr takes no arguments.", nil)
	}
	return &cardEntityAttrNode{}, nil
}

func (node *cardEntityAttrNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	marker, ok := ctx.Private[cardEntityMarkerKey].(string)
	if !ok {
		return ctx.Error("card_entity_attr outside a card_entity block", nil)
	}
	if _, err := writer.WriteString(marker); err != nil {
		return ctx.Error(err.Error(), nil)
	}
	return nil
}

// markupReadsEntity reports whether rendered card markup refers to the Alpine
// `entity` as a JavaScript token: entity.Meta, 100-entity.x, ...entity,
// $data?.entity, card.entity. Words that only contain it (entityType,
// entity-row, data-entity-type) do not count, and neither does the data-entity
// attribute name or a [data-entity] selector. Ordinary prose that says "entity"
// does, which only costs that card its slimming. Plugin code that reads the
// entity from a script the markup does not show opts in by naming it, for
// example with a data-uses-entity attribute.
func markupReadsEntity(markup string) bool {
	const word = "entity"
	for offset := 0; ; {
		i := strings.Index(markup[offset:], word)
		if i < 0 {
			return false
		}
		start, end := offset+i, offset+i+len(word)
		offset = end
		if end < len(markup) && (isIdentifierByte(markup[end]) || markup[end] == '-') {
			continue
		}
		if start > 0 && isIdentifierByte(markup[start-1]) {
			continue
		}
		// data-entity as an attribute name (followed by =, > or the next
		// attribute) or a [data-entity] selector. Anything else after it, such
		// as data-entity.Meta or data-entity /* c */ .Meta, is an expression
		// subtracting the entity from a variable `data`.
		if strings.HasSuffix(markup[:start], "data-") && (end == len(markup) || strings.IndexByte("=]>'\" \t\r\n", markup[end]) >= 0) {
			next := strings.TrimLeft(markup[end:], " \t\r\n")
			if next == "" || strings.IndexByte("=]>'\"", next[0]) >= 0 || isIdentifierByte(next[0]) {
				continue
			}
		}
		return true
	}
}

func isIdentifierByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '$'
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
