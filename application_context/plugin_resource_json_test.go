package application_context

import (
	"strings"
	"testing"

	"github.com/spf13/afero"

	"mahresources/models"
	"mahresources/models/query_models"
)

const resourceJSONPlugin = `plugin = { name = "jsonreader", version = "1.0", api_version = 1,
           capabilities = { "db:read", "hooks", "inject" } }
local report = ""
function init()
    mah.on("after_note_create", function(data)
        local n, sum = 0, 0
        local ok, err = mah.db.each_resource_json(tonumber(data.description) or 0, function(key, i, value)
            n = n + 1
            sum = sum + value.v
        end)
        if ok then
            report = "ok:" .. n .. ":" .. sum
        else
            report = "error:" .. tostring(err)
        end
        return data
    end)
    mah.inject("page_top", function(c) return report end)
end
`

func putJSON(t *testing.T, ctx *MahresourcesContext, name string, owner *models.Group, body string) *models.Resource {
	t.Helper()
	location := name + ".json"
	if err := afero.WriteFile(ctx.fs, location, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", location, err)
	}
	res := &models.Resource{Name: name, Hash: "hash-" + name, Location: location, ContentType: "application/json",
		FileSize: int64(len(body)), OwnerId: &owner.ID}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("create resource %s: %v", name, err)
	}
	return res
}

// mah.db.each_resource_json opens the file through the handle bound to the
// caller, like get_resource_data: a caller confined to a subtree streams a
// resource inside it and cannot stream one outside it.
func TestPluginEachResourceJSONIsBoundToTheCallersScope(t *testing.T) {
	ctx := newTwoPluginContext(t, map[string]string{"jsonreader": resourceJSONPlugin})
	principal, inside := scopeProbeFixture(t, ctx)
	outside := &models.Group{Name: "far-away"}
	if err := ctx.db.Create(outside).Error; err != nil {
		t.Fatal(err)
	}
	body := `{"items": [{"v": 1}, {"v": 2}, {"v": 3}]}`
	insideRes := putJSON(t, ctx, "in-subtree", inside, body)
	outsideRes := putJSON(t, ctx, "out-of-subtree", outside, body)
	scoped := ctx.WithPrincipal(principal)

	streamAs := func(target uint) string {
		t.Helper()
		if _, err := scoped.CreateOrUpdateNote(&query_models.NoteEditor{
			NoteCreator: query_models.NoteCreator{Name: "trigger", OwnerId: inside.ID, Description: itoa(target)},
		}); err != nil {
			t.Fatalf("confined user could not create a note in its own subtree: %v", err)
		}
		return runSlot(ctx, "page_top")
	}

	if got := streamAs(insideRes.ID); got != "ok:3:6" {
		t.Fatalf("streaming a resource inside the caller's subtree reported %q", got)
	}
	if got := streamAs(outsideRes.ID); !strings.HasPrefix(got, "error:") {
		t.Fatalf("a caller confined to %q streamed a resource outside it: %q", inside.Name, got)
	}
}
