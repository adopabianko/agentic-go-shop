package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"example.com/agentic-go-shop/api"
	"example.com/agentic-go-shop/internal/adapters/memory"
	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/httpapi"
	"example.com/agentic-go-shop/internal/inventory"
	"example.com/agentic-go-shop/internal/orders"
)

type harness struct {
	t      *testing.T
	server *httptest.Server
	spec   map[string]any
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	store, err := memory.New([]domain.Stock{{SKU: "SKU-001", Available: 10}, {SKU: "SKU-002", Available: 5}})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpapi.New(orders.New(store, inventory.Service{}), store, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(server.Close)
	var spec map[string]any
	if err := json.Unmarshal(api.Document, &spec); err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, server: server, spec: spec}
}

func (h *harness) request(method, path, operation, body, key, media string, want int) (map[string]any, http.Header) {
	h.t.Helper()
	req, err := http.NewRequest(method, h.server.URL+path, strings.NewReader(body))
	if err != nil {
		h.t.Fatal(err)
	}
	if media != "" {
		req.Header.Set("Content-Type", media)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := h.server.Client().Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	if resp.StatusCode != want {
		h.t.Fatalf("%s %s: got %d want %d: %s", method, path, resp.StatusCode, want, raw)
	}
	if resp.Header.Get("Content-Type") != "application/json" || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(resp.Header.Get("X-Request-ID")) {
		h.t.Fatalf("bad headers: %v", resp.Header)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		h.t.Fatal(err)
	}
	if operation != "" {
		pathSpec := h.spec["paths"].(map[string]any)[operation].(map[string]any)
		op := pathSpec[strings.ToLower(method)].(map[string]any)
		response, ok := op["responses"].(map[string]any)[strconv.Itoa(want)].(map[string]any)
		if !ok {
			h.t.Fatalf("status %d absent from %s", want, operation)
		}
		schema := response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
		validate(h.t, h.spec, schema, value, "response")
		if headers, ok := response["headers"].(map[string]any); ok {
			for name, entry := range headers {
				definition := entry.(map[string]any)
				validate(h.t, h.spec, definition["schema"].(map[string]any), resp.Header.Get(name), "header."+name)
			}
		}
	} else if want >= 400 {
		validate(h.t, h.spec, map[string]any{"$ref": "#/components/schemas/Error"}, value, "error")
	}
	return value, resp.Header
}

func TestHTTPJourneyAndContract(t *testing.T) {
	h := newHarness(t)
	h.request("GET", "/healthz", "/healthz", "", "", "", 200)
	body := `{"customer_id":"customer-001","items":[{"sku":"SKU-001","quantity":2}]}`
	created, headers := h.request("POST", "/v1/orders", "/v1/orders", body, "journey-key-001", "application/json", 201)
	if headers.Get("Idempotency-Replayed") != "false" {
		t.Fatal(headers)
	}
	id := created["id"].(string)
	replayed, headers := h.request("POST", "/v1/orders", "/v1/orders", body, "journey-key-001", "application/json; charset=utf-8", 201)
	if !reflect.DeepEqual(created, replayed) || headers.Get("Idempotency-Replayed") != "true" {
		t.Fatal("replay changed")
	}
	current, _ := h.request("GET", "/v1/orders/"+id, "/v1/orders/{id}", "", "", "", 200)
	if !reflect.DeepEqual(created, current) {
		t.Fatal("get changed order")
	}
	stock, _ := h.request("GET", "/v1/inventory/SKU-001", "/v1/inventory/{sku}", "", "", "", 200)
	if stock["available"] != float64(8) || stock["reserved"] != float64(2) {
		t.Fatal(stock)
	}
	h.request("POST", "/v1/orders", "/v1/orders", strings.Replace(body, `"quantity":2`, `"quantity":3`, 1), "journey-key-001", "application/json", 409)
	cancelled, _ := h.request("POST", "/v1/orders/"+id+"/cancel", "/v1/orders/{id}/cancel", "", "", "", 200)
	again, _ := h.request("POST", "/v1/orders/"+id+"/cancel", "/v1/orders/{id}/cancel", "", "", "", 200)
	if cancelled["status"] != "cancelled" || !reflect.DeepEqual(cancelled, again) {
		t.Fatal("cancel changed")
	}
	stock, _ = h.request("GET", "/v1/inventory/SKU-001", "/v1/inventory/{sku}", "", "", "", 200)
	if stock["available"] != float64(10) || stock["reserved"] != float64(0) {
		t.Fatal(stock)
	}
	replayed, _ = h.request("POST", "/v1/orders", "/v1/orders", body, "journey-key-001", "application/json", 201)
	if !reflect.DeepEqual(created, replayed) {
		t.Fatal("creation replay must retain original response")
	}
}

func TestHTTPRejectsInvalidRequests(t *testing.T) {
	h := newHarness(t)
	valid := `{"customer_id":"c","items":[{"sku":"SKU-001","quantity":1}]}`
	for _, tc := range []struct {
		name, body, key, media string
		status                 int
	}{
		{"missing key", valid, "", "application/json", 400},
		{"bad key", valid, "short", "application/json", 400},
		{"media", valid, "valid-key", "text/plain", 415},
		{"unknown", `{"customer_id":"c","items":[],"admin":true}`, "valid-key", "application/json", 400},
		{"trailing", valid + ` {}`, "valid-key", "application/json", 400},
		{"null", `null`, "valid-key", "application/json", 400},
		{"malformed", `{`, "valid-key", "application/json", 400},
		{"too large", string(bytes.Repeat([]byte(" "), httpapi.MaxBodyBytes)) + valid, "valid-key", "application/json", 413},
		{"negative", strings.Replace(valid, `"quantity":1`, `"quantity":-1`, 1), "valid-key", "application/json", 400},
		{"empty items", `{"customer_id":"c","items":[]}`, "valid-key", "application/json", 400},
		{"unknown SKU", strings.Replace(valid, "SKU-001", "missing", 1), "valid-key", "application/json", 404},
		{"insufficient", strings.Replace(valid, `"quantity":1`, `"quantity":11`, 1), "valid-key", "application/json", 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h.t = t
			h.request("POST", "/v1/orders", "/v1/orders", tc.body, tc.key, tc.media, tc.status)
		})
		h.t = t
	}
	h.request("GET", "/v1/orders/missing", "/v1/orders/{id}", "", "", "", 404)
	h.request("GET", "/v1/inventory/missing", "/v1/inventory/{sku}", "", "", "", 404)
	h.request("POST", "/v1/orders/missing/cancel", "/v1/orders/{id}/cancel", "", "", "", 404)
	h.request("POST", "/v1/orders/missing/cancel", "/v1/orders/{id}/cancel", "{}", "", "application/json", 400)
	h.request("GET", "/missing", "", "", "", "", 404)
	_, headers := h.request("DELETE", "/v1/orders/missing", "", "", "", "", 405)
	if headers.Get("Allow") != "GET" {
		t.Fatal(headers)
	}
}

// This deliberately small validator checks the schema vocabulary used in this
// contract. It is not a general OpenAPI/JSON Schema validator. Unsupported schema
// keywords fail loudly so future contracts cannot silently bypass the checks.
func validate(t *testing.T, root, schema map[string]any, value any, path string) {
	t.Helper()
	for key := range schema {
		switch key {
		case "$ref", "type", "properties", "required", "additionalProperties", "items", "minItems", "maxItems", "minimum", "maximum", "minLength", "maxLength", "pattern", "enum", "format", "description":
		default:
			t.Fatalf("unsupported schema keyword %s at %s", key, path)
		}
	}
	if reference, ok := schema["$ref"].(string); ok {
		var node any = root
		for _, part := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
			node = node.(map[string]any)[part]
		}
		validate(t, root, node.(map[string]any), value, path)
		return
	}
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, candidate := range enum {
			if reflect.DeepEqual(candidate, value) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s invalid enum: %v", path, value)
		}
	}
	switch schema["type"] {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("%s expected object", path)
		}
		properties, _ := schema["properties"].(map[string]any)
		if required, ok := schema["required"].([]any); ok {
			for _, name := range required {
				if _, ok := object[name.(string)]; !ok {
					t.Fatalf("%s missing %s", path, name)
				}
			}
		}
		for key, v := range object {
			if property, ok := properties[key].(map[string]any); ok {
				validate(t, root, property, v, path+"."+key)
			} else if schema["additionalProperties"] == false {
				t.Fatalf("%s unexpected field %s", path, key)
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			t.Fatalf("%s expected array", path)
		}
		bounds(t, schema, float64(len(array)), "minItems", "maxItems", path)
		for i, v := range array {
			validate(t, root, schema["items"].(map[string]any), v, fmt.Sprintf("%s[%d]", path, i))
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			t.Fatalf("%s expected string", path)
		}
		bounds(t, schema, float64(len([]rune(text))), "minLength", "maxLength", path)
		if pattern, ok := schema["pattern"].(string); ok && !regexp.MustCompile(pattern).MatchString(text) {
			t.Fatalf("%s invalid pattern: %s", path, text)
		}
		if format, ok := schema["format"].(string); ok {
			if format != "date-time" {
				t.Fatalf("unsupported format %s", format)
			}
			if _, err := time.Parse(time.RFC3339Nano, text); err != nil {
				t.Fatal(err)
			}
		}
	case "integer":
		n, ok := value.(float64)
		if !ok || n != float64(int64(n)) {
			t.Fatalf("%s expected integer", path)
		}
		bounds(t, schema, n, "minimum", "maximum", path)
	default:
		t.Fatalf("unsupported type %v", schema["type"])
	}
}

func bounds(t *testing.T, schema map[string]any, n float64, min, max, path string) {
	t.Helper()
	if v, ok := schema[min].(float64); ok && n < v {
		t.Fatalf("%s below minimum", path)
	}
	if v, ok := schema[max].(float64); ok && n > v {
		t.Fatalf("%s above maximum", path)
	}
}
