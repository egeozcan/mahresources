package http_utils

import (
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestRemoveValue(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		item  string
		want  []string
	}{
		{
			name:  "remove existing item",
			items: []string{"a", "b", "c"},
			item:  "b",
			want:  []string{"a", "c"},
		},
		{
			name:  "remove non-existing item",
			items: []string{"a", "b", "c"},
			item:  "d",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "remove from empty slice",
			items: []string{},
			item:  "a",
			want:  nil,
		},
		{
			name:  "remove from nil slice",
			items: nil,
			item:  "a",
			want:  nil,
		},
		{
			name:  "remove all occurrences",
			items: []string{"a", "b", "a", "c", "a"},
			item:  "a",
			want:  []string{"b", "c"},
		},
		{
			name:  "remove first item",
			items: []string{"a", "b", "c"},
			item:  "a",
			want:  []string{"b", "c"},
		},
		{
			name:  "remove last item",
			items: []string{"a", "b", "c"},
			item:  "c",
			want:  []string{"a", "b"},
		},
		{
			name:  "remove only item",
			items: []string{"a"},
			item:  "a",
			want:  nil,
		},
		{
			name:  "empty string removal",
			items: []string{"a", "", "c"},
			item:  "",
			want:  []string{"a", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemoveValue(tt.items, tt.item)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RemoveValue(%v, %q) = %v, want %v", tt.items, tt.item, got, tt.want)
			}
		})
	}
}

func TestGetQueryParameter(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		param    string
		defVal   string
		want     string
	}{
		{
			name:   "parameter exists",
			url:    "/test?name=value",
			param:  "name",
			defVal: "default",
			want:   "value",
		},
		{
			name:   "parameter missing",
			url:    "/test?other=value",
			param:  "name",
			defVal: "default",
			want:   "default",
		},
		{
			name:   "empty parameter value returns default",
			url:    "/test?name=",
			param:  "name",
			defVal: "default",
			want:   "default",
		},
		{
			name:   "no query string",
			url:    "/test",
			param:  "name",
			defVal: "default",
			want:   "default",
		},
		{
			name:   "multiple parameters",
			url:    "/test?a=1&b=2&c=3",
			param:  "b",
			defVal: "default",
			want:   "2",
		},
		{
			name:   "special characters in value",
			url:    "/test?name=hello%20world",
			param:  "name",
			defVal: "default",
			want:   "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			got := GetQueryParameter(req, tt.param, tt.defVal)
			if got != tt.want {
				t.Errorf("GetQueryParameter(req, %q, %q) = %q, want %q", tt.param, tt.defVal, got, tt.want)
			}
		})
	}
}

func TestGetIntQueryParameter(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		param  string
		defVal int64
		want   int64
	}{
		{
			name:   "valid integer",
			url:    "/test?id=42",
			param:  "id",
			defVal: 0,
			want:   42,
		},
		{
			name:   "negative integer",
			url:    "/test?id=-5",
			param:  "id",
			defVal: 0,
			want:   -5,
		},
		{
			name:   "zero value",
			url:    "/test?id=0",
			param:  "id",
			defVal: 99,
			want:   0,
		},
		{
			name:   "missing parameter",
			url:    "/test",
			param:  "id",
			defVal: 99,
			want:   99,
		},
		{
			name:   "invalid integer",
			url:    "/test?id=abc",
			param:  "id",
			defVal: 99,
			want:   99,
		},
		{
			name:   "float value truncates",
			url:    "/test?id=3.14",
			param:  "id",
			defVal: 99,
			want:   99, // ParseInt fails on floats
		},
		{
			name:   "empty value",
			url:    "/test?id=",
			param:  "id",
			defVal: 99,
			want:   99,
		},
		{
			name:   "large number",
			url:    "/test?id=9223372036854775807",
			param:  "id",
			defVal: 0,
			want:   9223372036854775807,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			got := GetIntQueryParameter(req, tt.param, tt.defVal)
			if got != tt.want {
				t.Errorf("GetIntQueryParameter(req, %q, %d) = %d, want %d", tt.param, tt.defVal, got, tt.want)
			}
		})
	}
}

func TestGetUIntQueryParameter(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		param  string
		defVal uint
		want   uint
	}{
		{
			name:   "valid unsigned integer",
			url:    "/test?id=42",
			param:  "id",
			defVal: 0,
			want:   42,
		},
		{
			name:   "zero value",
			url:    "/test?id=0",
			param:  "id",
			defVal: 99,
			want:   0,
		},
		{
			name:   "missing parameter",
			url:    "/test",
			param:  "id",
			defVal: 99,
			want:   99,
		},
		{
			name:   "negative integer returns default",
			url:    "/test?id=-5",
			param:  "id",
			defVal: 99,
			want:   99,
		},
		{
			name:   "invalid value",
			url:    "/test?id=abc",
			param:  "id",
			defVal: 99,
			want:   99,
		},
		{
			name:   "empty value",
			url:    "/test?id=",
			param:  "id",
			defVal: 99,
			want:   99,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			got := GetUIntQueryParameter(req, tt.param, tt.defVal)
			if got != tt.want {
				t.Errorf("GetUIntQueryParameter(req, %q, %d) = %d, want %d", tt.param, tt.defVal, got, tt.want)
			}
		})
	}
}

func TestRequestAcceptsHTML(t *testing.T) {
	tests := []struct {
		name   string
		accept string
		want   bool
	}{
		{
			name:   "accepts text/html",
			accept: "text/html",
			want:   true,
		},
		{
			name:   "accepts text/html with charset",
			accept: "text/html; charset=utf-8",
			want:   true,
		},
		{
			name:   "accepts multiple including html",
			accept: "application/json, text/html, */*",
			want:   true,
		},
		{
			name:   "application/json only",
			accept: "application/json",
			want:   false,
		},
		{
			name:   "empty accept header",
			accept: "",
			want:   false,
		},
		{
			name:   "wildcard",
			accept: "*/*",
			want:   false,
		},
		{
			name:   "text/plain",
			accept: "text/plain",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			got := requestAcceptsHTML(req)
			if got != tt.want {
				t.Errorf("requestAcceptsHTML(req with Accept=%q) = %v, want %v", tt.accept, got, tt.want)
			}
		})
	}
}
