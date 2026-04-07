package application_context

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateAutoDetectRules_EmptyString(t *testing.T) {
	assert.NoError(t, ValidateAutoDetectRules(""))
}

func TestValidateAutoDetectRules_ValidRule(t *testing.T) {
	assert.NoError(t, ValidateAutoDetectRules(`{"contentTypes":["image/jpeg","image/png"],"width":{"min":100},"priority":5}`))
}

func TestValidateAutoDetectRules_InvalidJSON(t *testing.T) {
	err := ValidateAutoDetectRules(`not json`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestValidateAutoDetectRules_MissingContentTypes(t *testing.T) {
	err := ValidateAutoDetectRules(`{"width":{"min":100}}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "contentTypes")
}

func TestValidateAutoDetectRules_EmptyContentTypes(t *testing.T) {
	err := ValidateAutoDetectRules(`{"contentTypes":[]}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "contentTypes")
}

func TestValidateAutoDetectRules_UnknownField(t *testing.T) {
	err := ValidateAutoDetectRules(`{"contentTypes":["image/png"],"contenTypes":["image/png"]}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestValidateAutoDetectRules_InvalidRangeType(t *testing.T) {
	assert.Error(t, ValidateAutoDetectRules(`{"contentTypes":["image/png"],"width":"big"}`))
}

func TestValidateAutoDetectRules_RangeNoBounds(t *testing.T) {
	err := ValidateAutoDetectRules(`{"contentTypes":["image/png"],"width":{}}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one")
}

func TestValidateAutoDetectRules_PriorityNotInt(t *testing.T) {
	err := ValidateAutoDetectRules(`{"contentTypes":["image/png"],"priority":1.5}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "priority")
}

func TestValidateAutoDetectRules_ContentTypesNotStringArray(t *testing.T) {
	assert.Error(t, ValidateAutoDetectRules(`{"contentTypes":"image/png"}`))
}

func TestValidateAutoDetectRules_AllFieldsValid(t *testing.T) {
	assert.NoError(t, ValidateAutoDetectRules(`{"contentTypes":["image/jpeg"],"width":{"min":100,"max":5000},"height":{"min":100},"aspectRatio":{"min":0.5,"max":2.0},"fileSize":{"min":1000},"pixelCount":{"min":100000},"bytesPerPixel":{"max":6.0},"priority":10}`))
}
