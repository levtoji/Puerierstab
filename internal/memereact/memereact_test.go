package memereact

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
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

func TestReactionCount(t *testing.T) {
	msg := func(reactions []discord.MessageReaction) *discord.Message {
		return &discord.Message{Reactions: reactions}
	}

	if c := reactionCount(msg(nil)); c != 0 {
		t.Fatalf("expected 0 for no reactions, got %d", c)
	}
	if c := reactionCount(msg([]discord.MessageReaction{
		{Count: 1, Emoji: discord.Emoji{Name: "a"}},
		{Count: 1, Emoji: discord.Emoji{Name: "b"}},
	})); c != 2 {
		t.Fatalf("expected 2 for two different single reactions, got %d", c)
	}
	if c := reactionCount(msg([]discord.MessageReaction{
		{Count: 2, Emoji: discord.Emoji{Name: "a"}},
	})); c != 2 {
		t.Fatalf("expected 2 for one emoji with count 2, got %d", c)
	}
	if c := reactionCount(msg([]discord.MessageReaction{
		{Count: 3, Me: true, Emoji: discord.Emoji{Name: "a"}},
		{Count: 1, Emoji: discord.Emoji{Name: "b"}},
	})); c != 3 {
		t.Fatalf("expected bot's own reaction excluded (1+2), got %d", c)
	}
}
