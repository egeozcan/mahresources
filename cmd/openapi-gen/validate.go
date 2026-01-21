//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/getkin/kin-openapi/openapi3"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: go run validate.go <spec.yaml>")
		os.Exit(1)
	}

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Load error: %v\n", err)
		os.Exit(1)
	}

	err = doc.Validate(loader.Context)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Validation error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Valid OpenAPI 3.0 spec\n")
	fmt.Printf("Paths: %d\n", doc.Paths.Len())
	fmt.Printf("Schemas: %d\n", len(doc.Components.Schemas))
	fmt.Printf("Tags: %d\n", len(doc.Tags))
}
