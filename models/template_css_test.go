package models

import "testing"

func TestTemplateCSSCascade(t *testing.T) {
	const shared = ".card{color:red}"
	const header = ".card{color:green}"
	const result = ".card{color:blue}"
	carriers := []interface{ TemplateCSS() string }{
		Category{CustomCSS: shared, CustomHeaderCSS: header, CustomMRQLResultCSS: result},
		ResourceCategory{CustomCSS: shared, CustomHeaderCSS: header, CustomMRQLResultCSS: result},
		NoteType{CustomCSS: shared, CustomHeaderCSS: header, CustomMRQLResultCSS: result},
	}
	for _, carrier := range carriers {
		if got := carrier.TemplateCSS(); got != shared+"\n"+header+"\n"+result {
			t.Errorf("%T: unexpected cascade: %s", carrier, got)
		}
	}
}
