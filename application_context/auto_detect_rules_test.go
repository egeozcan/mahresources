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

func TestMatchAutoDetectRule_ContentTypeMatch(t *testing.T) {
	rule := AutoDetectRule{ContentTypes: []string{"image/jpeg", "image/png"}}
	matched, evaluated := rule.Match("image/jpeg", 0, 0, 1000)
	assert.True(t, matched)
	assert.Equal(t, 1, evaluated)
}

func TestMatchAutoDetectRule_ContentTypeMismatch(t *testing.T) {
	rule := AutoDetectRule{ContentTypes: []string{"image/jpeg"}}
	matched, _ := rule.Match("image/png", 0, 0, 1000)
	assert.False(t, matched)
}

func TestMatchAutoDetectRule_WidthRange(t *testing.T) {
	min, max := float64(100), float64(500)
	rule := AutoDetectRule{ContentTypes: []string{"image/png"}, Width: &RangeRule{Min: &min, Max: &max}}

	matched, evaluated := rule.Match("image/png", 200, 100, 1000)
	assert.True(t, matched)
	assert.Equal(t, 2, evaluated)

	matched, _ = rule.Match("image/png", 600, 100, 1000)
	assert.False(t, matched)

	matched, _ = rule.Match("image/png", 50, 100, 1000)
	assert.False(t, matched)
}

func TestMatchAutoDetectRule_DimensionFieldsSkippedWhenNoDimensions(t *testing.T) {
	min := float64(1000)
	rule := AutoDetectRule{ContentTypes: []string{"application/pdf"}, Width: &RangeRule{Min: &min}}
	matched, evaluated := rule.Match("application/pdf", 0, 0, 50000)
	assert.True(t, matched)
	assert.Equal(t, 1, evaluated)
}

func TestMatchAutoDetectRule_AspectRatio(t *testing.T) {
	min, max := float64(1.3), float64(1.8)
	rule := AutoDetectRule{ContentTypes: []string{"image/png"}, AspectRatio: &RangeRule{Min: &min, Max: &max}}
	matched, evaluated := rule.Match("image/png", 1920, 1080, 500000)
	assert.True(t, matched)
	assert.Equal(t, 2, evaluated)

	matched, _ = rule.Match("image/png", 1080, 1920, 500000)
	assert.False(t, matched)
}

func TestMatchAutoDetectRule_BytesPerPixel(t *testing.T) {
	max := float64(3.0)
	rule := AutoDetectRule{ContentTypes: []string{"image/png"}, BytesPerPixel: &RangeRule{Max: &max}}
	matched, _ := rule.Match("image/png", 1000, 1000, 2000000)
	assert.True(t, matched)
	matched, _ = rule.Match("image/png", 1000, 1000, 5000000)
	assert.False(t, matched)
}

func TestMatchAutoDetectRule_FileSizeOnly(t *testing.T) {
	min, max := float64(1000), float64(100000)
	rule := AutoDetectRule{ContentTypes: []string{"application/pdf"}, FileSize: &RangeRule{Min: &min, Max: &max}}
	matched, evaluated := rule.Match("application/pdf", 0, 0, 50000)
	assert.True(t, matched)
	assert.Equal(t, 2, evaluated)
}

func TestMatchAutoDetectRule_PixelCount(t *testing.T) {
	min := float64(2000000)
	rule := AutoDetectRule{ContentTypes: []string{"image/jpeg"}, PixelCount: &RangeRule{Min: &min}}
	matched, _ := rule.Match("image/jpeg", 2000, 1500, 500000)
	assert.True(t, matched)
	matched, _ = rule.Match("image/jpeg", 800, 600, 500000)
	assert.False(t, matched)
}
