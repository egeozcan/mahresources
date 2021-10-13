package template_entities

import "testing"

func TestGeneratePagination(t *testing.T) {
	url := "http://www.example.com?a=1&page=11"
	pagination, err := GeneratePagination(url, 600, 10, 12)

	if err != nil {
		t.Errorf("Failed while parsing url: %v, got error %v", url, err)
	}

	for i, link := range *pagination.Entries {
		if link.Display == "12" && !link.Selected {
			t.Error("Current page not selected")
		}

		t.Log(i, link)
	}
}
