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
