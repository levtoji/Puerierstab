package reactions

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func newLogger(t *testing.T) *Logger {
	t.Helper()
	return &Logger{
		reactions: make(map[snowflake.ID][]Reaction),
		filePath:  filepath.Join(t.TempDir(), ".reactions.json"),
	}
}

func TestLogReactionGivenAndReceived(t *testing.T) {
	l := newLogger(t)
	kevin := snowflake.ID(1)
	paul := snowflake.ID(2)

	l.LogReaction(kevin, snowflake.ID(0), "🍕")
	l.LogReaction(kevin, paul, "😂")

	given, received := l.Stats(kevin, 90*24*time.Hour)
	if given["🍕"] != 1 || given["😂"] != 1 {
		t.Fatalf("expected given counts, got %v", given)
	}
	if len(received) != 0 {
		t.Fatalf("expected no received reactions for kevin, got %v", received)
	}

	given2, received2 := l.Stats(paul, 90*24*time.Hour)
	if len(given2) != 0 {
		t.Fatalf("expected no given reactions for paul, got %v", given2)
	}
	if received2["😂"] != 1 {
		t.Fatalf("expected paul received count, got %v", received2)
	}
}

func TestStatsMaxAge(t *testing.T) {
	l := newLogger(t)
	kevin := snowflake.ID(1)
	l.reactions[kevin] = []Reaction{
		{ActorID: kevin, TargetUserID: 0, Emoji: "alt", Timestamp: time.Now().Add(-100 * 24 * time.Hour)},
		{ActorID: kevin, TargetUserID: 0, Emoji: "neu", Timestamp: time.Now()},
	}
	given, _ := l.Stats(kevin, 90*24*time.Hour)
	if given["neu"] != 1 {
		t.Fatalf("expected only 'neu', got %v", given)
	}
	if _, ok := given["alt"]; ok {
		t.Fatalf("expected 'alt' filtered out, got %v", given)
	}
}

func TestCleanup(t *testing.T) {
	l := newLogger(t)
	kevin := snowflake.ID(1)
	l.reactions[kevin] = []Reaction{
		{ActorID: kevin, TargetUserID: 0, Emoji: "alt", Timestamp: time.Now().Add(-100 * 24 * time.Hour)},
		{ActorID: kevin, TargetUserID: 0, Emoji: "neu", Timestamp: time.Now()},
	}
	l.cleanup()
	if len(l.reactions[kevin]) != 1 || l.reactions[kevin][0].Emoji != "neu" {
		t.Fatalf("expected only 'neu' after cleanup, got %v", l.reactions[kevin])
	}
}

func TestSaveAndLoad(t *testing.T) {
	fp := filepath.Join(t.TempDir(), ".reactions.json")
	l := &Logger{reactions: make(map[snowflake.ID][]Reaction), filePath: fp}
	l.LogReaction(snowflake.ID(1), snowflake.ID(2), "🔥")

	l2 := &Logger{reactions: make(map[snowflake.ID][]Reaction), filePath: fp}
	l2.load()
	given, _ := l2.Stats(snowflake.ID(1), 90*24*time.Hour)
	if given["🔥"] != 1 {
		t.Fatalf("expected given count for user 1, got %v", given)
	}
	given2, received2 := l2.Stats(snowflake.ID(2), 90*24*time.Hour)
	if len(given2) != 0 {
		t.Fatalf("expected no given reactions for user 2, got %v", given2)
	}
	if received2["🔥"] != 1 {
		t.Fatalf("expected received count for user 2, got %v", received2)
	}
}
