package template_entities

import "testing"

func TestGeneratePagination(t *testing.T) {
	url := "http://www.example.com"
	pagination, err := GeneratePagination(url, 600, 10, 12)

	if err != nil {
		t.Errorf("Failed while parsing url: %v, got error %v", url, err)
	}

	for a, i := range *pagination.Entries {
		t.Log(a, i)
	}
}
