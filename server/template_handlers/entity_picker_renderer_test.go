package template_handlers

import (
	"mahresources/contracts"
	"mahresources/models"
	"strings"
	"testing"
)

func TestEntityPickerDefaultEscapesAndKeepsSelectionOutside(t *testing.T) {
	t.Chdir("../..")
	row := contracts.EntityPickerEntity{ID: 7, Name: "<script>name</script>", Raw: &models.Resource{}}
	html, err := RenderEntityPickerDefault("resource", row)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`target="_blank"`, `rel="noopener noreferrer"`, `&lt;script&gt;`, `/v1/resource/preview?id=7`} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %s: %s", want, html)
		}
	}
	if strings.Contains(html, `type="checkbox"`) || strings.Contains(html, "<script>") {
		t.Fatal(html)
	}
}
