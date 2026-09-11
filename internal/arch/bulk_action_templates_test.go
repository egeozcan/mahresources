package arch

import (
	"mahresources/listviews"
	"os"
	"path/filepath"
	"testing"
)

func TestBulkActionComponentsExist(t *testing.T) {
	for _, entity := range []string{"resource", "note", "group", "tag", "download"} {
		for _, action := range listviews.BulkActions(entity) {
			if action.Component == "" {
				continue
			}
			name := filepath.Join(moduleRoot(t), "templates", "partials", "bulkActions", action.Component+".tpl")
			if _, err := os.Stat(name); err != nil {
				t.Errorf("%s/%s component %q: %v", entity, action.ID, action.Component, err)
			}
		}
	}
}
