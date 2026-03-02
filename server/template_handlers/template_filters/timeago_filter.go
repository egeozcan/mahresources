package template_filters

import (
	"fmt"
	"time"

	"github.com/flosch/pongo2/v4"
)

func timeagoFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	t, ok := in.Interface().(time.Time)
	if !ok {
		return pongo2.AsValue(""), nil
	}

	duration := time.Since(t)

	switch {
	case duration < time.Minute:
		return pongo2.AsValue("just now"), nil
	case duration < time.Hour:
		mins := int(duration.Minutes())
		if mins == 1 {
			return pongo2.AsValue("1 minute ago"), nil
		}
		return pongo2.AsValue(fmt.Sprintf("%d minutes ago", mins)), nil
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		if hours == 1 {
			return pongo2.AsValue("1 hour ago"), nil
		}
		return pongo2.AsValue(fmt.Sprintf("%d hours ago", hours)), nil
	case duration < 30*24*time.Hour:
		days := int(duration.Hours() / 24)
		if days == 1 {
			return pongo2.AsValue("1 day ago"), nil
		}
		return pongo2.AsValue(fmt.Sprintf("%d days ago", days)), nil
	default:
		return pongo2.AsValue(t.Format("2006-01-02")), nil
	}
}
