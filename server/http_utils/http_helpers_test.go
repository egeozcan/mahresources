package http_utils

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestHandleErrorHTMLContainsStyling(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/test", nil)
	req.Header.Set("Accept", "text/html")

	HandleError(errors.New("test error"), w, req, http.StatusBadRequest)

	body := w.Body.String()
	// Should contain "occurred" not "occured"
	if strings.Contains(body, "occured") {
		t.Errorf("body should not contain the typo 'occured'")
	}
	if !strings.Contains(body, "occurred") {
		t.Errorf("body should contain 'occurred'")
	}
	if !strings.Contains(body, "test error") {
		t.Errorf("body should contain the error message 'test error'")
	}
	// Should have some styling
	if !strings.Contains(body, "<style") && !strings.Contains(body, ".css") {
		t.Errorf("body should contain styling (<style> or .css link)")
	}
}

func TestHandleErrorHTMLUseCorrectCSSPaths(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/test", nil)
	req.Header.Set("Accept", "text/html")

	HandleError(errors.New("test error"), w, req, http.StatusConflict)

	body := w.Body.String()
	// CSS hrefs must include the /public/ prefix so the browser can find them
	if strings.Contains(body, `href="/tailwind.css"`) {
		t.Error("CSS path should be /public/tailwind.css, not /tailwind.css")
	}
	if strings.Contains(body, `href="/index.css"`) {
		t.Error("CSS path should be /public/index.css, not /index.css")
	}
	if !strings.Contains(body, `/public/tailwind.css`) {
		t.Error("body should reference /public/tailwind.css")
	}
	if !strings.Contains(body, `/public/index.css`) {
		t.Error("body should reference /public/index.css")
	}
}

func TestHandleErrorJSONUnchanged(t *testing.T) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/test", nil)
	req.Header.Set("Accept", "application/json")

	HandleError(errors.New("test error"), w, req, http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	var result map[string]string
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if result["error"] != "test error" {
		t.Errorf("expected error 'test error', got %q", result["error"])
	}
}

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

func TestGetPageParameterClampsOverflow(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want int64
	}{
		{
			name: "page=1",
			url:  "/test?page=1",
			want: 1,
		},
		{
			name: "page=0 clamped to 1",
			url:  "/test?page=0",
			want: 1,
		},
		{
			name: "page=-1 clamped to 1",
			url:  "/test?page=-1",
			want: 1,
		},
		{
			name: "page missing defaults to 1",
			url:  "/test",
			want: 1,
		},
		{
			name: "normal page",
			url:  "/test?page=42",
			want: 42,
		},
		{
			name: "max int64 clamped to maxPage",
			url:  "/test?page=9223372036854775807",
			want: maxPage,
		},
		{
			name: "large page clamped to maxPage",
			url:  "/test?page=368934881474191033",
			want: maxPage,
		},
		{
			name: "page at maxPage boundary",
			url:  "/test?page=1000000000",
			want: 1000000000,
		},
		{
			name: "page just above maxPage clamped",
			url:  "/test?page=1000000001",
			want: maxPage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			got := GetPageParameter(req)
			if got != tt.want {
				t.Errorf("GetPageParameter(req) = %d, want %d", got, tt.want)
			}
			// Verify no overflow: (page-1)*200 must be non-negative
			offset := (got - 1) * 200
			if offset < 0 {
				t.Errorf("offset = (page-1)*200 = %d is negative, indicating overflow", offset)
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

func TestIsSafeRedirect(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "valid absolute path",
			url:  "/groups",
			want: true,
		},
		{
			name: "valid path with query string",
			url:  "/resource?id=42",
			want: true,
		},
		{
			name: "valid path with fragment",
			url:  "/notes#section",
			want: true,
		},
		{
			name: "valid nested path",
			url:  "/v1/groups/addMeta",
			want: true,
		},
		{
			name: "double slash rejected",
			url:  "//evil.com/path",
			want: false,
		},
		{
			name: "absolute URL rejected",
			url:  "https://evil.com/path",
			want: false,
		},
		{
			name: "javascript scheme rejected",
			url:  "javascript:alert(1)",
			want: false,
		},
		{
			name: "data scheme rejected",
			url:  "data:text/html,<script>alert(1)</script>",
			want: false,
		},
		{
			name: "empty string rejected",
			url:  "",
			want: false,
		},
		{
			name: "relative path rejected",
			url:  "relative/path",
			want: false,
		},
		{
			name: "backslash path rejected",
			url:  "\\evil.com",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSafeRedirect(tt.url)
			if got != tt.want {
				t.Errorf("isSafeRedirect(%q) = %v, want %v", tt.url, got, tt.want)
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
			got := RequestAcceptsHTML(req)
			if got != tt.want {
				t.Errorf("RequestAcceptsHTML(req with Accept=%q) = %v, want %v", tt.accept, got, tt.want)
			}
		})
	}
}
