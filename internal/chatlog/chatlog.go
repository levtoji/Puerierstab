package chatlog

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

type Entry struct {
	UserID    snowflake.ID `json:"user_id"`
	Content   string       `json:"content"`
	Timestamp time.Time    `json:"timestamp"`
}

type Logger struct {
	entries  map[snowflake.ID][]Entry
	filePath string
	mu       sync.RWMutex
}

func New(dir string) *Logger {
	l := &Logger{
		entries:  make(map[snowflake.ID][]Entry),
		filePath: filepath.Join(dir, ".chatlog.json"),
	}
	l.load()
	return l
}

func (l *Logger) StartCleanup() chan struct{} {
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(1 * time.Hour):
			}
			l.cleanup()
		}
	}()
	return stop
}

func (l *Logger) Log(userID snowflake.ID, content string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries[userID] = append(l.entries[userID], Entry{
		UserID:    userID,
		Content:   content,
		Timestamp: time.Now(),
	})
	l.save()
}

// OnMessageCreate is the event listener for GuildMessageCreate
func (l *Logger) OnMessageCreate(event *events.GuildMessageCreate) {
	if event.Message.Author.Bot {
		return
	}
	l.Log(event.Message.Author.ID, event.Message.Content)
}

func (l *Logger) UserCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.entries)
}

func (l *Logger) AllUsers() []snowflake.ID {
	l.mu.RLock()
	defer l.mu.RUnlock()
	users := make([]snowflake.ID, 0, len(l.entries))
	for userID := range l.entries {
		users = append(users, userID)
	}
	return users
}

func (l *Logger) GetMessages(userID snowflake.ID, maxAge time.Duration) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	cutoff := time.Now().Add(-maxAge)
	var messages []string
	for _, e := range l.entries[userID] {
		if e.Timestamp.After(cutoff) {
			messages = append(messages, e.Content)
		}
	}
	return messages
}

func (l *Logger) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	for userID, entries := range l.entries {
		var kept []Entry
		for _, e := range entries {
			if e.Timestamp.After(cutoff) {
				kept = append(kept, e)
			}
		}
		if len(kept) == 0 {
			delete(l.entries, userID)
		} else {
			l.entries[userID] = kept
		}
	}
	l.save()
}

func (l *Logger) save() {
	tmpPath := l.filePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		slog.Warn("failed to create chatlog save file", slog.Any("err", err))
		return
	}
	if err := json.NewEncoder(f).Encode(l.entries); err != nil {
		f.Close()
		slog.Warn("failed to encode chatlog", slog.Any("err", err))
		return
	}
	f.Close()
	if err := os.Rename(tmpPath, l.filePath); err != nil {
		slog.Warn("failed to rename chatlog save file", slog.Any("err", err))
	}
}

func (l *Logger) load() {
	data, err := os.ReadFile(l.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read chatlog file", slog.Any("err", err))
		}
		return
	}
	if err := json.Unmarshal(data, &l.entries); err != nil {
		slog.Warn("failed to parse chatlog file", slog.Any("err", err))
		l.entries = make(map[snowflake.ID][]Entry)
		return
	}
	l.cleanup()
	slog.Info("loaded chatlog", slog.Int("users", len(l.entries)))
}
