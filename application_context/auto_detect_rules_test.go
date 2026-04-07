package application_context

import (
	"fmt"
	"mahresources/models"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

// --- detectResourceCategory tests ---

func setupDetectTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	dbName := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&models.ResourceCategory{}, &models.Resource{}, &models.LogEntry{})
	require.NoError(t, err)

	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default"}
	defaultRC.ID = 1
	db.FirstOrCreate(defaultRC, 1)

	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, readOnlyDB, &MahresourcesConfig{})
	ctx.DefaultResourceCategoryID = defaultRC.ID
	return ctx
}

func TestDetectResourceCategory_MatchesRule(t *testing.T) {
	ctx := setupDetectTestContext(t)
	cat := models.ResourceCategory{Name: "Photos", AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":1}`}
	require.NoError(t, ctx.db.Create(&cat).Error)

	got := ctx.detectResourceCategory("image/jpeg", 1920, 1080, 500000)
	assert.Equal(t, cat.ID, got)
}

func TestDetectResourceCategory_NoMatch_ReturnsDefault(t *testing.T) {
	ctx := setupDetectTestContext(t)
	cat := models.ResourceCategory{Name: "Photos", AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":1}`}
	require.NoError(t, ctx.db.Create(&cat).Error)

	got := ctx.detectResourceCategory("application/pdf", 0, 0, 50000)
	assert.Equal(t, ctx.DefaultResourceCategoryID, got)
}

func TestDetectResourceCategory_HighestPriorityWins(t *testing.T) {
	ctx := setupDetectTestContext(t)
	lowPriority := models.ResourceCategory{Name: "LowPriority", AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":1}`}
	highPriority := models.ResourceCategory{Name: "HighPriority", AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":10}`}
	require.NoError(t, ctx.db.Create(&lowPriority).Error)
	require.NoError(t, ctx.db.Create(&highPriority).Error)

	got := ctx.detectResourceCategory("image/jpeg", 1920, 1080, 500000)
	assert.Equal(t, highPriority.ID, got)
}

func TestDetectResourceCategory_TieBreakByEvaluatedFields(t *testing.T) {
	ctx := setupDetectTestContext(t)
	minW := float64(100)
	// Both have same priority, but "Detailed" evaluates more fields (contentTypes + width = 2 vs contentTypes = 1)
	basic := models.ResourceCategory{Name: "Basic", AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":5}`}
	detailed := models.ResourceCategory{Name: "Detailed", AutoDetectRules: `{"contentTypes":["image/jpeg"],"width":{"min":100},"priority":5}`}
	require.NoError(t, ctx.db.Create(&basic).Error)
	require.NoError(t, ctx.db.Create(&detailed).Error)
	_ = minW // just for clarity

	got := ctx.detectResourceCategory("image/jpeg", 1920, 1080, 500000)
	assert.Equal(t, detailed.ID, got)
}

func TestDetectResourceCategory_TieBreakByLowestID(t *testing.T) {
	ctx := setupDetectTestContext(t)
	// Same priority, same evaluated fields (just contentTypes) -- lower ID wins
	catA := models.ResourceCategory{Name: "CatA", AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":5}`}
	catB := models.ResourceCategory{Name: "CatB", AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":5}`}
	require.NoError(t, ctx.db.Create(&catA).Error)
	require.NoError(t, ctx.db.Create(&catB).Error)
	// catA was created first, so it should have a lower ID
	require.Less(t, catA.ID, catB.ID)

	got := ctx.detectResourceCategory("image/jpeg", 0, 0, 500000)
	assert.Equal(t, catA.ID, got)
}

func TestDetectResourceCategory_NoRulesExist(t *testing.T) {
	ctx := setupDetectTestContext(t)
	// No categories with AutoDetectRules
	cat := models.ResourceCategory{Name: "Plain"}
	require.NoError(t, ctx.db.Create(&cat).Error)

	got := ctx.detectResourceCategory("image/jpeg", 1920, 1080, 500000)
	assert.Equal(t, ctx.DefaultResourceCategoryID, got)
}

func TestDetectResourceCategory_SkippedFieldsDontCountForTiebreak(t *testing.T) {
	ctx := setupDetectTestContext(t)
	// Both match PDFs (no dimensions). "WithWidth" has a width rule but it gets
	// skipped because PDFs have no dimensions (evaluated=1). "WithFileSize" has
	// a fileSize rule that is always evaluated (evaluated=2). Same priority,
	// so fileSize category should win via evaluated count.
	withWidth := models.ResourceCategory{Name: "WithWidth", AutoDetectRules: `{"contentTypes":["application/pdf"],"width":{"min":100},"priority":5}`}
	withFileSize := models.ResourceCategory{Name: "WithFileSize", AutoDetectRules: `{"contentTypes":["application/pdf"],"fileSize":{"min":1000},"priority":5}`}
	require.NoError(t, ctx.db.Create(&withWidth).Error)
	require.NoError(t, ctx.db.Create(&withFileSize).Error)

	got := ctx.detectResourceCategory("application/pdf", 0, 0, 50000)
	assert.Equal(t, withFileSize.ID, got)
}
