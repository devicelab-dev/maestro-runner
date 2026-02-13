package jsengine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dop251/goja"
)

// httpModule returns the http object with get, post, put, delete methods
func (e *Engine) httpModule() *goja.Object {
	obj := e.runtime.NewObject()

	// http.get(url, [options])
	if err := obj.Set("get", func(call goja.FunctionCall) goja.Value {
		return e.doHTTPRequest("GET", call)
	}); err != nil {
		panic(e.runtime.NewTypeError(fmt.Sprintf("failed to set http.get: %v", err)))
	}

	// http.post(url, [options])
	if err := obj.Set("post", func(call goja.FunctionCall) goja.Value {
		return e.doHTTPRequest("POST", call)
	}); err != nil {
		panic(e.runtime.NewTypeError(fmt.Sprintf("failed to set http.post: %v", err)))
	}

	// http.put(url, [options])
	if err := obj.Set("put", func(call goja.FunctionCall) goja.Value {
		return e.doHTTPRequest("PUT", call)
	}); err != nil {
		panic(e.runtime.NewTypeError(fmt.Sprintf("failed to set http.put: %v", err)))
	}

	// http.delete(url, [options])
	if err := obj.Set("delete", func(call goja.FunctionCall) goja.Value {
		return e.doHTTPRequest("DELETE", call)
	}); err != nil {
		panic(e.runtime.NewTypeError(fmt.Sprintf("failed to set http.delete: %v", err)))
	}

	// http.request(method, url, [options])
	if err := obj.Set("request", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			panic(e.runtime.NewTypeError("http.request requires method and url"))
		}
		method := call.Arguments[0].String()
		// Shift arguments
		newArgs := call.Arguments[1:]
		newCall := goja.FunctionCall{
			This:      call.This,
			Arguments: newArgs,
		}
		return e.doHTTPRequest(method, newCall)
	}); err != nil {
		panic(e.runtime.NewTypeError(fmt.Sprintf("failed to set http.request: %v", err)))
	}

	return obj
}

// HTTPResponse represents the response from an HTTP request
type HTTPResponse struct {
	Status  int                    `json:"status"`
	Body    string                 `json:"body"`
	Headers map[string]string      `json:"headers"`
	Ok      bool                   `json:"ok"`
	JSON    map[string]interface{} `json:"json,omitempty"`
}

// doHTTPRequest performs an HTTP request and returns the response
func (e *Engine) doHTTPRequest(method string, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		panic(e.runtime.NewTypeError(fmt.Sprintf("http.%s requires url", method)))
	}

	url := call.Arguments[0].String()

	// Parse options if provided
	var body io.Reader
	headers := make(map[string]string)
	timeout := 30 * time.Second

	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Arguments[1]) {
		opts := call.Arguments[1].Export()
		if optsMap, ok := opts.(map[string]interface{}); ok {
			// Body
			if b, ok := optsMap["body"]; ok {
				switch v := b.(type) {
				case string:
					body = bytes.NewBufferString(v)
				case map[string]interface{}:
					jsonBytes, _ := json.Marshal(v)
					body = bytes.NewBuffer(jsonBytes)
					if _, hasContentType := headers["Content-Type"]; !hasContentType {
						headers["Content-Type"] = "application/json"
					}
				}
			}

			// Headers
			if h, ok := optsMap["headers"]; ok {
				if headersMap, ok := h.(map[string]interface{}); ok {
					for k, v := range headersMap {
						headers[k] = fmt.Sprintf("%v", v)
					}
				}
			}

			// Timeout
			if t, ok := optsMap["timeout"]; ok {
				switch v := t.(type) {
				case int64:
					timeout = time.Duration(v) * time.Millisecond
				case float64:
					timeout = time.Duration(v) * time.Millisecond
				}
			}
		}
	}

	// Create request
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		panic(e.runtime.NewTypeError(fmt.Sprintf("failed to create request: %v", err)))
	}

	// Set headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Create client with timeout
	client := &http.Client{
		Timeout: timeout,
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		panic(e.runtime.NewTypeError(fmt.Sprintf("HTTP request failed: %v", err)))
	}
	defer func() { _ = resp.Body.Close() }()

	// Read body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(e.runtime.NewTypeError(fmt.Sprintf("failed to read response: %v", err)))
	}

	// Build response object
	response := HTTPResponse{
		Status:  resp.StatusCode,
		Body:    string(bodyBytes),
		Headers: make(map[string]string),
		Ok:      resp.StatusCode >= 200 && resp.StatusCode < 300,
	}

	// Copy headers
	for k, v := range resp.Header {
		if len(v) > 0 {
			response.Headers[k] = v[0]
		}
	}

	// Try to parse JSON body
	var jsonBody map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &jsonBody); err == nil {
		response.JSON = jsonBody
	}

	// Convert to JS object
	responseObj := e.runtime.NewObject()
	for _, kv := range []struct {
		key string
		val interface{}
	}{
		{"status", response.Status},
		{"body", response.Body},
		{"headers", response.Headers},
		{"ok", response.Ok},
	} {
		if err := responseObj.Set(kv.key, kv.val); err != nil {
			panic(e.runtime.NewTypeError(fmt.Sprintf("failed to set response.%s: %v", kv.key, err)))
		}
	}

	// Add json as parsed data (or null if not JSON)
	if response.JSON != nil {
		if err := responseObj.Set("json", response.JSON); err != nil {
			panic(e.runtime.NewTypeError(fmt.Sprintf("failed to set response.json: %v", err)))
		}
	} else {
		if err := responseObj.Set("json", goja.Null()); err != nil {
			panic(e.runtime.NewTypeError(fmt.Sprintf("failed to set response.json: %v", err)))
		}
	}

	return responseObj
}
