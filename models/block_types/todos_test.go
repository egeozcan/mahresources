package block_types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeTodosState(t *testing.T) {
	content := json.RawMessage(`{"items":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}`)

	tests := []struct {
		name    string
		content json.RawMessage
		state   json.RawMessage
		want    string
		wantErr bool
	}{
		{name: "keeps only item IDs", content: content, state: json.RawMessage(`{"checked":["a","x"]}`), want: `{"checked":["a"]}`},
		{name: "drops unknown keys", content: content, state: json.RawMessage(`{"checked":["b"],"junk":"zzz"}`), want: `{"checked":["b"]}`},
		{name: "de-duplicates preserving order", content: content, state: json.RawMessage(`{"checked":["b","a","b"]}`), want: `{"checked":["b","a"]}`},
		{name: "empty checked marshals as array", content: content, state: json.RawMessage(`{"checked":[]}`), want: `{"checked":[]}`},
		{name: "missing checked marshals as array", content: content, state: json.RawMessage(`{}`), want: `{"checked":[]}`},
		{name: "no item IDs left", content: content, state: json.RawMessage(`{"checked":["x","y"]}`), want: `{"checked":[]}`},
		{name: "empty content has no items", content: json.RawMessage(`{"items":[]}`), state: json.RawMessage(`{"checked":["a"]}`), want: `{"checked":[]}`},
		{name: "malformed state", content: content, state: json.RawMessage(`{"checked":`), wantErr: true},
		{name: "checked of wrong type", content: content, state: json.RawMessage(`{"checked":5}`), wantErr: true},
		{name: "checked entry of wrong type", content: content, state: json.RawMessage(`{"checked":[1]}`), wantErr: true},
		{name: "malformed content", content: json.RawMessage(`{"items":`), state: json.RawMessage(`{"checked":[]}`), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeTodosState(tt.content, tt.state)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}
