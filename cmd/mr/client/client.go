package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Client wraps HTTP communication with the mahresources API.
type Client struct {
	BaseURL    string
	httpClient *http.Client
}

// New creates a new API client for the given base URL.
func New(baseURL string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{},
	}
}

// apiError represents an error response from the API.
type apiError struct {
	Error string `json:"error"`
}

func (c *Client) buildURL(path string, query url.Values) string {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func decodeError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("HTTP %d (failed to read body: %v)", resp.StatusCode, err)
	}

	var apiErr apiError
	if json.Unmarshal(body, &apiErr) == nil && apiErr.Error != "" {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, apiErr.Error)
	}

	// Truncate body for display
	s := string(body)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, s)
}

func decodeResponse(resp *http.Response, result any) error {
	if resp.StatusCode >= 400 {
		return decodeError(resp)
	}
	if result == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

// Get performs a GET request and decodes the JSON response into result.
func (c *Client) Get(path string, query url.Values, result any) error {
	req, err := http.NewRequest(http.MethodGet, c.buildURL(path, query), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return decodeResponse(resp, result)
}

// Post performs a POST request with a JSON body and decodes the response.
func (c *Client) Post(path string, query url.Values, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(http.MethodPost, c.buildURL(path, query), bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return decodeResponse(resp, result)
}

// PostForm performs a POST request with form-encoded body and decodes the response.
func (c *Client) PostForm(path string, query url.Values, formData url.Values, result any) error {
	var bodyReader io.Reader
	if formData != nil {
		bodyReader = strings.NewReader(formData.Encode())
	}

	req, err := http.NewRequest(http.MethodPost, c.buildURL(path, query), bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if formData != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return decodeResponse(resp, result)
}

// Delete performs a DELETE request and decodes the JSON response.
func (c *Client) Delete(path string, query url.Values, result any) error {
	req, err := http.NewRequest(http.MethodDelete, c.buildURL(path, query), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return decodeResponse(resp, result)
}

// Put performs a PUT request with a JSON body and decodes the response.
func (c *Client) Put(path string, query url.Values, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(http.MethodPut, c.buildURL(path, query), bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return decodeResponse(resp, result)
}

// Patch performs a PATCH request with a JSON body and decodes the response.
func (c *Client) Patch(path string, query url.Values, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(http.MethodPatch, c.buildURL(path, query), bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return decodeResponse(resp, result)
}

// UploadFile performs a multipart file upload.
func (c *Client) UploadFile(path string, query url.Values, fieldName string, filePath string, extraFields map[string]string, result any) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}

	for k, v := range extraFields {
		if err := writer.WriteField(k, v); err != nil {
			return err
		}
	}

	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, c.buildURL(path, query), &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return decodeResponse(resp, result)
}

// DownloadFile streams the response body to a file on disk.
func (c *Client) DownloadFile(path string, query url.Values, destPath string) (int64, error) {
	req, err := http.NewRequest(http.MethodGet, c.buildURL(path, query), nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return 0, decodeError(resp)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	return io.Copy(out, resp.Body)
}

// GetRaw performs a GET request and returns the raw response.
// The caller is responsible for closing the response body.
func (c *Client) GetRaw(path string, query url.Values) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, c.buildURL(path, query), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}
