package api_tests

import (
	"net/http"
	"testing"
)

func TestDeleteHandlers_ZeroIDGuard(t *testing.T) {
	tc := SetupTestEnv(t)

	tests := []struct {
		name string
		url  string
	}{
		{"group delete", "/v1/group/delete"},
		{"note type delete", "/v1/note/noteType/delete"},
		{"relation delete", "/v1/relation/delete"},
		{"relation type delete", "/v1/relationType/delete"},
		{"resource removeSeries", "/v1/resource/removeSeries"},
		{"tag delete", "/v1/tag/delete"},
		{"series delete", "/v1/series/delete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// POST with no ID body should return 400
			rr := tc.MakeRequest(http.MethodPost, tt.url, nil)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("%s: expected status 400, got %d (body: %s)", tt.name, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestDeleteHandlers_ZeroIDGuard_ExplicitZero(t *testing.T) {
	tc := SetupTestEnv(t)

	tests := []struct {
		name string
		url  string
	}{
		{"group delete with zero ID", "/v1/group/delete"},
		{"note type delete with zero ID", "/v1/note/noteType/delete"},
		{"relation delete with zero ID", "/v1/relation/delete"},
		{"relation type delete with zero ID", "/v1/relationType/delete"},
		{"resource removeSeries with zero ID", "/v1/resource/removeSeries"},
		{"tag delete with zero ID", "/v1/tag/delete"},
		{"series delete with zero ID", "/v1/series/delete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// POST with explicit zero ID should return 400
			rr := tc.MakeRequest(http.MethodPost, tt.url, map[string]interface{}{"ID": 0})

			if rr.Code != http.StatusBadRequest {
				t.Errorf("%s: expected status 400, got %d (body: %s)", tt.name, rr.Code, rr.Body.String())
			}
		})
	}
}
