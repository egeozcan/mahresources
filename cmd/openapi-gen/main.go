// Command openapi-gen generates the OpenAPI specification from code.
//
// Usage:
//
//	go run ./cmd/openapi-gen
//
// Or with go generate:
//
//	//go:generate go run ./cmd/openapi-gen
package main

import (
	"flag"
	"fmt"
	"os"

	"mahresources/server"
	"mahresources/server/openapi"
)

func main() {
	outputFile := flag.String("output", "openapi.yaml", "Output file path")
	format := flag.String("format", "yaml", "Output format: yaml or json")
	flag.Parse()

	// Create registry and register all routes
	registry := openapi.NewRegistry()
	server.RegisterAPIRoutesWithOpenAPI(registry)

	// Generate spec
	var data []byte
	var err error

	if *format == "json" {
		data, err = registry.MarshalJSON()
	} else {
		data, err = registry.MarshalYAML()
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating spec: %v\n", err)
		os.Exit(1)
	}

	// Write to file
	err = os.WriteFile(*outputFile, data, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OpenAPI spec generated: %s\n", *outputFile)
}
