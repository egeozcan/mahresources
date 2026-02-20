package types

import (
	"database/sql/driver"
	"fmt"
	"net/url"
)

type URL url.URL

func (u *URL) Scan(value any) error {
	if value == nil {
		return nil
	}

	var s string
	switch v := value.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return fmt.Errorf("unsupported type for URL.Scan: %T", value)
	}

	parsed, err := url.Parse(s)
	if err != nil {
		return err
	}

	*u = URL(*parsed)

	return nil
}

// Value return json value, implement driver.Valuer interface
func (u URL) Value() (driver.Value, error) {
	urlObj := url.URL(u)
	return urlObj.String(), nil
}
