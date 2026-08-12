package memereact

import (
	"testing"
)

func TestNewReturnsNilWithoutKeys(t *testing.T) {
	if r := New(Config{}); r != nil {
		t.Fatal("expected nil without API keys")
	}
	if r := New(Config{AIAPIKey: "key"}); r != nil {
		t.Fatal("expected nil without Giphy key")
	}
	if r := New(Config{AIAPIKey: "key", GiphyAPIKey: "key"}); r == nil {
		t.Fatal("expected non-nil with both keys")
	}
}

func TestGiphySearchReal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Giphy API test in short mode")
	}
}

func TestDefaultValues(t *testing.T) {
	r := New(Config{AIAPIKey: "a", GiphyAPIKey: "b"})
	if r.cfg.AIModel != "big-pickle" {
		t.Fatalf("expected default model big-pickle, got %q", r.cfg.AIModel)
	}
	if r.cfg.AIBaseURL != "https://opencode.ai/zen/v1" {
		t.Fatalf("expected default base URL, got %q", r.cfg.AIBaseURL)
	}
}
