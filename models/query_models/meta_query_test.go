package query_models

import (
	"fmt"
	"testing"
)

func TestParseMetaArray(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  ColumnMeta
	}{
		{
			name:  "test:EQ:1",
			input: "test:EQ:1",
			want: ColumnMeta{
				Key:       "test",
				Value:     float64(1),
				Operation: "EQ",
			},
		},
		{
			name:  "foo:LT:\"test\"",
			input: "foo:LT:\"test\"",
			want: ColumnMeta{
				Key:       "foo",
				Value:     "test",
				Operation: "LT",
			},
		},
		{
			name:  "foo:LT:test",
			input: "foo:LT:test",
			want: ColumnMeta{
				Key:       "foo",
				Value:     "test",
				Operation: "LT",
			},
		},
		// Values that contain colons (URLs, timestamps) must not be silently dropped
		{
			name:  "value with colon (URL) and explicit op",
			input: "site:EQ:https://example.com:8080/path",
			want: ColumnMeta{
				Key:       "site",
				Value:     "https://example.com:8080/path",
				Operation: "EQ",
			},
		},
		{
			name:  "value with colon (URL) default op",
			input: "site:https://example.com",
			want: ColumnMeta{
				Key:       "site",
				Value:     "https://example.com",
				Operation: "LI",
			},
		},
		{
			name:  "value with colon (timestamp)",
			input: "time:EQ:14:30:00",
			want: ColumnMeta{
				Key:       "time",
				Value:     "14:30:00",
				Operation: "EQ",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseMeta(tt.input)
			valueWanted := tt.want.Value
			valueGot := got.Value
			valuesDoNotMatch := fmt.Sprintf("%v", valueWanted) != fmt.Sprintf("%v", valueGot)
			keysDoNotMatch := tt.want.Key != got.Key
			operationDoesNotMatch := tt.want.Operation != got.Operation
			if valuesDoNotMatch || keysDoNotMatch || operationDoesNotMatch {
				t.Errorf("ParseMetaArray() = %v, want %v", got, tt.want)
			}
		})
	}
}
