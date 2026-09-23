package contracts

import (
	"encoding/json"
	"io"
)

// JobOutputContent is the transport-neutral content returned after a Job output
// has been authorized and resolved. Exactly one of Body, Data or Location is
// generally populated.
type JobOutputContent struct {
	Body        io.ReadCloser
	Data        json.RawMessage
	Location    string
	ContentType string
	Filename    string
	Inline      bool
}
