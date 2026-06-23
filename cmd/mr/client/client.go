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

// New creates a new API client for the given base URL. If an API token is
// available (MR_TOKEN env, or the stored credentials file written by
// `mr auth login`), every request is authenticated with it via a Bearer header.
// When the server has auth disabled the token is simply ignored.
func New(baseURL string) *Client {
	hc := &http.Client{}
	if token := ResolveToken(baseURL); token != "" {
		hc.Transport = &authTransport{token: token, base: http.DefaultTransport}
	}
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: hc,
	}
}

// authTransport injects an Authorization: Bearer header on every request.
type authTransport struct {
	token string
	base  http.RoundTripper
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.token != "" && req.Header.Get("Authorization") == "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	return t.base.RoundTrip(req)
}

// ResolveToken returns the API token to use for baseURL. The MR_TOKEN env var is
// an explicit global override and takes precedence; otherwise the stored token
// for baseURL's origin is returned (or "" if none). Stored tokens are bound to a
// server origin so a token minted against one server is never sent to a
// different host.
func ResolveToken(baseURL string) string {
	if t := strings.TrimSpace(os.Getenv("MR_TOKEN")); t != "" {
		return t
	}
	return readTokenMap()[normalizeOrigin(baseURL)]
}

// normalizeOrigin reduces a server URL to its scheme://host[:port] origin
// (lowercased) — the key under which that server's token is stored.
func normalizeOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return strings.ToLower(u.Scheme + "://" + u.Host)
	}
	return strings.ToLower(strings.TrimRight(raw, "/"))
}

// readTokenMap loads the origin→token map from the credentials file. A legacy
// single-token file (pre-origin-binding) is not valid JSON and yields an empty
// map, which forces a re-login that rewrites the file in the new format — so a
// stale global token is never reused across origins.
func readTokenMap() map[string]string {
	m := map[string]string{}
	path := tokenFilePath()
	if path == "" {
		return m
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	_ = json.Unmarshal(b, &m)
	return m
}

// writeTokenMap persists the origin→token map (0600).
func writeTokenMap(m map[string]string) error {
	path := tokenFilePath()
	if path == "" {
		return fmt.Errorf("could not determine token file path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}

// tokenFilePath returns the path to the stored token file. Honors MR_TOKEN_FILE,
// then XDG_CONFIG_HOME, then ~/.config/mahresources/token.
func tokenFilePath() string {
	if p := os.Getenv("MR_TOKEN_FILE"); p != "" {
		return p
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "mahresources", "token")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "mahresources", "token")
}

// StoreToken persists token for baseURL's origin (0600), leaving any tokens for
// other servers intact.
func StoreToken(baseURL, token string) error {
	m := readTokenMap()
	m[normalizeOrigin(baseURL)] = strings.TrimSpace(token)
	return writeTokenMap(m)
}

// ClearToken removes the stored token for baseURL's origin. The credentials file
// is deleted only when no tokens remain.
func ClearToken(baseURL string) error {
	m := readTokenMap()
	delete(m, normalizeOrigin(baseURL))
	if len(m) == 0 {
		path := tokenFilePath()
		if path == "" {
			return nil
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return writeTokenMap(m)
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

// DeleteJSON performs a DELETE request with a JSON body and decodes the response.
func (c *Client) DeleteJSON(path string, query url.Values, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(http.MethodDelete, c.buildURL(path, query), bodyReader)
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

// UploadFileStreaming performs a multipart file upload without buffering the
// entire file in RAM. It uses an io.Pipe so the multipart encoder writes
// directly into the HTTP request body, keeping memory usage bounded even for
// multi-GB files.
func (c *Client) UploadFileStreaming(path string, query url.Values, fieldName string, filePath string, extraFields map[string]string, result any) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Write the multipart body in a goroutine so it streams into the pipe.
	errCh := make(chan error, 1)
	go func() {
		defer pw.Close()

		part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
		if err != nil {
			errCh <- err
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			errCh <- err
			return
		}
		for k, v := range extraFields {
			if err := writer.WriteField(k, v); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- writer.Close()
	}()

	req, err := http.NewRequest(http.MethodPost, c.buildURL(path, query), pr)
	if err != nil {
		pr.Close()
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check the goroutine's error
	if writeErr := <-errCh; writeErr != nil {
		return fmt.Errorf("multipart write: %w", writeErr)
	}

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
