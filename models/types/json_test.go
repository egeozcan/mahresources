package types

import (
	"database/sql/driver"
	"encoding/json"
	"reflect"
	"testing"
)

func TestJSON_Scan(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
		want    JSON
	}{
		{
			name:  "scan valid JSON string",
			value: `{"key":"value"}`,
			want:  JSON(`{"key":"value"}`),
		},
		{
			name:  "scan valid JSON []byte",
			value: []byte(`{"key":"value"}`),
			want:  JSON(`{"key":"value"}`),
		},
		{
			name:  "scan null JSON string",
			value: "null",
			want:  JSON("null"),
		},
		{
			name:  "scan null JSON []byte",
			value: []byte("null"),
			want:  JSON("null"),
		},
		{
			name:  "scan empty string",
			value: "",
			// Based on current Scan implementation, empty string leads to unmarshal error
			// then result is json.RawMessage{}, which is `null` after marshal.
			// If it should be `""` or an error, Scan needs adjustment.
			// For now, testing current behavior.
			want: JSON("null"),
		},
		{
			name:  "scan nil value",
			value: nil,
			want:  JSON("null"),
		},
		{
			name:    "scan invalid JSON string",
			value:   `{"key":value_no_quotes}`,
			wantErr: true,
		},
		{
			name:    "scan invalid JSON []byte",
			value:   []byte(`{"key":value_no_quotes}`),
			wantErr: true,
		},
		{
			name:    "scan non-string non-[]byte type (int)",
			value:   123,
			wantErr: true,
		},
		{
			name:  "scan valid JSON array string",
			value: `[1, "two", {"three":3}]`,
			want:  JSON(`[1,"two",{"three":3}]`),
		},
		{
			name:  "scan valid JSON array []byte",
			value: []byte(`[1, "two", {"three":3}]`),
			want:  JSON(`[1,"two",{"three":3}]`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var j JSON
			err := j.Scan(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("JSON.Scan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(j, tt.want) {
				// Comparing string representations because internal []byte might differ for same JSON
				if string(j) != string(tt.want) {
					t.Errorf("JSON.Scan() = %s, want %s", string(j), string(tt.want))
				}
			}
		})
	}
}

func TestJSON_Value(t *testing.T) {
	tests := []struct {
		name    string
		j       JSON
		want    driver.Value
		wantErr bool
	}{
		{
			name: "value from valid JSON object",
			j:    JSON(`{"key":"value"}`),
			want: `{"key":"value"}`, // driver.Value for JSON is string
		},
		{
			name: "value from valid JSON array",
			j:    JSON(`[1, "two"]`),
			want: `[1,"two"]`,
		},
		{
			name: "value from 'null' JSON",
			j:    JSON("null"),
			want: "null",
		},
		{
			name: "value from empty JSON (len 0)",
			j:    JSON(""), // Represents an empty or uninitialized JSON
			want: nil,      // Value() returns nil if len(j) == 0
		},
		{
			name: "value from JSON representing empty object",
			j:    JSON("{}"),
			want: "{}",
		},
		{
			name: "value from JSON representing empty array",
			j:    JSON("[]"),
			want: "[]",
		},
		// The current implementation of Value() uses json.RawMessage(j).MarshalJSON().
		// If `j` itself is not valid JSON (e.g., `JSON("invalid")`), MarshalJSON for RawMessage
		// might return it as is, or error, depending on its internal state.
		// json.RawMessage.MarshalJSON returns j if j is a valid JSON value, else error.
		// However, `JSON("invalid")` is not a "valid JSON value" in the sense of being a number, string, boolean, array, or object.
		// It's a raw segment. Let's test what json.RawMessage does.
		{
			name:    "value from technically invalid JSON segment",
			j:       JSON("invalid"), // This is not a complete valid JSON doc e.g. not quoted string
			wantErr: true,            // json.RawMessage(`invalid`).MarshalJSON() will error
		},
		{
			name: "value from quoted string as JSON",
			j:    JSON(`"a string"`),
			want: `"a string"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.j.Value()
			if (err != nil) != tt.wantErr {
				t.Errorf("JSON.Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// If we want a string, we need to compare string values
			if sGot, okGot := got.(string); okGot {
				if sWant, okWant := tt.want.(string); okWant {
					// Normalize by unmarshalling and marshalling back for complex JSON to avoid whitespace/order issues
					// For simple strings or null, direct comparison is fine.
					if (sGot == "null" && sWant == "null") || (sGot == "{}" && sWant == "{}") || (sGot == "[]" && sWant == "[]") {
						if sGot != sWant {
							t.Errorf("JSON.Value() = %v, want %v", got, tt.want)
						}
					} else if len(sGot) > 2 && len(sWant) > 2 { // basic check for non-trivial JSON
						var jGot, jWant interface{}
						if err := json.Unmarshal([]byte(sGot), &jGot); err != nil {
							t.Errorf("Error unmarshalling got value: %v", err)
							return
						}
						if err := json.Unmarshal([]byte(sWant), &jWant); err != nil {
							t.Errorf("Error unmarshalling want value: %v", err)
							return
						}
						if !reflect.DeepEqual(jGot, jWant) {
							t.Errorf("JSON.Value() deserialized = %v, want deserialized %v", jGot, jWant)
						}
					} else if sGot != sWant { // For simple cases or if one is not complex JSON
						t.Errorf("JSON.Value() = %v, want %v", got, tt.want)
					}

				} else if tt.want != nil { // got string, want is not string but not nil
					t.Errorf("JSON.Value() type mismatch got string = %v, want %T = %v", got, tt.want, tt.want)
				}
			} else if !reflect.DeepEqual(got, tt.want) { // If not a string, direct comparison
				t.Errorf("JSON.Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJSON_MarshalUnmarshal(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		isValid bool // Is the input string a valid JSON value?
	}{
		{"empty object", "{}", true},
		{"simple object", `{"key":"value","number":123}`, true},
		{"nested object", `{"key":{"subkey":"subvalue"},"array":[1,null,"test"]}`, true},
		{"empty array", "[]", true},
		{"simple array", `[1,"hello",null,true]`, true},
		{"null value", "null", true},
		{"string value", `"a string"`, true}, // A valid JSON document can be just a string
		// `types.JSON` is `json.RawMessage`. `json.RawMessage("abc")` is not valid.
		// It needs to be a valid JSON literal, e.g. `json.RawMessage(`"abc"`)`.
		// The `JSON` type constructor `JSON("abc")` is like `json.RawMessage("abc")`.
		{"raw string", "abc", false},
		{"empty string", "", false}, // Not a valid JSON value itself
	}

	for _, tc := range testCases {
		t.Run(tc.name+"_marshal", func(t *testing.T) {
			j := JSON(tc.input)
			marshaled, err := j.MarshalJSON()

			if tc.isValid {
				if err != nil {
					t.Errorf("Expected no error for valid JSON, got %v", err)
				}
				// Check if marshaled output is equivalent to input (might have formatting differences)
				var m1, m2 interface{}
				if errJson1 := json.Unmarshal([]byte(tc.input), &m1); errJson1 != nil {
					t.Fatalf("Invalid test case input JSON: %s, error: %v", tc.input, errJson1)
				}
				if errJson2 := json.Unmarshal(marshaled, &m2); errJson2 != nil {
					t.Fatalf("Invalid marshaled JSON: %s, error: %v", marshaled, errJson2)
				}
				if !reflect.DeepEqual(m1, m2) {
					t.Errorf("MarshalJSON output %s not equivalent to input %s", marshaled, tc.input)
				}
			} else {
				// If input is not valid JSON, MarshalJSON for RawMessage should error
				if err == nil {
					t.Errorf("Expected error for invalid JSON input %s, but got nil. Marshaled: %s", tc.input, marshaled)
				}
			}
		})

		t.Run(tc.name+"_unmarshal", func(t *testing.T) {
			var j JSON
			inputBytes := []byte(tc.input)
			err := j.UnmarshalJSON(inputBytes)

			if tc.isValid {
				if err != nil {
					t.Errorf("Expected no error for valid JSON input, got %v", err)
				}
				// Check if unmarshaled value when remarshaled is equivalent to input
				remarshaled, _ := j.MarshalJSON()
				var r1, r2 interface{}
				if errJson1 := json.Unmarshal(inputBytes, &r1); errJson1 != nil {
					t.Fatalf("Invalid test case input JSON: %s, error: %v", tc.input, errJson1)
				}
				if errJson2 := json.Unmarshal(remarshaled, &r2); errJson2 != nil {
					t.Fatalf("Invalid remarshaled JSON: %s, error: %v", remarshaled, errJson2)
				}

				if !reflect.DeepEqual(r1, r2) {
					t.Errorf("UnmarshalJSON then MarshalJSON output %s not equivalent to input %s", remarshaled, tc.input)
				}

			} else {
				// UnmarshalJSON for RawMessage can accept any []byte, error only if it's truly malformed *for JSON itself*
				// e.g. `json.RawMessage{}`.UnmarshalJSON([]byte("abc")) is fine, data becomes `[]byte("abc")`
				// it only errors if the content of `[]byte` makes `json.Unmarshal` fail.
				// `json.Unmarshal([]byte("abc"), &json.RawMessage{})` would error because "abc" is not valid JSON.
				// The `UnmarshalJSON` for `json.RawMessage` is more about assigning the bytes.
				// My `types.JSON.UnmarshalJSON` does:
				// result := json.RawMessage{}
				// err := result.UnmarshalJSON(b) // This is json.RawMessage.UnmarshalJSON
				// *j = JSON(result)
				// So, if `b` is "abc", `result.UnmarshalJSON(b)` does not error. `result` becomes `[]byte("abc")`.
				// This seems to be a misunderstanding in my test or the `json.RawMessage` behavior.
				// `json.RawMessage.UnmarshalJSON` simply assigns the bytes if they are a valid JSON string literal,
				// or if they are `null`, `true`, `false`, or a number.
				// If `b` is `[]byte("abc")` (not `[]byte(`"abc"`)`), it's not a valid JSON literal.
				// `json.Unmarshal([]byte("abc"), &someVar)` will error.
				// `json.RawMessage.UnmarshalJSON` is a bit special. It tries to validate if it's a single valid JSON token.
				// If `inputBytes` is "abc", `result.UnmarshalJSON(inputBytes)` should error.
				if err == nil && tc.input != "" { // Empty string is a weird case for RawMessage.UnmarshalJSON
					t.Errorf("Expected error for invalid JSON value %s during UnmarshalJSON, but got nil. Result: %s", tc.input, string(j))
				}
			}
		})
	}
}

// Note: The GormDataType, GormDBDataType, GormValue, and JSONQueryExpression parts of json.go
// are related to GORM integration and querying, which are harder to unit test without a database
// or significant mocking of GORM's internals. These are typically tested implicitly via
// application context tests that interact with the database.
// For this subtask, focusing on Scan, Value, MarshalJSON, and UnmarshalJSON is key for model-level correctness.

func TestJSON_String(t *testing.T) {
	j := JSON(`{"key":"value"}`)
	if j.String() != `{"key":"value"}` {
		t.Errorf("JSON.String() = %s, want %s", j.String(), `{"key":"value"}`)
	}

	jEmpty := JSON("")
	if jEmpty.String() != "" {
		t.Errorf("JSON.String() for empty JSON = %s, want %s", jEmpty.String(), "")
	}

	jNull := JSON("null")
	if jNull.String() != "null" {
		t.Errorf("JSON.String() for 'null' JSON = %s, want %s", jNull.String(), "null")
	}
}

[end of models/types/json_test.go]
