package template_context_providers

import (
	"fmt"
	"io"
	"mahresources/application_context/mock_context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGroupContextProviderImpl(t *testing.T) {
	reader := mock_context.NewMockGroupContext()
	handler := func(w http.ResponseWriter, r *http.Request) {
		groupContextProviderImpl(reader)(r)
	}

	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	fmt.Println(resp.StatusCode)
	fmt.Println(resp.Header.Get("Content-Type"))
	fmt.Println(string(body))
}
