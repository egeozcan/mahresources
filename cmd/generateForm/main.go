package main

import (
	"encoding/json"
	"fmt"
	"github.com/invopop/jsonschema"
)

// Struct for the JSON document
type Document struct {
	Name    string   `json:"name"`
	Age     int      `json:"age"`
	IsAdult bool     `json:"isAdult"`
	Hobbies []string `json:"hobbies"`
}

// Function to get JSON schema from a struct
func getJSONSchema(document interface{}) *jsonschema.Schema {
	schema := jsonschema.Reflect(document)
	return schema
}

// Function to get HTML form from an object JSON schema
// supports integer, boolean, string and arrays of these, with all possible options
func getHTMLForm(schema *jsonschema.Schema, jsonData map[string]interface{}) string {
	form := "<form>\n"
	for _, key := range schema.Properties.Keys() {
		prop, _ := schema.Properties.Get(key)

	}
	form += "</form>\n"
	return form
}

func main() {
	doc := Document{
		Name:    "John Doe",
		Age:     30,
		IsAdult: true,
		Hobbies: []string{"football", "reading", "gaming"},
	}

	// Getting JSON schema from the struct
	schema := getJSONSchema(doc)

	// Converting the struct to map for simplicity
	docMap := make(map[string]interface{})
	inrec, _ := json.Marshal(doc)
	json.Unmarshal(inrec, &docMap)

	// Getting HTML form from the JSON schema
	formHTML := getHTMLForm(schema, docMap)
	fmt.Println(formHTML)
}
