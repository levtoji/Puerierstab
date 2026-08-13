package reactions

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

const retention = 90 * 24 * time.Hour

type Reaction struct {
	ActorID      snowflake.ID `json:"actor_id"`
	TargetUserID snowflake.ID `json:"target_user_id"`
	Emoji        string       `json:"emoji"`
	Timestamp    time.Time    `json:"timestamp"`
}

type Logger struct {
	reactions map[snowflake.ID][]Reaction
	filePath  string
	mu        sync.RWMutex
}

func New(dir string) *Logger {
	l := &Logger{
		reactions: make(map[snowflake.ID][]Reaction),
		filePath:  filepath.Join(dir, ".reactions.json"),
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

func (l *Logger) LogReaction(actorID, targetID snowflake.ID, emoji string) {
	if emoji == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.reactions[actorID] = append(l.reactions[actorID], Reaction{
		ActorID:      actorID,
		TargetUserID: targetID,
		Emoji:        emoji,
		Timestamp:    time.Now(),
	})
	l.save()
}

func (l *Logger) OnReactionAdd(event *events.GuildMessageReactionAdd) {
	if event.UserID == event.Client().ApplicationID {
		return
	}
	emoji := emojiString(event.Emoji)
	if emoji == "" {
		return
	}
	var targetID snowflake.ID
	if event.MessageAuthorID != nil {
		targetID = *event.MessageAuthorID
	}
	l.LogReaction(event.UserID, targetID, emoji)
}

func emojiString(e discord.PartialEmoji) string {
	if e.Name != nil {
		return *e.Name
	}
	if e.ID != nil {
		return e.ID.String()
	}
	return ""
}

func (l *Logger) Stats(userID snowflake.ID, maxAge time.Duration) (map[string]int, map[string]int) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	cutoff := time.Now().Add(-maxAge)
	given := make(map[string]int)
	received := make(map[string]int)
	for _, r := range l.reactions[userID] {
		if r.Timestamp.Before(cutoff) {
			continue
		}
		given[r.Emoji]++
	}
	for actorID, rs := range l.reactions {
		if actorID == userID {
			continue
		}
		for _, r := range rs {
			if r.Timestamp.Before(cutoff) {
				continue
			}
			if r.TargetUserID == userID {
				received[r.Emoji]++
			}
		}
	}
	return given, received
}

func (l *Logger) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-retention)
	for userID, rs := range l.reactions {
		var kept []Reaction
		for _, r := range rs {
			if r.Timestamp.After(cutoff) {
				kept = append(kept, r)
			}
		}
		if len(kept) == 0 {
			delete(l.reactions, userID)
		} else {
			l.reactions[userID] = kept
		}
	}
	l.save()
}

func (l *Logger) save() {
	tmpPath := l.filePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		slog.Warn("failed to create reactions save file", slog.Any("err", err))
		return
	}
	if err := json.NewEncoder(f).Encode(l.reactions); err != nil {
		f.Close()
		slog.Warn("failed to encode reactions", slog.Any("err", err))
		return
	}
	f.Close()
	if err := os.Rename(tmpPath, l.filePath); err != nil {
		slog.Warn("failed to rename reactions save file", slog.Any("err", err))
	}
}

func (l *Logger) load() {
	data, err := os.ReadFile(l.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read reactions file", slog.Any("err", err))
		}
		return
	}
	if err := json.Unmarshal(data, &l.reactions); err != nil {
		slog.Warn("failed to parse reactions file", slog.Any("err", err))
		l.reactions = make(map[snowflake.ID][]Reaction)
		return
	}
	l.cleanup()
	slog.Info("loaded reactions", slog.Int("users", len(l.reactions)))
}
