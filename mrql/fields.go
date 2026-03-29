package mrql

// FieldType describes how a field is stored and queried.
type FieldType int

const (
	FieldString   FieldType = iota // string / text value
	FieldNumber                    // numeric value
	FieldDateTime                  // timestamp
	FieldRelation                  // many-to-many relation (tags, groups)
	FieldMeta                      // meta.* dynamic key-value pair
)

// FieldDef describes one queryable field on an entity.
type FieldDef struct {
	Name   string    // MRQL field name (camelCase)
	Type   FieldType // how the value is typed
	Column string    // underlying DB column / relation name
}

// commonFields are available on every entity type.
var commonFields = []FieldDef{
	{Name: "id", Type: FieldNumber, Column: "id"},
	{Name: "name", Type: FieldString, Column: "name"},
	{Name: "description", Type: FieldString, Column: "description"},
	{Name: "created", Type: FieldDateTime, Column: "created_at"},
	{Name: "updated", Type: FieldDateTime, Column: "updated_at"},
	{Name: "tags", Type: FieldRelation, Column: "tags"},
	// "meta" prefix is handled separately via FieldMeta lookup
}

// resourceFields are fields only available on the Resource entity.
var resourceFields = []FieldDef{
	{Name: "groups", Type: FieldRelation, Column: "groups"},
	{Name: "group", Type: FieldRelation, Column: "groups"}, // alias
	{Name: "owner", Type: FieldRelation, Column: "owner_id"},
	{Name: "category", Type: FieldNumber, Column: "resource_category_id"},
	{Name: "contentType", Type: FieldString, Column: "content_type"},
	{Name: "fileSize", Type: FieldNumber, Column: "file_size"},
	{Name: "width", Type: FieldNumber, Column: "width"},
	{Name: "height", Type: FieldNumber, Column: "height"},
	{Name: "originalName", Type: FieldString, Column: "original_name"},
	{Name: "hash", Type: FieldString, Column: "hash"},
}

// noteFields are fields only available on the Note entity.
var noteFields = []FieldDef{
	{Name: "groups", Type: FieldRelation, Column: "groups"},
	{Name: "group", Type: FieldRelation, Column: "groups"}, // alias
	{Name: "owner", Type: FieldRelation, Column: "owner_id"},
	{Name: "noteType", Type: FieldNumber, Column: "note_type_id"},
}

// groupFields are fields only available on the Group entity.
var groupFields = []FieldDef{
	{Name: "category", Type: FieldNumber, Column: "category_id"},
	{Name: "parent", Type: FieldRelation, Column: "parent_id"},
	{Name: "children", Type: FieldRelation, Column: "children"},
}

// ValidEntityTypes maps valid entity type string values to their EntityType constant.
var ValidEntityTypes = map[string]EntityType{
	"resource": EntityResource,
	"note":     EntityNote,
	"group":    EntityGroup,
}

// fieldIndex builds a lookup map from field name → FieldDef for a slice of fields.
func fieldIndex(fields []FieldDef) map[string]FieldDef {
	m := make(map[string]FieldDef, len(fields))
	for _, f := range fields {
		m[f.Name] = f
	}
	return m
}

// pre-built indexes for fast lookup
var (
	commonIndex   = fieldIndex(commonFields)
	resourceIndex = fieldIndex(resourceFields)
	noteIndex     = fieldIndex(noteFields)
	groupIndex    = fieldIndex(groupFields)
)

// LookupField returns the FieldDef for the given field name on the given entity type.
// Common fields are always checked first; then entity-specific fields.
// meta.* fields are matched via prefix and return a synthetic FieldDef with Type FieldMeta.
// Returns false if the field is not valid for the entity type.
func LookupField(entityType EntityType, fieldName string) (FieldDef, bool) {
	// meta.* is always valid
	if len(fieldName) > 5 && fieldName[:5] == "meta." {
		return FieldDef{Name: fieldName, Type: FieldMeta, Column: fieldName}, true
	}

	// Common fields are valid on all entity types
	if fd, ok := commonIndex[fieldName]; ok {
		return fd, true
	}

	// Entity-specific lookup
	switch entityType {
	case EntityResource:
		fd, ok := resourceIndex[fieldName]
		return fd, ok
	case EntityNote:
		fd, ok := noteIndex[fieldName]
		return fd, ok
	case EntityGroup:
		fd, ok := groupIndex[fieldName]
		return fd, ok
	default:
		// Unspecified entity: only common fields are allowed
		return FieldDef{}, false
	}
}

// IsCommonField returns true if fieldName is a field available on all entities.
func IsCommonField(fieldName string) bool {
	_, ok := commonIndex[fieldName]
	return ok
}

// isFieldOnAnyEntity returns true if fieldName is valid on at least one entity type.
func isFieldOnAnyEntity(fieldName string) bool {
	if IsCommonField(fieldName) {
		return true
	}
	for _, idx := range []map[string]FieldDef{resourceIndex, noteIndex, groupIndex} {
		if _, ok := idx[fieldName]; ok {
			return true
		}
	}
	return false
}
