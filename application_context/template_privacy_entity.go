package application_context

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"mahresources/models/types"
	"mahresources/shortcodes"
)

// Entity and QueryResult: the masked deep copies a preview renders against.

// privacyCheckEvery is how many nodes Entity copies between context checks.
const privacyCheckEvery = 1000

// privacyMaxDepth bounds the recursion of one copy.
const privacyMaxDepth = 64

var (
	privacyTimeType       = reflect.TypeOf(time.Time{})
	privacyDeletedAtType  = reflect.TypeOf(gorm.DeletedAt{})
	privacyNullTimeType   = reflect.TypeOf(sql.NullTime{})
	privacyURLType        = reflect.TypeOf(types.URL{})
	privacyJSONType       = reflect.TypeOf(types.JSON{})
	privacyContentTypeRE  = regexp.MustCompile(`^[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*$`)
	privacyCarrierStructs = map[string]bool{"Category": true, "ResourceCategory": true, "NoteType": true}
)

// privacyFieldKept is the P5 allow-list: the vocabulary that stays readable when
// it sits inside an entity. Everything else is masked, so a model field added
// later is protected by default.
func privacyFieldKept(structName, field string) bool {
	switch {
	case privacyCarrierStructs[structName]:
		return field == "Name" || field == "MetaSchema" || field == "SectionConfig" || strings.HasPrefix(field, "Custom")
	case structName == "GroupRelationType":
		return field == "Name"
	case structName == "NoteBlock":
		return field == "Type"
	}
	return false
}

type privacyVisit struct {
	ptr uintptr
	typ reflect.Type
}

type privacyCopier struct {
	ctx     context.Context
	p       *TemplatePrivacy
	visited map[privacyVisit]reflect.Value
	nodes   int
	err     error
}

// Entity returns a deep copy of e (a model, usually a pointer to one) with every
// Entity value replaced (P2, P5, P6, P16). The original and everything
// reachable from it is left untouched. It returns the sticky limit error or the
// context's error rather than a partial copy.
func (p *TemplatePrivacy) Entity(ctx context.Context, e any) (any, error) {
	if p == nil || e == nil {
		return e, nil
	}
	c := &privacyCopier{ctx: ctx, p: p, visited: map[privacyVisit]reflect.Value{}}
	out := c.copy(reflect.ValueOf(e), "", "", nil, 0)
	if c.err != nil {
		return nil, c.err
	}
	if err := p.Err(); err != nil {
		return nil, err
	}
	if !out.IsValid() {
		return nil, nil
	}
	return out.Interface(), nil
}

func (c *privacyCopier) tick() bool {
	if c.err != nil {
		return false
	}
	c.nodes++
	if c.nodes%privacyCheckEvery == 0 {
		if err := c.ctx.Err(); err != nil {
			c.err = err
			return false
		}
	}
	return true
}

// metaSchemaOf is the saved schema of the category an entity struct carries.
func metaSchemaOf(v reflect.Value) string {
	for _, name := range []string{"Category", "ResourceCategory", "NoteType"} {
		f := v.FieldByName(name)
		if f.IsValid() && f.Kind() == reflect.Ptr && !f.IsNil() {
			if s := f.Elem().FieldByName("MetaSchema"); s.IsValid() && s.Kind() == reflect.String {
				return s.String()
			}
		}
	}
	return ""
}

func (c *privacyCopier) copy(src reflect.Value, structName, field string, parent *reflect.Value, depth int) reflect.Value {
	if !src.IsValid() || !c.tick() {
		return reflect.Value{}
	}
	typ := src.Type()
	zero := reflect.Zero(typ)
	if depth > privacyMaxDepth {
		return zero
	}
	switch typ {
	case privacyTimeType:
		if src.Interface().(time.Time).IsZero() {
			return src
		}
		return reflect.ValueOf(templatePrivacyStandIn)
	case privacyDeletedAtType:
		d := src.Interface().(gorm.DeletedAt)
		if d.Valid {
			d.Time = templatePrivacyStandIn
		}
		return reflect.ValueOf(d)
	case privacyNullTimeType:
		d := src.Interface().(sql.NullTime)
		if d.Valid {
			d.Time = templatePrivacyStandIn
		}
		return reflect.ValueOf(d)
	case privacyURLType:
		u := url.URL(src.Interface().(types.URL))
		if u.String() == "" {
			return src
		}
		alias, err := url.Parse(c.p.URLAlias(u.String()))
		if err != nil {
			return zero
		}
		return reflect.ValueOf(types.URL(*alias))
	case privacyJSONType:
		raw := src.Interface().(types.JSON)
		if len(raw) == 0 {
			return src
		}
		if privacyFieldKept(structName, field) {
			c.p.Keep(string(raw))
			return reflect.ValueOf(append(types.JSON(nil), raw...))
		}
		schema := ""
		if parent != nil {
			schema = metaSchemaOf(*parent)
		}
		return reflect.ValueOf(types.JSON(c.p.Meta(raw, schema)))
	}
	switch src.Kind() {
	case reflect.String:
		s := src.String()
		out := reflect.New(typ).Elem()
		switch {
		case structName == "Resource" && field == "ContentType":
			out.SetString(privacyContentType(c.p, s))
		case privacyFieldKept(structName, field):
			out.SetString(c.p.Keep(s))
		default:
			out.SetString(c.p.Text(s))
		}
		return out
	case reflect.Ptr:
		if src.IsNil() {
			return zero
		}
		key := privacyVisit{src.Pointer(), typ}
		if done, ok := c.visited[key]; ok {
			return done
		}
		out := reflect.New(typ.Elem())
		c.visited[key] = out
		if inner := c.copy(src.Elem(), structName, field, parent, depth+1); inner.IsValid() {
			out.Elem().Set(inner)
		}
		return out
	case reflect.Interface:
		if src.IsNil() {
			return zero
		}
		out := reflect.New(typ).Elem()
		if inner := c.copy(src.Elem(), structName, field, parent, depth+1); inner.IsValid() {
			out.Set(inner)
		}
		return out
	case reflect.Slice:
		if src.IsNil() {
			return zero
		}
		if typ.Elem().Kind() == reflect.Uint8 {
			return zero // preview thumbnails and other bytes: a render never needs them
		}
		out := reflect.MakeSlice(typ, src.Len(), src.Len())
		for i := 0; i < src.Len(); i++ {
			if inner := c.copy(src.Index(i), structName, field, parent, depth+1); inner.IsValid() {
				out.Index(i).Set(inner)
			}
		}
		return out
	case reflect.Array:
		out := reflect.New(typ).Elem()
		for i := 0; i < src.Len(); i++ {
			if inner := c.copy(src.Index(i), structName, field, parent, depth+1); inner.IsValid() {
				out.Index(i).Set(inner)
			}
		}
		return out
	case reflect.Map:
		if src.IsNil() {
			return zero
		}
		out := reflect.MakeMapWithSize(typ, src.Len())
		keys := src.MapKeys()
		sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
		for _, k := range keys {
			if k.Kind() == reflect.String {
				c.p.Keep(k.String())
			}
			if inner := c.copy(src.MapIndex(k), structName, field, parent, depth+1); inner.IsValid() {
				out.SetMapIndex(k, inner)
			}
		}
		return out
	case reflect.Struct:
		out := reflect.New(typ).Elem()
		name := typ.Name()
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if !f.IsExported() {
				continue
			}
			if inner := c.copy(src.Field(i), name, f.Name, &src, depth+1); inner.IsValid() {
				out.Field(i).Set(inner)
			}
		}
		return out
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return zero
	}
	// Numbers and booleans pass through, and their text forms join the corpus.
	c.p.keepScalar(src.Interface())
	return src
}

// QueryResult masks a query result field by field (P15). It never mutates r or
// anything reachable from it: the query cache shares row maps and bucket-key
// maps between clones, so every container is allocated anew.
func (p *TemplatePrivacy) QueryResult(ctx context.Context, r *shortcodes.QueryResult) (*shortcodes.QueryResult, error) {
	if p == nil || r == nil {
		return r, nil
	}
	out := &shortcodes.QueryResult{
		EntityType: r.EntityType, Mode: r.Mode, SavedID: r.SavedID,
		EffectiveQuery: p.Keep(r.EffectiveQuery),
	}
	if r.Total != nil {
		total := *r.Total
		out.Total = &total
		p.keepScalar(total)
	}
	var err error
	if r.Items != nil {
		if out.Items, err = p.queryItems(ctx, r.Items); err != nil {
			return nil, err
		}
	}
	if r.Rows != nil {
		out.Rows = make([]map[string]any, len(r.Rows))
		for i, row := range r.Rows {
			out.Rows[i] = p.queryRow(row)
		}
	}
	if r.Groups != nil {
		out.Groups = make([]shortcodes.QueryResultGroup, len(r.Groups))
		for i, g := range r.Groups {
			out.Groups[i].Key = p.queryRow(g.Key)
			if g.Items != nil {
				if out.Groups[i].Items, err = p.queryItems(ctx, g.Items); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := p.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *TemplatePrivacy) queryItems(ctx context.Context, items []shortcodes.QueryResultItem) ([]shortcodes.QueryResultItem, error) {
	out := make([]shortcodes.QueryResultItem, len(items))
	for i, it := range items {
		n := it
		var err error
		if n.Entity, err = p.Entity(ctx, it.Entity); err != nil {
			return nil, err
		}
		n.Meta = json.RawMessage(p.Meta(it.Meta, it.MetaSchema))
		n.MetaSchema = p.Keep(it.MetaSchema)
		n.ScopeGroupName, n.ParentGroupName, n.RootGroupName = p.Text(it.ScopeGroupName), p.Text(it.ParentGroupName), p.Text(it.RootGroupName)
		n.ScopeCategoryName, n.ParentCategoryName, n.RootCategoryName = p.Keep(it.ScopeCategoryName), p.Keep(it.ParentCategoryName), p.Keep(it.RootCategoryName)
		n.CustomMRQLResult, n.CustomCSS = p.Keep(it.CustomMRQLResult), p.Keep(it.CustomCSS)
		p.keepScalar(int64(it.EntityID))
		out[i] = n
	}
	return out, nil
}

func (p *TemplatePrivacy) queryRow(row map[string]any) map[string]any {
	if row == nil {
		return nil
	}
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]any, len(row))
	for _, k := range keys {
		p.Keep(k)
		out[k] = p.queryValue(row[k])
	}
	return out
}

func (p *TemplatePrivacy) queryValue(v any) any {
	switch x := v.(type) {
	case string:
		return p.Text(x)
	case time.Time:
		return p.Date(x)
	case *time.Time:
		if x == nil {
			return x
		}
		return p.Date(*x)
	case map[string]any:
		return p.queryRow(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = p.queryValue(e)
		}
		return out
	case []byte:
		return p.Text(string(x))
	}
	p.keepScalar(v)
	return v
}

// privacyContentType keeps the lower-case type/subtype of a content type and
// drops its parameters (P5): the model needs image versus video, and a stored
// value can come straight from an import. Anything that is not plain lower-case
// type/subtype once the parameters are cut is masked. nil p keeps all.
func privacyContentType(p *TemplatePrivacy, s string) string {
	if p == nil || s == "" {
		return s
	}
	base, _, _ := strings.Cut(s, ";")
	base = strings.TrimSpace(base)
	if privacyContentTypeRE.MatchString(base) {
		return p.Keep(base)
	}
	return p.Text(s)
}

// TemplateEntityMetaSchema is the saved MetaSchema of the category an entity
// carries (empty when none is loaded): the schema whose enum and const values
// stay readable in the entity's Meta.
func TemplateEntityMetaSchema(entity any) string {
	v := reflect.ValueOf(entity)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}
	return metaSchemaOf(v)
}
