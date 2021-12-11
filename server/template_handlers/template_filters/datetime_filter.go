package template_filters

import (
	"github.com/flosch/pongo2/v4"
	"time"
)

//goland:noinspection GoUnusedParameter
func filterDateTime(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	t, isTime := in.Interface().(*time.Time)
	if !isTime || t == nil {
		return pongo2.AsValue(""), nil
	}
	return pongo2.AsValue(t.Format("2006-01-02T03:04")), nil
}
