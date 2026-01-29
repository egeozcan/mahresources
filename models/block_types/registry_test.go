package block_types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistry_GetBlockType(t *testing.T) {
	bt := GetBlockType("text")
	assert.NotNil(t, bt)
	assert.Equal(t, "text", bt.Type())
}

func TestRegistry_GetBlockType_Unknown(t *testing.T) {
	bt := GetBlockType("unknown_type")
	assert.Nil(t, bt)
}

func TestRegistry_ValidateContent_Text(t *testing.T) {
	bt := GetBlockType("text")
	content := json.RawMessage(`{"text": "hello"}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestRegistry_ValidateContent_Text_Invalid(t *testing.T) {
	bt := GetBlockType("text")
	content := json.RawMessage(`{"invalid": 123}`)
	err := bt.ValidateContent(content)
	assert.Error(t, err)
}

func TestRegistry_GetBlockType_Heading(t *testing.T) {
	bt := GetBlockType("heading")
	assert.NotNil(t, bt)
	assert.Equal(t, "heading", bt.Type())
}

func TestRegistry_ValidateContent_Heading_ValidLevels(t *testing.T) {
	bt := GetBlockType("heading")

	// Test all valid levels 1-6
	for level := 1; level <= 6; level++ {
		content := json.RawMessage(`{"text": "Test", "level": ` + string(rune('0'+level)) + `}`)
		err := bt.ValidateContent(content)
		assert.NoError(t, err, "level %d should be valid", level)
	}
}

func TestRegistry_ValidateContent_Heading_InvalidLevel_Zero(t *testing.T) {
	bt := GetBlockType("heading")
	content := json.RawMessage(`{"text": "Test", "level": 0}`)
	err := bt.ValidateContent(content)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "level must be 1-6")
}

func TestRegistry_ValidateContent_Heading_InvalidLevel_Seven(t *testing.T) {
	bt := GetBlockType("heading")
	content := json.RawMessage(`{"text": "Test", "level": 7}`)
	err := bt.ValidateContent(content)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "level must be 1-6")
}

func TestRegistry_ValidateContent_Heading_InvalidLevel_Negative(t *testing.T) {
	bt := GetBlockType("heading")
	content := json.RawMessage(`{"text": "Test", "level": -1}`)
	err := bt.ValidateContent(content)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "level must be 1-6")
}

func TestRegistry_GetBlockType_Divider(t *testing.T) {
	bt := GetBlockType("divider")
	assert.NotNil(t, bt)
	assert.Equal(t, "divider", bt.Type())
}

func TestRegistry_ValidateContent_Divider(t *testing.T) {
	bt := GetBlockType("divider")
	content := json.RawMessage(`{}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestRegistry_GetBlockType_Gallery(t *testing.T) {
	bt := GetBlockType("gallery")
	assert.NotNil(t, bt)
	assert.Equal(t, "gallery", bt.Type())
}

func TestRegistry_ValidateContent_Gallery(t *testing.T) {
	bt := GetBlockType("gallery")
	content := json.RawMessage(`{"resourceIds": [1, 2, 3]}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestRegistry_ValidateState_Gallery_ValidLayout(t *testing.T) {
	bt := GetBlockType("gallery")

	// Test grid layout
	state := json.RawMessage(`{"layout": "grid"}`)
	err := bt.ValidateState(state)
	assert.NoError(t, err)

	// Test list layout
	state = json.RawMessage(`{"layout": "list"}`)
	err = bt.ValidateState(state)
	assert.NoError(t, err)

	// Test empty layout (allowed)
	state = json.RawMessage(`{}`)
	err = bt.ValidateState(state)
	assert.NoError(t, err)
}

func TestRegistry_ValidateState_Gallery_InvalidLayout(t *testing.T) {
	bt := GetBlockType("gallery")
	state := json.RawMessage(`{"layout": "invalid"}`)
	err := bt.ValidateState(state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "layout must be 'grid' or 'list'")
}

func TestRegistry_GetBlockType_References(t *testing.T) {
	bt := GetBlockType("references")
	assert.NotNil(t, bt)
	assert.Equal(t, "references", bt.Type())
}

func TestRegistry_ValidateContent_References(t *testing.T) {
	bt := GetBlockType("references")
	content := json.RawMessage(`{"groupIds": [1, 2, 3]}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestRegistry_GetBlockType_Todos(t *testing.T) {
	bt := GetBlockType("todos")
	assert.NotNil(t, bt)
	assert.Equal(t, "todos", bt.Type())
}

func TestRegistry_ValidateContent_Todos_Valid(t *testing.T) {
	bt := GetBlockType("todos")
	content := json.RawMessage(`{"items": [{"id": "1", "label": "Task 1"}, {"id": "2", "label": "Task 2"}]}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestRegistry_ValidateContent_Todos_MissingID(t *testing.T) {
	bt := GetBlockType("todos")
	content := json.RawMessage(`{"items": [{"id": "", "label": "Task 1"}]}`)
	err := bt.ValidateContent(content)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must have an id")
}

func TestRegistry_ValidateState_Todos(t *testing.T) {
	bt := GetBlockType("todos")
	state := json.RawMessage(`{"checked": ["1", "2"]}`)
	err := bt.ValidateState(state)
	assert.NoError(t, err)
}

func TestRegistry_GetBlockType_Table(t *testing.T) {
	bt := GetBlockType("table")
	assert.NotNil(t, bt)
	assert.Equal(t, "table", bt.Type())
}

func TestRegistry_ValidateContent_Table_ManualData(t *testing.T) {
	bt := GetBlockType("table")
	content := json.RawMessage(`{"columns": ["Name", "Value"], "rows": [["a", 1], ["b", 2]]}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestRegistry_ValidateContent_Table_QueryID(t *testing.T) {
	bt := GetBlockType("table")
	content := json.RawMessage(`{"queryId": 123}`)
	err := bt.ValidateContent(content)
	assert.NoError(t, err)
}

func TestRegistry_ValidateContent_Table_BothNotAllowed(t *testing.T) {
	bt := GetBlockType("table")
	content := json.RawMessage(`{"columns": ["Name"], "rows": [], "queryId": 123}`)
	err := bt.ValidateContent(content)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot have both columns/rows and queryId")
}

func TestRegistry_ValidateState_Table_ValidSortDir(t *testing.T) {
	bt := GetBlockType("table")

	// Test asc
	state := json.RawMessage(`{"sortColumn": "name", "sortDir": "asc"}`)
	err := bt.ValidateState(state)
	assert.NoError(t, err)

	// Test desc
	state = json.RawMessage(`{"sortColumn": "name", "sortDir": "desc"}`)
	err = bt.ValidateState(state)
	assert.NoError(t, err)

	// Test empty (allowed)
	state = json.RawMessage(`{}`)
	err = bt.ValidateState(state)
	assert.NoError(t, err)
}

func TestRegistry_ValidateState_Table_InvalidSortDir(t *testing.T) {
	bt := GetBlockType("table")
	state := json.RawMessage(`{"sortColumn": "name", "sortDir": "invalid"}`)
	err := bt.ValidateState(state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sortDir must be 'asc' or 'desc'")
}

func TestRegistry_GetAllBlockTypes(t *testing.T) {
	types := GetAllBlockTypes()
	// Should have at least text, heading, divider, gallery, references, todos, table
	assert.GreaterOrEqual(t, len(types), 7)

	// Check all expected types are present
	typeNames := make(map[string]bool)
	for _, bt := range types {
		typeNames[bt.Type()] = true
	}
	assert.True(t, typeNames["text"])
	assert.True(t, typeNames["heading"])
	assert.True(t, typeNames["divider"])
	assert.True(t, typeNames["gallery"])
	assert.True(t, typeNames["references"])
	assert.True(t, typeNames["todos"])
	assert.True(t, typeNames["table"])
}
