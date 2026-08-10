package channelnamer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func TestNewDisabled(t *testing.T) {
	if n := New(Config{}); n != nil {
		t.Fatalf("expected nil for empty config")
	}
	if n := New(Config{APIKey: "key"}); n != nil {
		t.Fatalf("expected nil when no channels")
	}
	if n := New(Config{ChannelIDs: []snowflake.ID{1}}); n != nil {
		t.Fatalf("expected nil when no API key")
	}
}

func TestNewEnabled(t *testing.T) {
	cfg := Config{
		ChannelIDs: []snowflake.ID{snowflake.ID(1), snowflake.ID(2)},
		APIKey:     "test-key",
		Model:      "big-pickle",
		BaseURL:    "https://opencode.ai/zen/v1",
	}
	n := New(cfg)
	if n == nil {
		t.Fatalf("expected non-nil Namer for valid config")
	}
	if n.config.Model != "big-pickle" {
		t.Fatalf("expected big-pickle model")
	}
}

func TestParseNames(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     int
		wantOK   bool
	}{
		{"simple", "Haus der flüsternden Wände 👻🏚️\nZum tanzenden Einhorn 🦄💃\nClub der müden Kekse 🍪😴", 3, true},
		{"empty lines ignored", "Name 1 😀\n\nName 2 🎉\n  \nName 3 🔥", 3, true},
		{"trim whitespace", "  Name 1 😀  \n  Name 2 🎉  ", 2, true},
		{"too few", "Nur ein Name 😢", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseNames(tt.response)
			if ok != tt.wantOK {
				t.Fatalf("expected ok=%v, got ok=%v", tt.wantOK, ok)
			}
			if ok && len(got) != tt.want {
				t.Fatalf("expected %d names, got %d: %v", tt.want, len(got), got)
			}
		})
	}
}

func TestBuildPrompt(t *testing.T) {
	channelIDs := []snowflake.ID{snowflake.ID(1), snowflake.ID(2)}
	recent := []string{"Alter Name 🔥"}

	prompt := buildPrompt(channelIDs, recent)

	if !strings.Contains(prompt, "2") {
		t.Fatalf("expected prompt to mention count 2")
	}
	if !strings.Contains(prompt, "Alter Name 🔥") {
		t.Fatalf("expected prompt to mention recent name")
	}
	if !strings.Contains(prompt, "Haus des heißen Dampfes") {
		t.Fatalf("expected prompt to contain examples")
	}
}

func TestGenerateNames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			w.WriteHeader(404)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(401)
			return
		}
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "big-pickle" {
			t.Fatalf("expected model big-pickle, got %q", req.Model)
		}
		if req.Temperature != 0.9 {
			t.Fatalf("expected temperature 0.9, got %f", req.Temperature)
		}
		resp := chatResponse{
			Choices: []chatChoice{
				{Message: chatMessage{Content: "Zum fröhlichen Faultier 🦥🎉\nHaus der singenden Kürbisse 🎃🎵"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	n := &Namer{
		config: Config{
			ChannelIDs: []snowflake.ID{snowflake.ID(1), snowflake.ID(2)},
			APIKey:     "test-key",
			Model:      "big-pickle",
			BaseURL:    server.URL,
		},
		httpClient: server.Client(),
	}

	names, err := n.generateNames([]string{"Old 🔥"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
}

func TestGenerateNamesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	n := &Namer{
		config:     Config{ChannelIDs: []snowflake.ID{1}, APIKey: "key", Model: "big-pickle", BaseURL: server.URL},
		httpClient: server.Client(),
	}

	_, err := n.generateNames(nil)
	if err == nil {
		t.Fatalf("expected error for 500 response")
	}
}
