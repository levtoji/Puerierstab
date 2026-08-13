package profile

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/levtoji/Puerierstab/internal/chatlog"
	"github.com/levtoji/Puerierstab/internal/reactions"
)

func TestNewDisabled(t *testing.T) {
	if p := New(Config{}, nil, nil); p != nil {
		t.Fatalf("expected nil for empty config")
	}
}

func TestNewEnabled(t *testing.T) {
	p := New(Config{APIKey: "key", Dir: t.TempDir()}, nil, nil)
	if p == nil {
		t.Fatalf("expected non-nil Profiler for valid config")
	}
	if p.cfg.Model != "big-pickle" {
		t.Fatalf("expected default model big-pickle, got %q", p.cfg.Model)
	}
	if p.cfg.BaseURL != "https://opencode.ai/zen/v1" {
		t.Fatalf("expected default base url, got %q", p.cfg.BaseURL)
	}
}

func TestNextSchedule(t *testing.T) {
	if d := nextSchedule(); d <= 0 || d > 24*time.Hour {
		t.Fatalf("expected next schedule within (0, 24h], got %v", d)
	}
}

func TestBuildProfilePrompt(t *testing.T) {
	msg := buildProfilePrompt("Kevin", "Hallo Welt", map[string]int{"🍕": 3}, map[string]int{"🔥": 5})
	for _, want := range []string{"Kevin", "Hallo Welt", "🍕 (3)", "🔥 (5)"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", want, msg)
		}
	}

	noData := buildProfilePrompt("Kevin", "", nil, nil)
	if !strings.Contains(noData, "Kevin") {
		t.Fatalf("expected prompt to work without data")
	}
}

func TestGenerateProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(401)
			return
		}
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "big-pickle" {
			t.Fatalf("expected model big-pickle, got %q", req.Model)
		}
		if req.Temperature != 0.7 {
			t.Fatalf("expected temperature 0.7, got %f", req.Temperature)
		}
		json.NewEncoder(w).Encode(chatResponse{Choices: []chatChoice{{Message: chatMessage{Content: "Kevin ist ein Pizza-Enthusiast."}}}})
	}))
	defer server.Close()

	p := &Profiler{
		cfg: Config{APIKey: "test-key", Model: "big-pickle", BaseURL: server.URL, Dir: t.TempDir()},
	}
	p.httpClient = server.Client()

	got, err := p.generateProfile(snowflake.ID(1), "Kevin", "Nachrichten", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Kevin ist ein Pizza-Enthusiast." {
		t.Fatalf("got %q", got)
	}
}

func TestGenerateProfileFallback(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if calls == 1 {
			w.WriteHeader(429)
			return
		}
		if req.Model != "fallback-model" {
			t.Fatalf("expected fallback model, got %q", req.Model)
		}
		json.NewEncoder(w).Encode(chatResponse{Choices: []chatChoice{{Message: chatMessage{Content: "Profil vom Fallback."}}}})
	}))
	defer server.Close()

	p := &Profiler{
		cfg: Config{APIKey: "test-key", Model: "big-pickle", FallbackModel: "fallback-model", BaseURL: server.URL, Dir: t.TempDir()},
	}
	p.httpClient = server.Client()

	got, err := p.generateProfile(snowflake.ID(1), "Kevin", "Nachrichten", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Profil vom Fallback." {
		t.Fatalf("got %q", got)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestStoreAll(t *testing.T) {
	s := &Store{Profiles: map[snowflake.ID]Profile{
		snowflake.ID(1): {Text: "A", UpdatedAt: time.Now()},
		snowflake.ID(2): {Text: "B", UpdatedAt: time.Now()},
	}}
	all := s.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(all))
	}
	if all[snowflake.ID(1)].Text != "A" {
		t.Fatalf("got %q", all[snowflake.ID(1)].Text)
	}
}

func TestRunOncePersists(t *testing.T) {
	dir := t.TempDir()
	chat := chatlog.New(dir)
	chat.Log(snowflake.ID(1), "Hallo Welt")
	rx := reactions.New(dir)
	rx.LogReaction(snowflake.ID(1), snowflake.ID(0), "🍕")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(chatResponse{Choices: []chatChoice{{Message: chatMessage{Content: "Kevin Profil"}}}})
	}))
	defer server.Close()

	p := New(Config{APIKey: "test-key", Model: "big-pickle", BaseURL: server.URL, Dir: dir}, chat, rx)
	p.httpClient = server.Client()
	if got := p.RunOnce(); got != 1 {
		t.Fatalf("expected 1 updated profile, got %d", got)
	}

	prof, ok := p.Get(snowflake.ID(1))
	if !ok {
		t.Fatalf("expected profile stored")
	}
	if prof.Text != "Kevin Profil" {
		t.Fatalf("got %q", prof.Text)
	}
}
