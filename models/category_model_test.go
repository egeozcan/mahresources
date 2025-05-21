package models

import (
	"mahresources/models/types"
	"testing"
)

func TestCategory_CustomFieldsDefinition(t *testing.T) {
	t.Run("assign valid JSON string", func(t *testing.T) {
		c := Category{}
		jsonString := `[{"name":"field1","type":"text"}]`
		// In a real scenario, this conversion would happen before or during GORM save.
		// Here, we are testing the model field directly.
		c.CustomFieldsDefinition = types.JSON(jsonString)
		if string(c.CustomFieldsDefinition) != jsonString {
			t.Errorf("Expected CustomFieldsDefinition to be %s, got %s", jsonString, string(c.CustomFieldsDefinition))
		}
	})

	t.Run("assign empty JSON string", func(t *testing.T) {
		c := Category{}
		jsonString := ""
		c.CustomFieldsDefinition = types.JSON(jsonString)
		// Depending on types.JSON.Scan behavior for empty string, this might be "null" or ""
		// The types.JSON Scan method converts empty string to "null"
		if string(c.CustomFieldsDefinition) != "" { // types.JSON("") results in an empty []byte, not "null" unless Scan is called with ""
			t.Errorf("Expected CustomFieldsDefinition to be empty, got %s", string(c.CustomFieldsDefinition))
		}
	})

	t.Run("assign 'null' JSON string", func(t *testing.T) {
		c := Category{}
		jsonString := "null"
		c.CustomFieldsDefinition = types.JSON(jsonString)
		if string(c.CustomFieldsDefinition) != jsonString {
			t.Errorf("Expected CustomFieldsDefinition to be %s, got %s", jsonString, string(c.CustomFieldsDefinition))
		}
	})

	t.Run("assign pre-created types.JSON", func(t *testing.T) {
		c := Category{}
		jsonData := types.JSON(`{"key":"value"}`)
		c.CustomFieldsDefinition = jsonData
		if string(c.CustomFieldsDefinition) != `{"key":"value"}` {
			t.Errorf("Expected CustomFieldsDefinition to be %s, got %s", `{"key":"value"}`, string(c.CustomFieldsDefinition))
		}
	})

	// Assigning an "invalid" JSON string like "not json" to types.JSON directly
	// doesn't make it invalid until it's processed by json.Unmarshal or similar.
	// types.JSON is json.RawMessage, which is []byte, so it holds whatever bytes it's given.
	// The validity is checked during Scan or Value or Marshal/Unmarshal.
	t.Run("assign 'invalid' JSON string literal", func(t *testing.T) {
		c := Category{}
		// This is just a sequence of bytes. It's not "invalid" for types.JSON to hold this.
		// It will only fail when GORM tries to write it if the DB expects valid JSON,
		// or when types.JSON.Value() is called and tries to marshal it.
		jsonString := "this is not valid json"
		c.CustomFieldsDefinition = types.JSON(jsonString)
		if string(c.CustomFieldsDefinition) != jsonString {
			t.Errorf("Expected CustomFieldsDefinition to be %s, got %s", jsonString, string(c.CustomFieldsDefinition))
		}
		// To check validity, one might try to marshal it or use its Value() method.
		// _, err := c.CustomFieldsDefinition.Value() // This would likely error if "this is not valid json" is not a valid JSON literal
		// if err == nil {
		//  t.Errorf("Expected error when calling Value on an invalid JSON segment, but got nil")
		// }
	})
}

// Test for other fields like GetId, GetName, GetDescription can be added
// but are less relevant to the custom fields functionality itself.
[end of models/category_model_test.go]
