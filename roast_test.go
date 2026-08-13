package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type aiStub struct {
	server          *httptest.Server
	calls           atomic.Int64
	primaryStatus   int
	fallbackStatus  int
	dropConn        bool
	responseContent string
}

func newAIStub(t *testing.T, primaryStatus int, dropConn bool) *aiStub {
	t.Helper()
	s := &aiStub{
		primaryStatus:   primaryStatus,
		fallbackStatus:  200,
		dropConn:        dropConn,
		responseContent: "roast from fallback model",
	}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls.Add(1)
		var body chatRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch body.Model {
		case aiModel:
			if s.dropConn {
				if h, ok := w.(http.Hijacker); ok {
					if conn, _, err := h.Hijack(); err == nil {
						conn.Close()
						return
					}
				}
			}
			if s.primaryStatus != 200 {
				w.WriteHeader(s.primaryStatus)
				return
			}
			writeAIResponse(w, s.responseContent)
		case aiFallbackModel:
			if s.fallbackStatus != 200 {
				w.WriteHeader(s.fallbackStatus)
				return
			}
			writeAIResponse(w, s.responseContent)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

func writeAIResponse(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, content)
}

func setAIStubGlobals(t *testing.T, srvURL, fallbackModel string) {
	t.Helper()
	oldModel, oldFallback, oldBase, oldKey := aiModel, aiFallbackModel, aiBaseURL, aiAPIKey
	aiModel = "primary-model"
	aiFallbackModel = fallbackModel
	aiBaseURL = srvURL
	aiAPIKey = "test-key"
	t.Cleanup(func() {
		aiModel, aiFallbackModel, aiBaseURL, aiAPIKey = oldModel, oldFallback, oldBase, oldKey
	})
}

func TestCallAISuccess(t *testing.T) {
	srv := newAIStub(t, 200, false)
	setAIStubGlobals(t, srv.server.URL, "fallback-model")

	got, err := callAI("sys", "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != srv.responseContent {
		t.Fatalf("got %q, want %q", got, srv.responseContent)
	}
	if srv.calls.Load() != 1 {
		t.Fatalf("expected 1 API call, got %d", srv.calls.Load())
	}
}

func TestCallAIFallbackOnRateLimit(t *testing.T) {
	srv := newAIStub(t, 429, false)
	setAIStubGlobals(t, srv.server.URL, "fallback-model")

	got, err := callAI("sys", "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != srv.responseContent {
		t.Fatalf("got %q, want %q", got, srv.responseContent)
	}
	if srv.calls.Load() != 2 {
		t.Fatalf("expected 2 API calls (primary + fallback), got %d", srv.calls.Load())
	}
}

func TestCallAIFallbackOnServerError(t *testing.T) {
	srv := newAIStub(t, 500, false)
	setAIStubGlobals(t, srv.server.URL, "fallback-model")

	got, err := callAI("sys", "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != srv.responseContent {
		t.Fatalf("got %q, want %q", got, srv.responseContent)
	}
	if srv.calls.Load() != 2 {
		t.Fatalf("expected 2 API calls (primary + fallback), got %d", srv.calls.Load())
	}
}

func TestCallAIFallbackOnTransportError(t *testing.T) {
	srv := newAIStub(t, 429, true)
	setAIStubGlobals(t, srv.server.URL, "fallback-model")

	got, err := callAI("sys", "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != srv.responseContent {
		t.Fatalf("got %q, want %q", got, srv.responseContent)
	}
	if srv.calls.Load() != 2 {
		t.Fatalf("expected 2 API calls (primary + fallback), got %d", srv.calls.Load())
	}
}

func TestCallAINoFallbackConfigured(t *testing.T) {
	srv := newAIStub(t, 429, false)
	setAIStubGlobals(t, srv.server.URL, "")

	_, err := callAI("sys", "prompt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("error %q does not mention 429", err)
	}
	if srv.calls.Load() != 1 {
		t.Fatalf("expected 1 API call, got %d", srv.calls.Load())
	}
}

func TestCallAIFallbackFailsToo(t *testing.T) {
	srv := newAIStub(t, 429, false)
	srv.fallbackStatus = 500
	setAIStubGlobals(t, srv.server.URL, "fallback-model")

	_, err := callAI("sys", "prompt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if srv.calls.Load() != 2 {
		t.Fatalf("expected exactly 2 API calls (no endless loop), got %d", srv.calls.Load())
	}
}

func TestCallAINoFallbackOnClientError(t *testing.T) {
	srv := newAIStub(t, 400, false)
	setAIStubGlobals(t, srv.server.URL, "fallback-model")

	_, err := callAI("sys", "prompt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if srv.calls.Load() != 1 {
		t.Fatalf("expected 1 API call (client error must not fall back), got %d", srv.calls.Load())
	}
}
