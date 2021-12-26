package types

import (
	"database/sql/driver"
	"net/url"
)

type URL url.URL

func (u *URL) Scan(value interface{}) error {
	url, err := url.Parse(value.(string))

	if err != nil {
		return err
	}

	*u = URL(*url)

	return nil
}

// Value return json value, implement driver.Valuer interface
func (u URL) Value() (driver.Value, error) {
	urlObj := url.URL(u)
	return urlObj.String(), nil
}
