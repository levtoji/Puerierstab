package chatlog

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestLogAndGet(t *testing.T) {
	dir := t.TempDir()
	l := &Logger{
		entries:  make(map[snowflake.ID][]Entry),
		filePath: filepath.Join(dir, ".chatlog.json"),
	}

	userID := snowflake.ID(1)
	l.Log(userID, "hallo")
	l.Log(userID, "welt")

	msgs := l.GetMessages(userID, 30*24*time.Hour)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0] != "hallo" || msgs[1] != "welt" {
		t.Fatalf("got wrong messages: %v", msgs)
	}
}

func TestGetMessagesMaxAge(t *testing.T) {
	dir := t.TempDir()
	l := &Logger{
		entries:  make(map[snowflake.ID][]Entry),
		filePath: filepath.Join(dir, ".chatlog.json"),
	}

	userID := snowflake.ID(1)
	l.entries[userID] = []Entry{
		{UserID: userID, Content: "alt", Timestamp: time.Now().Add(-48 * time.Hour)},
		{UserID: userID, Content: "neu", Timestamp: time.Now()},
	}

	msgs := l.GetMessages(userID, 24*time.Hour)
	if len(msgs) != 1 || msgs[0] != "neu" {
		t.Fatalf("expected only 'neu', got %v", msgs)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, ".chatlog.json")

	l := &Logger{entries: make(map[snowflake.ID][]Entry), filePath: fp}
	l.Log(snowflake.ID(1), "test message")

	l2 := &Logger{entries: make(map[snowflake.ID][]Entry), filePath: fp}
	l2.load()

	msgs := l2.GetMessages(snowflake.ID(1), 30*24*time.Hour)
	if len(msgs) != 1 || msgs[0] != "test message" {
		t.Fatalf("expected 'test message', got %v", msgs)
	}
}

func TestCleanup(t *testing.T) {
	dir := t.TempDir()
	l := &Logger{
		entries:  make(map[snowflake.ID][]Entry),
		filePath: filepath.Join(dir, ".chatlog.json"),
	}

	userID := snowflake.ID(1)
	l.entries[userID] = []Entry{
		{UserID: userID, Content: "alt", Timestamp: time.Now().Add(-40 * 24 * time.Hour)},
		{UserID: userID, Content: "neu", Timestamp: time.Now()},
	}

	l.cleanup()

	entries := l.entries[userID]
	if len(entries) != 1 || entries[0].Content != "neu" {
		t.Fatalf("expected only 'neu' after cleanup, got %v", l.entries[userID])
	}
}
