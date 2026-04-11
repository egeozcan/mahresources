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
	"time"
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	if len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, result)
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

// JobStatus is the JSON shape returned by /v1/jobs/get.
type JobStatus struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Phase      string `json:"phase,omitempty"`
	Progress   int64  `json:"progress"`
	TotalSize  int64  `json:"totalSize"`
	Error      string `json:"error,omitempty"`
	ResultPath string `json:"resultPath,omitempty"`
}

// PollJob polls /v1/jobs/get?id=<jobID> every interval until the job
// reaches a terminal state (completed/failed/cancelled) or until
// totalTimeout elapses.
func (c *Client) PollJob(jobID string, interval, totalTimeout time.Duration) (*JobStatus, error) {
	deadline := time.Now().Add(totalTimeout)
	for {
		var status JobStatus
		if err := c.Get("/v1/jobs/get", url.Values{"id": []string{jobID}}, &status); err != nil {
			return nil, err
		}
		if status.Status == "completed" || status.Status == "failed" || status.Status == "cancelled" {
			return &status, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("client: job %s did not complete within %s (last status: %s)", jobID, totalTimeout, status.Status)
		}
		time.Sleep(interval)
	}
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
