package template_context_providers

import "github.com/gorilla/schema"

var decoder = schema.NewDecoder()

func init() {
	decoder.IgnoreUnknownKeys(true)
}
