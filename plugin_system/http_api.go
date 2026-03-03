package plugin_system

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const (
	defaultHttpTimeout      = 10 * time.Second
	maxHttpTimeout          = 30 * time.Second
	maxHttpResponseBody     = 5 * 1024 * 1024 // 5MB
	maxHttpRedirects        = 10
	maxConcurrentHttpReqs   = 16
	httpUserAgent           = "mahresources-plugin/1.0"
)

// httpCallback holds a pending callback to be executed on the Lua VM thread.
type httpCallback struct {
	vm       *lua.LState
	fn       *lua.LFunction
	response map[string]any
}

// newHttpClient creates the shared HTTP client used for all plugin HTTP requests.
// Per-request timeouts are enforced via context.WithTimeout, so no client-level
// Timeout is set here to avoid redundant/conflicting deadline behavior.
func newHttpClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxHttpRedirects {
				return fmt.Errorf("stopped after %d redirects", maxHttpRedirects)
			}
			return nil
		},
	}
}

// registerHttpModule registers the mah.http sub-table in the Lua VM.
func (pm *PluginManager) registerHttpModule(L *lua.LState, mahMod *lua.LTable) {
	httpMod := L.NewTable()

	// mah.http.get(url, [options,] callback)
	httpMod.RawSetString("get", L.NewFunction(func(L *lua.LState) int {
		url := L.CheckString(1)
		headers, timeout, callback := parseOptionsAndCallback(L, 2)
		if callback == nil {
			L.ArgError(2, "callback function required")
			return 0
		}

		if err := validateScheme(url); err != nil {
			pm.queueErrorCallback(L, callback, "GET", url, err.Error())
			return 0
		}

		pm.httpWg.Add(1)
		go pm.executeHttpRequest("GET", url, "", headers, timeout, L, callback)
		return 0
	}))

	// mah.http.post(url, body, [options,] callback)
	httpMod.RawSetString("post", L.NewFunction(func(L *lua.LState) int {
		url := L.CheckString(1)
		body := L.CheckString(2)
		headers, timeout, callback := parseOptionsAndCallback(L, 3)
		if callback == nil {
			L.ArgError(3, "callback function required")
			return 0
		}

		if err := validateScheme(url); err != nil {
			pm.queueErrorCallback(L, callback, "POST", url, err.Error())
			return 0
		}

		pm.httpWg.Add(1)
		go pm.executeHttpRequest("POST", url, body, headers, timeout, L, callback)
		return 0
	}))

	// mah.http.request(method, url, options, callback)
	httpMod.RawSetString("request", L.NewFunction(func(L *lua.LState) int {
		method := strings.ToUpper(L.CheckString(1))
		url := L.CheckString(2)
		optsTbl := L.CheckTable(3)
		callback := L.CheckFunction(4)

		headers, timeout, body := extractRequestOptions(L, optsTbl)

		if err := validateScheme(url); err != nil {
			pm.queueErrorCallback(L, callback, method, url, err.Error())
			return 0
		}

		pm.httpWg.Add(1)
		go pm.executeHttpRequest(method, url, body, headers, timeout, L, callback)
		return 0
	}))

	mahMod.RawSetString("http", httpMod)
}

// parseOptionsAndCallback extracts optional options table and required callback
// from Lua arguments starting at the given index.
// Patterns: (callback) or (options, callback)
func parseOptionsAndCallback(L *lua.LState, startIdx int) (map[string]string, time.Duration, *lua.LFunction) {
	arg := L.Get(startIdx)

	// If the first arg is a function, no options table
	if fn, ok := arg.(*lua.LFunction); ok {
		return nil, defaultHttpTimeout, fn
	}

	// Otherwise expect options table + callback
	if tbl, ok := arg.(*lua.LTable); ok {
		callback := L.CheckFunction(startIdx + 1)
		headers, timeout, _ := extractRequestOptions(L, tbl)
		return headers, timeout, callback
	}

	return nil, defaultHttpTimeout, nil
}

// extractRequestOptions reads headers, timeout, and body from an options table.
func extractRequestOptions(_ *lua.LState, tbl *lua.LTable) (map[string]string, time.Duration, string) {
	headers := make(map[string]string)
	timeout := defaultHttpTimeout
	var body string

	if headersTbl, ok := tbl.RawGetString("headers").(*lua.LTable); ok {
		headersTbl.ForEach(func(key, value lua.LValue) {
			if k, ok := key.(lua.LString); ok {
				headers[string(k)] = value.String()
			}
		})
	}

	if timeoutVal, ok := tbl.RawGetString("timeout").(lua.LNumber); ok {
		t := time.Duration(float64(timeoutVal)) * time.Second
		if t > maxHttpTimeout {
			t = maxHttpTimeout
		}
		if t > 0 {
			timeout = t
		}
	}

	if bodyVal, ok := tbl.RawGetString("body").(lua.LString); ok {
		body = string(bodyVal)
	}

	return headers, timeout, body
}

// validateScheme ensures only http:// and https:// URLs are allowed.
func validateScheme(url string) error {
	lower := strings.ToLower(url)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return nil
	}
	return fmt.Errorf("unsupported URL scheme (only http and https are allowed)")
}

// executeHttpRequest performs the HTTP request in a goroutine and queues the callback.
func (pm *PluginManager) executeHttpRequest(method, url, body string, headers map[string]string, timeout time.Duration, vm *lua.LState, callback *lua.LFunction) {
	defer pm.httpWg.Done()

	// Acquire concurrency semaphore
	pm.httpSem <- struct{}{}
	defer func() { <-pm.httpSem }()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		pm.queueHttpCallback(httpCallback{
			vm: vm,
			fn: callback,
			response: map[string]any{
				"error":  err.Error(),
				"url":    url,
				"method": method,
			},
		})
		return
	}

	// Set default User-Agent
	req.Header.Set("User-Agent", httpUserAgent)

	// Apply custom headers (may override User-Agent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := pm.httpClient.Do(req)
	if err != nil {
		pm.queueHttpCallback(httpCallback{
			vm: vm,
			fn: callback,
			response: map[string]any{
				"error":  err.Error(),
				"url":    url,
				"method": method,
			},
		})
		return
	}
	defer resp.Body.Close()

	// Read body with size limit
	limitedReader := io.LimitReader(resp.Body, maxHttpResponseBody+1)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		pm.queueHttpCallback(httpCallback{
			vm: vm,
			fn: callback,
			response: map[string]any{
				"error":  fmt.Sprintf("reading response body: %v", err),
				"url":    url,
				"method": method,
			},
		})
		return
	}

	bodyStr := string(bodyBytes)
	if len(bodyBytes) > maxHttpResponseBody {
		bodyStr = bodyStr[:maxHttpResponseBody]
		log.Printf("[plugin] warning: HTTP response body truncated at %d bytes for %s %s", maxHttpResponseBody, method, url)
	}

	// Build response headers (lowercase keys, comma-joined per RFC 7230)
	respHeaders := make(map[string]any)
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			respHeaders[strings.ToLower(k)] = strings.Join(vals, ", ")
		}
	}

	pm.queueHttpCallback(httpCallback{
		vm: vm,
		fn: callback,
		response: map[string]any{
			"status_code": float64(resp.StatusCode),
			"status":      resp.Status,
			"body":        bodyStr,
			"headers":     respHeaders,
			"url":         url,
			"method":      method,
		},
	})
}

// queueErrorCallback is a convenience for queuing an error response.
// queueHttpCallback is non-blocking, so no goroutine is needed.
func (pm *PluginManager) queueErrorCallback(vm *lua.LState, callback *lua.LFunction, method, url, errMsg string) {
	pm.queueHttpCallback(httpCallback{
		vm: vm,
		fn: callback,
		response: map[string]any{
			"error":  errMsg,
			"url":    url,
			"method": method,
		},
	})
}

// queueHttpCallback appends a callback to the pending list and signals the drain goroutine.
func (pm *PluginManager) queueHttpCallback(cb httpCallback) {
	pm.httpMu.Lock()
	pm.httpPending = append(pm.httpPending, cb)
	pm.httpMu.Unlock()

	// Non-blocking signal
	select {
	case pm.httpNotify <- struct{}{}:
	default:
	}
}

// drainHttpCallbacks runs as a background goroutine, executing pending HTTP callbacks
// on their respective Lua VMs.
func (pm *PluginManager) drainHttpCallbacks() {
	for {
		select {
		case <-pm.done:
			return
		case <-pm.httpNotify:
			pm.processPendingCallbacks()
		}
	}
}

// processPendingCallbacks drains all pending callbacks and executes them.
func (pm *PluginManager) processPendingCallbacks() {
	pm.httpMu.Lock()
	pending := pm.httpPending
	pm.httpPending = nil
	pm.httpMu.Unlock()

	for _, cb := range pending {
		if pm.closed.Load() {
			return
		}

		mu := pm.VMLock(cb.vm)
		if mu == nil {
			continue
		}
		mu.Lock()

		tbl := goToLuaTable(cb.vm, cb.response)

		timeoutCtx, cancel := context.WithTimeout(context.Background(), luaExecTimeout)
		cb.vm.SetContext(timeoutCtx)

		err := cb.vm.CallByParam(lua.P{
			Fn:      cb.fn,
			NRet:    0,
			Protect: true,
		}, tbl)

		cb.vm.RemoveContext()
		cancel()

		if err != nil {
			log.Printf("[plugin] warning: HTTP callback error: %v", err)
		}

		mu.Unlock()
	}
}
