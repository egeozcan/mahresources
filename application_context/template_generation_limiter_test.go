package application_context

import (
	"fmt"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/models"
)

// TestGenerationLimitersAreSharedByEveryContextCopy pins that the generation
// limiters exist from construction and that every per-request copy shares
// them. Created lazily, a copy made by WithPrincipal (what scopedAPI hands each
// request) got fresh buckets and the limit never triggered, and two first calls
// on one context raced on the field (run under -race).
func TestGenerationLimitersAreSharedByEveryContextCopy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), &MahresourcesConfig{})

	scope := uint(1)
	principals := []*auth.Principal{nil, {UserID: 2, Role: models.RoleEditor}, {UserID: 3, Role: models.RoleUser, ScopeGroupID: &scope}}
	var mu sync.Mutex
	template, mrql, disclosure := map[*MRQLGenerationRateLimiter]bool{}, map[*MRQLGenerationRateLimiter]bool{}, map[*MRQLGenerationRateLimiter]bool{}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := ctx
			if i%2 == 1 {
				c = ctx.WithPrincipal(principals[i%len(principals)])
			}
			tl, ml, dl := c.TemplateGenerationRateLimiter(), c.MRQLGenerationRateLimiter(), c.TemplateDisclosureRateLimiter()
			mu.Lock()
			template[tl], mrql[ml], disclosure[dl] = true, true, true
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if len(template) != 1 || len(mrql) != 1 || len(disclosure) != 1 || template[nil] || mrql[nil] || disclosure[nil] {
		t.Fatalf("want one shared limiter of each kind, got %d template, %d MRQL and %d dry-run limiters", len(template), len(mrql), len(disclosure))
	}
}
