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
