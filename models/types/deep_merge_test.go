package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func parse(s string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		panic(err)
	}
	return m
}

func TestDeepMerge_IncomingWins(t *testing.T) {
	base := parse(`{"a": 1, "b": 2}`)
	incoming := parse(`{"b": 99, "c": 3}`)
	result := DeepMergeJSON(base, incoming)
	expected := parse(`{"a": 1, "b": 99, "c": 3}`)
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("got %v, want %v", result, expected)
	}
}

func TestDeepMerge_NestedMerge(t *testing.T) {
	base := parse(`{"nested": {"x": 1, "y": 2}, "top": "base"}`)
	incoming := parse(`{"nested": {"y": 99, "z": 3}, "top": "incoming"}`)
	result := DeepMergeJSON(base, incoming)
	expected := parse(`{"nested": {"x": 1, "y": 99, "z": 3}, "top": "incoming"}`)
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("got %v, want %v", result, expected)
	}
}

func TestDeepMerge_IncomingOverwritesNonMap(t *testing.T) {
	base := parse(`{"a": {"nested": true}}`)
	incoming := parse(`{"a": "string_now"}`)
	result := DeepMergeJSON(base, incoming)
	expected := parse(`{"a": "string_now"}`)
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("got %v, want %v", result, expected)
	}
}

func TestDeepMerge_NilBase(t *testing.T) {
	incoming := parse(`{"a": 1}`)
	result := DeepMergeJSON(nil, incoming)
	if !reflect.DeepEqual(result, incoming) {
		t.Fatalf("got %v, want %v", result, incoming)
	}
}

func TestDeepMerge_NilIncoming(t *testing.T) {
	base := parse(`{"a": 1}`)
	result := DeepMergeJSON(base, nil)
	if !reflect.DeepEqual(result, base) {
		t.Fatalf("got %v, want %v", result, base)
	}
}
