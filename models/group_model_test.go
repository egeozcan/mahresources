package models

import (
	"mahresources/models/types"
	"testing"
)

func TestGroup_Meta(t *testing.T) {
	t.Run("assign valid JSON string to Meta", func(t *testing.T) {
		g := Group{}
		jsonString := `{"field1":"value1","count":10}`
		g.Meta = types.JSON(jsonString)
		if string(g.Meta) != jsonString {
			t.Errorf("Expected Meta to be %s, got %s", jsonString, string(g.Meta))
		}
	})

	t.Run("assign empty JSON string to Meta", func(t *testing.T) {
		g := Group{}
		jsonString := ""
		g.Meta = types.JSON(jsonString)
		// types.JSON("") results in an empty []byte.
		if string(g.Meta) != "" {
			t.Errorf("Expected Meta to be empty, got %s", string(g.Meta))
		}
	})

	t.Run("assign 'null' JSON string to Meta", func(t *testing.T) {
		g := Group{}
		jsonString := "null"
		g.Meta = types.JSON(jsonString)
		if string(g.Meta) != jsonString {
			t.Errorf("Expected Meta to be %s, got %s", jsonString, string(g.Meta))
		}
	})

	t.Run("assign pre-created types.JSON to Meta", func(t *testing.T) {
		g := Group{}
		jsonData := types.JSON(`{"anotherKey":true}`)
		g.Meta = jsonData
		if string(g.Meta) != `{"anotherKey":true}` {
			t.Errorf("Expected Meta to be %s, got %s", `{"anotherKey":true}`, string(g.Meta))
		}
	})

	t.Run("assign 'invalid' JSON string literal to Meta", func(t *testing.T) {
		g := Group{}
		jsonString := "this is not valid json"
		g.Meta = types.JSON(jsonString) // types.JSON will hold these bytes
		if string(g.Meta) != jsonString {
			t.Errorf("Expected Meta to be %s, got %s", jsonString, string(g.Meta))
		}
		// Validity would be checked upon Value() or database interaction.
		// _, err := g.Meta.Value()
		// if err == nil {
		// 	 t.Errorf("Expected error when calling Value on an invalid JSON segment, but got nil")
		// }
	})
}

// Other Group model methods (GetId, GetName, etc.) can be tested here
// but are not directly related to the Meta field's JSON nature.
[end of models/group_model_test.go]
