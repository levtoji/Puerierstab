package poll

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestNewStore(t *testing.T) {
	store := NewStore()
	if store == nil {
		t.Fatalf("expected non-nil store")
	}
}

func TestCreateAndGetPoll(t *testing.T) {
	store := NewStore()
	poll := store.Create("Test?", []string{"A", "B", "C"}, snowflake.ID(123))

	if poll.ID == "" {
		t.Fatalf("expected non-empty poll ID")
	}
	if poll.Question != "Test?" {
		t.Fatalf("expected question, got %q", poll.Question)
	}
	if len(poll.Options) != 3 {
		t.Fatalf("expected 3 options, got %d", len(poll.Options))
	}
	if len(poll.Votes) != 0 {
		t.Fatalf("expected empty votes")
	}

	got, ok := store.Get(poll.ID)
	if !ok {
		t.Fatalf("expected to find poll by ID")
	}
	if got.Question != "Test?" {
		t.Fatalf("retrieved poll has wrong question")
	}
}

func TestToggleVote(t *testing.T) {
	poll := &Poll{
		ID:      "test-id",
		Options: []string{"A", "B"},
		Votes:   make(map[int]map[snowflake.ID]struct{}),
	}
	userID := snowflake.ID(1)

	poll.ToggleVote(0, userID)
	if poll.VoteCount(0) != 1 {
		t.Fatalf("expected 1 vote after toggle, got %d", poll.VoteCount(0))
	}

	poll.ToggleVote(0, userID)
	if poll.VoteCount(0) != 0 {
		t.Fatalf("expected 0 votes after second toggle, got %d", poll.VoteCount(0))
	}
}

func TestMultiChoiceVoting(t *testing.T) {
	poll := &Poll{
		ID:      "test-id",
		Options: []string{"A", "B", "C"},
		Votes:   make(map[int]map[snowflake.ID]struct{}),
	}
	user := snowflake.ID(1)

	poll.ToggleVote(0, user)
	poll.ToggleVote(2, user)

	if poll.VoteCount(0) != 1 || poll.VoteCount(1) != 0 || poll.VoteCount(2) != 1 {
		t.Fatalf("expected votes on options 0 and 2")
	}

	if !poll.HasVoted(user, 0) || poll.HasVoted(user, 1) || !poll.HasVoted(user, 2) {
		t.Fatalf("HasVoted returned wrong results")
	}
}

func TestParseOptions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
		wantErr  bool
	}{
		{"simple", "A, B, C", []string{"A", "B", "C"}, false},
		{"trim spaces", "  A ,  B  , C ", []string{"A", "B", "C"}, false},
		{"single", "Nur eins", nil, true},
		{"empty parts ignored", "A,,B", []string{"A", "B"}, false},
		{"too many", "A,B,C,D,E,F,G", nil, true},
		{"too few", "A", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOptions(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d options, got %d", len(tt.expected), len(got))
			}
			for i := range tt.expected {
				if got[i] != tt.expected[i] {
					t.Fatalf("option %d: expected %q, got %q", i, tt.expected[i], got[i])
				}
			}
		})
	}
}

func TestBuildComponents(t *testing.T) {
	poll := &Poll{
		ID:      "abc",
		Options: []string{"Ja", "Nein", "Vielleicht"},
	}
	components := poll.Components()
	if len(components) != 3 {
		t.Fatalf("expected 3 action rows, got %d", len(components))
	}
	for i, row := range components {
		actionRow, ok := row.(discord.ActionRowComponent)
		if !ok {
			t.Fatalf("expected ActionRowComponent, got %T", row)
		}
		if len(actionRow.Components) != 1 {
			t.Fatalf("expected 1 button per row, got %d", len(actionRow.Components))
		}
		btn := actionRow.Components[0].(discord.ButtonComponent)
		wantID := "poll:abc:" + itoa(i)
		if btn.CustomID != wantID {
			t.Fatalf("expected customID %q, got %q", wantID, btn.CustomID)
		}
		if !strings.Contains(btn.Label, poll.Options[i]) {
			t.Fatalf("expected label to contain option text %q, got %q", poll.Options[i], btn.Label)
		}
	}
}

func TestPollEmbed(t *testing.T) {
	poll := &Poll{
		Question:  "Was gibt's?",
		Options:   []string{"Pizza", "Pasta"},
		Votes:     make(map[int]map[snowflake.ID]struct{}),
		CreatedAt: time.Now(),
	}
	poll.ToggleVote(0, snowflake.ID(1))
	poll.ToggleVote(0, snowflake.ID(2))
	poll.ToggleVote(1, snowflake.ID(3))

	embed := poll.Embed()
	if embed.Title != "Was gibt's?" {
		t.Fatalf("expected question as title, got %q", embed.Title)
	}
	if embed.Color != 0x5865F2 {
		t.Fatalf("expected color 0x5865F2, got 0x%X", embed.Color)
	}
	if len(embed.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(embed.Fields))
	}
	if !strings.Contains(embed.Fields[0].Name, "Pizza") {
		t.Fatalf("expected field name to contain Pizza, got %q", embed.Fields[0].Name)
	}
	if !strings.Contains(embed.Fields[0].Value, "2") {
		t.Fatalf("expected field value to contain 2 votes, got %q", embed.Fields[0].Value)
	}
}

func TestParsePollCustomID(t *testing.T) {
	id, idx, ok := ParseCustomID("poll:test123:2")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if id != "test123" {
		t.Fatalf("expected id test123, got %q", id)
	}
	if idx != 2 {
		t.Fatalf("expected idx 2, got %d", idx)
	}
}

func TestParsePollCustomIDInvalid(t *testing.T) {
	_, _, ok := ParseCustomID("role_toggle:abc")
	if ok {
		t.Fatalf("expected ok=false for non-poll customID")
	}
	_, _, ok = ParseCustomID("poll:abc")
	if ok {
		t.Fatalf("expected ok=false for malformed poll customID")
	}
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

func TestPollCreatedAt(t *testing.T) {
	before := time.Now()
	store := NewStore()
	poll := store.Create("Test?", []string{"A", "B"}, snowflake.ID(1))
	after := time.Now()

	if poll.CreatedAt.Before(before) || poll.CreatedAt.After(after) {
		t.Fatalf("CreatedAt %v not between %v and %v", poll.CreatedAt, before, after)
	}
}

func TestPollIsExpired(t *testing.T) {
	p := &Poll{
		ID:        "test",
		CreatedAt: time.Now().Add(-25 * time.Hour),
	}
	if !p.IsExpired() {
		t.Fatal("expected expired poll")
	}

	p.CreatedAt = time.Now()
	if p.IsExpired() {
		t.Fatal("expected fresh poll not expired")
	}
}

func TestPollEmbedExpiredTitle(t *testing.T) {
	p := &Poll{
		Question:  "Was gibt's?",
		Options:   []string{"A", "B"},
		Votes:     make(map[int]map[snowflake.ID]struct{}),
		CreatedAt: time.Now().Add(-25 * time.Hour),
	}

	embed := p.Embed()
	if !strings.Contains(embed.Title, "(abgelaufen)") {
		t.Fatalf("expected expired title, got %q", embed.Title)
	}
}

func TestPollEmbedFreshTitle(t *testing.T) {
	p := &Poll{
		Question:  "Was gibt's?",
		Options:   []string{"A", "B"},
		Votes:     make(map[int]map[snowflake.ID]struct{}),
		CreatedAt: time.Now(),
	}

	embed := p.Embed()
	if strings.Contains(embed.Title, "(abgelaufen)") {
		t.Fatalf("fresh poll should not have expired title, got %q", embed.Title)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	store := &Store{
		polls:    make(map[string]*Poll),
		filePath: filepath.Join(dir, ".polls.json"),
	}

	store.Create("Frage 1", []string{"A", "B"}, snowflake.ID(1))
	store.Create("Frage 2", []string{"X", "Y", "Z"}, snowflake.ID(2))

	store2 := &Store{
		polls:    make(map[string]*Poll),
		filePath: filepath.Join(dir, ".polls.json"),
	}
	store2.load()

	if len(store2.polls) != 2 {
		t.Fatalf("expected 2 polls after load, got %d", len(store2.polls))
	}
}

func TestLoadFiltersExpiredPolls(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, ".polls.json")

	store := &Store{
		polls:    make(map[string]*Poll),
		filePath: fp,
	}

	store.polls["fresh"] = &Poll{
		ID:        "fresh",
		Question:  "Fresh",
		Options:   []string{"A", "B"},
		Votes:     make(map[int]map[snowflake.ID]struct{}),
		CreatedAt: time.Now(),
	}
	store.polls["stale"] = &Poll{
		ID:        "stale",
		Question:  "Stale",
		Options:   []string{"A", "B"},
		Votes:     make(map[int]map[snowflake.ID]struct{}),
		CreatedAt: time.Now().Add(-48 * time.Hour),
	}
	store.save()

	store2 := &Store{
		polls:    make(map[string]*Poll),
		filePath: fp,
	}
	store2.load()

	if len(store2.polls) != 1 {
		t.Fatalf("expected 1 poll after filtering expired, got %d", len(store2.polls))
	}
	if _, ok := store2.polls["fresh"]; !ok {
		t.Fatal("expected fresh poll to survive load")
	}
	if _, ok := store2.polls["stale"]; ok {
		t.Fatal("expected stale poll to be filtered out")
	}
}

func TestToggleVoteSaves(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, ".polls.json")

	store := &Store{
		polls:    make(map[string]*Poll),
		filePath: fp,
	}
	p := store.Create("Test?", []string{"A", "B"}, snowflake.ID(1))
	store.ToggleVote(p.ID, 0, snowflake.ID(99))

	store2 := &Store{
		polls:    make(map[string]*Poll),
		filePath: fp,
	}
	store2.load()

	loaded, ok := store2.Get(p.ID)
	if !ok {
		t.Fatal("expected to find saved poll")
	}
	if loaded.VoteCount(0) != 1 {
		t.Fatalf("expected 1 vote after load, got %d", loaded.VoteCount(0))
	}
}
