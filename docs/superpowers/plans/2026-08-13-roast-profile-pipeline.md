# Roast-Verbesserung + Profil-Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `/roast` auf 90-Tage-Daten + Reaktionen + gespeichertem Persönlichkeitsprofil basieren lassen, mit besserem Fallback-Modell (`gpt-5.4`) und einer täglichen Hintergrund-Profil-Pipeline.

**Architecture:** Drei neue/unabhängige Bausteine: (1) `internal/reactions` sammelt Emoji-Reaktionen (gegeben/bekommen) live aus dem Gateway und persistiert sie in `.reactions.json`; (2) `internal/profile` erstellt täglich um 3 Uhr per AI ein Persönlichkeitsprofil pro User aus Chatlog + Reaktionen und speichert es in `.profiles.json`; (3) `roast.go` erweitert den Roast-Prompt um 90d-Fenster, Reaktions-Stats und das Profil. Chatlog-Retention und Backfill-Cutoff werden von 30d auf 90d angehoben.

**Tech Stack:** Go 1.24, `github.com/disgoorg/disgo` v0.19.6 (Gateway-Events), `github.com/disgoorg/snowflake/v2`, httptest für AI-Stubs, OpenAI-kompatibles Chat-Completions-API (`https://opencode.ai/zen/v1`).

---

## Datei-Struktur

| Datei | Status | Verantwortung |
|---|---|---|
| `internal/reactions/reactions.go` | neu | Reaction-Store (`.reactions.json`), `OnReactionAdd`-Listener, `Stats()` |
| `internal/reactions/reactions_test.go` | neu | Store save/load/cleanup, LogReaction, Stats |
| `internal/profile/profile.go` | neu | Profil-Store (`.profiles.json`), AI-Call mit Fallback, `RunOnce`, `Start` (3 Uhr) |
| `internal/profile/profile_test.go` | neu | nextSchedule, buildProfilePrompt, AI-Stub, Fallback |
| `internal/chatlog/chatlog.go` | ändern | `cleanup()` 30d → 90d |
| `internal/chatlog/chatlog_test.go` | ändern | Cleanup-Test auf 90d anpassen |
| `commands.go` | ändern | Backfill-Cutoff 30d → 90d, Meldungstext, Globals `reactionLog`, `profilePipeline` |
| `roast.go` | ändern | 90d-Fenster, Temp 1.0, Prompt-Härtung, Reaktionen + Profil im Prompt |
| `roast_test.go` | ändern | Tests für `buildRoastPrompt` (mit/ohne Profil/Reaktionen) |
| `main.go` | ändern | Reactions-Listener + Profiler-Wiring |
| `AGENTS.md` | ändern | Retention 90d, neue Pakete, Fallback-Beispiel `gpt-5.4` |

Konventionen aus dem Codebase, die alle Schritte einhalten:
- Tests sind `package foo` (intern), Zugriff auf unexportierte Symbole.
- Store-Persistenz: tmp-File schreiben + `os.Rename`, bei Fehler nur `slog.Warn` + Skip.
- AI-Fallback-Regel (identisch in roast.go/memereact/channelnamer): bei `client.Do`-Fehler ODER Status 429/5xx einmal mit Fallback-Modell retry; Guard `model == fallbackModel` verhindert Endlosschleife.
- `slog.Warn("AI primary model failed, falling back", slog.String("from", ...), slog.String("to", ...), slog.Any("err", err))`.

---

## Task 1: Chatlog-Retention auf 90 Tage

**Files:**
- Modify: `internal/chatlog/chatlog.go:112`
- Modify: `internal/chatlog/chatlog_test.go:66-85`

- [ ] **Step 1: Test anpassen (90d-Retention)**

In `internal/chatlog/chatlog_test.go` `TestCleanup` ändern — der "alt"-Eintrag muss älter als 90d sein, damit er entfernt wird, und der "mittel"-Fall (40d) muss erhalten bleiben:

```go
func TestCleanup(t *testing.T) {
	dir := t.TempDir()
	l := &Logger{
		entries:  make(map[snowflake.ID][]Entry),
		filePath: filepath.Join(dir, ".chatlog.json"),
	}

	userID := snowflake.ID(1)
	l.entries[userID] = []Entry{
		{UserID: userID, Content: "alt", Timestamp: time.Now().Add(-100 * 24 * time.Hour)},
		{UserID: userID, Content: "mittel", Timestamp: time.Now().Add(-40 * 24 * time.Hour)},
		{UserID: userID, Content: "neu", Timestamp: time.Now()},
	}

	l.cleanup()

	entries := l.entries[userID]
	if len(entries) != 2 || entries[0].Content != "mittel" || entries[1].Content != "neu" {
		t.Fatalf("expected 'mittel' and 'neu' after cleanup, got %v", l.entries[userID])
	}
}
```

- [ ] **Step 2: Test läuft rot**

Run: `go test ./internal/chatlog/ -run TestCleanup -v`
Expected: FAIL — "alt" (100d) wird von `cleanup()` mit 30d-Cutoff entfernt, "mittel" (40d) wird aber AUCH entfernt → `len(entries)` ist 1 statt 2.

- [ ] **Step 3: Cleanup-Cutoff auf 90d ändern**

In `internal/chatlog/chatlog.go` in `cleanup()`:

```go
	cutoff := time.Now().Add(-90 * 24 * time.Hour)
```

- [ ] **Step 4: Test läuft grün**

Run: `go test ./internal/chatlog/ -run TestCleanup -v`
Expected: PASS.

- [ ] **Step 5: Backfill-Cutoff + Meldung auf 90d**

In `commands.go` (Zeile ~290):

```go
		cutoff := time.Now().Add(-90 * 24 * time.Hour)
```

und in der Antwort (Zeile ~332):

```go
		respond(event, fmt.Sprintf("Chatlog neu aufgebaut: %d Nachrichten von %d Usern (90 Tage).", len(entries), chatLog.UserCount()))
```

- [ ] **Step 6: Alle Tests grün**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: alles ok.

- [ ] **Step 7: Commit**

```bash
git add internal/chatlog/chatlog.go internal/chatlog/chatlog_test.go commands.go
git commit -m "feat: chatlog retention und backfill auf 90 tage erweitert"
```

---

## Task 2: Reaktions-Store (internal/reactions)

**Files:**
- Create: `internal/reactions/reactions.go`
- Create: `internal/reactions/reactions_test.go`

Konzept: `Logger` hält `map[snowflake.ID][]Reaction` (je Actor), persistiert in `.reactions.json`, Cleanup 90d. `OnReactionAdd` nimmt das Gateway-Event entgegen und delegiert die eigentliche Logik an `LogReaction(actorID, targetID, emoji)` (testbar ohne Event-Konstruktion). `MessageAuthorID` kommt direkt aus dem `MESSAGE_REACTION_ADD`-Payload von Discord.

- [ ] **Step 1: Failing Tests schreiben**

`internal/reactions/reactions_test.go`:

```go
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
	if received["😂"] != 1 {
		t.Fatalf("expected received count for paul's message, got %v", received)
	}

	given2, _ := l.Stats(paul, 90*24*time.Hour)
	if given2["😂"] != 1 {
		t.Fatalf("expected paul given count, got %v", given2)
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
	given, received := l2.Stats(snowflake.ID(1), 90*24*time.Hour)
	if given["🔥"] != 1 || received[snowflake.ID(2).String()] != 0 && len(received) != 1 {
		t.Fatalf("unexpected stats after load: given=%v received=%v", given, received)
	}
}
```

- [ ] **Step 2: Tests laufen rot**

Run: `go test ./internal/reactions/ -v`
Expected: FAIL — Paket existiert nicht ("package reactions is not in std").

- [ ] **Step 3: Store implementieren**

`internal/reactions/reactions.go`:

```go
package reactions

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

func emojiString(e interface{ Name() *string }) string {
	if n := e.Name(); n != nil {
		return *n
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
```

**Hinweis zu `emojiString`:** `discord.PartialEmoji` hat `Name *string` als Feld. Statt einer Interface-Adapterfunktion ist die direkte Implementierung sauberer:

```go
func emojiString(e discord.PartialEmoji) string {
	if e.Name != nil {
		return *e.Name
	}
	if e.ID != nil {
		return e.ID.String()
	}
	return ""
}
```

Ersetze die Interface-Variante oben durch diese. Import dafür: `"github.com/disgoorg/disgo/discord"`.

- [ ] **Step 4: Tests laufen grün**

Run: `go test ./internal/reactions/ -v`
Expected: PASS.

**Hinweis zu `TestSaveAndLoad`:** Der Test prüft `received` unsauber. Ersetze die Assertion durch:

```go
	given, received := l2.Stats(snowflake.ID(1), 90*24*time.Hour)
	if given["🔥"] != 1 {
		t.Fatalf("expected given count, got %v", given)
	}
	if received["🔥"] != 1 {
		t.Fatalf("expected received count for user 2, got %v", received)
	}
```

- [ ] **Step 5: gofmt + Tests**

Run: `gofmt -l internal/reactions/` (leer) und `go build ./... && go test ./... && go vet ./...`
Expected: alles ok.

- [ ] **Step 6: Commit**

```bash
git add internal/reactions/
git commit -m "feat: reactions store für emoji-reaktionen (gegeben/bekommen)"
```

---

## Task 3: Profil-Pipeline (internal/profile)

**Files:**
- Create: `internal/profile/profile.go`
- Create: `internal/profile/profile_test.go`

Konzept: `Profiler` hält `Config` (AI-Keys + Dir), refs auf `*chatlog.Logger` und `*reactions.Logger`, sowie `Store` (`.profiles.json`). `RunOnce()` iteriert User mit Nachrichten, baut Prompt aus 90d Nachrichten + Reaktions-Stats, ruft AI (mit Fallback), speichert Profil. `Start()` plant täglich 3 Uhr.

- [ ] **Step 1: Failing Tests schreiben**

`internal/profile/profile_test.go`:

```go
package profile

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
		if req.Temperature != 0.8 {
			t.Fatalf("expected temperature 0.8, got %f", req.Temperature)
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
	p.RunOnce()

	prof, ok := p.Get(snowflake.ID(1))
	if !ok {
		t.Fatalf("expected profile stored")
	}
	if prof.Text != "Kevin Profil" {
		t.Fatalf("got %q", prof.Text)
	}
}
```

- [ ] **Step 2: Tests laufen rot**

Run: `go test ./internal/profile/ -v`
Expected: FAIL — Paket existiert nicht.

- [ ] **Step 3: Profiler implementieren**

`internal/profile/profile.go`:

```go
package profile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/levtoji/Puerierstab/internal/chatlog"
	"github.com/levtoji/Puerierstab/internal/reactions"
)

const (
	maxHistoryChars = 1500
	window          = 90 * 24 * time.Hour
)

type Config struct {
	APIKey        string
	Model         string
	FallbackModel string
	BaseURL       string
	Dir           string
}

type Profile struct {
	Text      string    `json:"text"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store struct {
	Profiles map[snowflake.ID]Profile `json:"profiles"`
	filePath string
	mu       sync.RWMutex
}

type Profiler struct {
	cfg        Config
	chat       *chatlog.Logger
	reactions  *reactions.Logger
	store      *Store
	httpClient *http.Client
}

func New(cfg Config, chat *chatlog.Logger, rx *reactions.Logger) *Profiler {
	if cfg.APIKey == "" {
		return nil
	}
	if cfg.Model == "" {
		cfg.Model = "big-pickle"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://opencode.ai/zen/v1"
	}
	store := &Store{Profiles: make(map[snowflake.ID]Profile), filePath: filepath.Join(cfg.Dir, ".profiles.json")}
	store.load()
	return &Profiler{
		cfg:        cfg,
		chat:       chat,
		reactions:  rx,
		store:      store,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *Profiler) Get(userID snowflake.ID) (Profile, bool) {
	p.store.mu.RLock()
	defer p.store.mu.RUnlock()
	prof, ok := p.store.Profiles[userID]
	return prof, ok
}

func (p *Profiler) Start() chan struct{} {
	stop := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("profile pipeline panic", slog.Any("panic", r))
			}
		}()
		for {
			if !sleepUntil(stop, nextSchedule()) {
				return
			}
			p.RunOnce()
		}
	}()
	return stop
}

func sleepUntil(stop chan struct{}, d time.Duration) bool {
	t := time.NewTimer(d)
	select {
	case <-stop:
		t.Stop()
		return false
	case <-t.C:
		return true
	}
}

func nextSchedule() time.Duration {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	if next.Before(now) || next.Equal(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}

func (p *Profiler) RunOnce() {
	if p.chat == nil {
		return
	}
	for _, userID := range p.chat.AllUsers() {
		msgs := p.chat.GetMessages(userID, window)
		if len(msgs) == 0 {
			continue
		}
		history := trimHistory(msgs)
		var given, received map[string]int
		if p.reactions != nil {
			given, received = p.reactions.Stats(userID, window)
		}

		name := "User"
		text, err := p.generateProfile(userID, name, history, given, received)
		if err != nil {
			slog.Warn("profile generation failed", slog.String("user_id", userID.String()), slog.Any("err", err))
			continue
		}
		if text == "" {
			continue
		}
		p.store.mu.Lock()
		p.store.Profiles[userID] = Profile{Text: text, UpdatedAt: time.Now()}
		p.store.save()
		p.store.mu.Unlock()
		slog.Info("profile updated", slog.String("user_id", userID.String()))
	}
}

func trimHistory(msgs []string) string {
	var trimmed []string
	total := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		if total+len(msgs[i]) > maxHistoryChars {
			break
		}
		trimmed = append([]string{msgs[i]}, trimmed...)
		total += len(msgs[i])
	}
	return strings.Join(trimmed, "\n")
}

func formatEmojiTop(counts map[string]int, limit int) string {
	if len(counts) == 0 {
		return ""
	}
	type pair struct {
		emoji string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for e, c := range counts {
		pairs = append(pairs, pair{e, c})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].emoji < pairs[j].emoji
	})
	var parts []string
	for i, p := range pairs {
		if i >= limit {
			break
		}
		parts = append(parts, fmt.Sprintf("%s (%d)", p.emoji, p.count))
	}
	return strings.Join(parts, ", ")
}

func buildProfilePrompt(name, history string, given, received map[string]int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Nachrichten von @%s (letzte 3 Monate):\n%s\n", name, history))
	if g := formatEmojiTop(given, 5); g != "" {
		b.WriteString(fmt.Sprintf("\nTop-Reaktionen, die @%s vergibt: %s\n", name, g))
	}
	if r := formatEmojiTop(received, 5); r != "" {
		b.WriteString(fmt.Sprintf("\nTop-Reaktionen auf @%s's Nachrichten: %s\n", name, r))
	}
	b.WriteString("\nSchreibe ein Persönlichkeitsprofil von 2-3 Sätzen. Präzise, witzig, aber nicht gemein. Deutsch.")
	return b.String()
}

type chatRequest struct {
	Model       string        `json:"model"`
	Temperature float64       `json:"temperature"`
	Messages    []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

func (p *Profiler) generateProfile(userID snowflake.ID, name, history string, given, received map[string]int) (string, error) {
	return p.generateProfileWithModel(userID, name, history, given, received, p.cfg.Model)
}

func (p *Profiler) generateProfileWithModel(userID snowflake.ID, name, history string, given, received map[string]int, model string) (string, error) {
	prompt := buildProfilePrompt(name, history, given, received)
	reqBody := chatRequest{
		Model:       model,
		Temperature: 0.8,
		Messages: []chatMessage{
			{Role: "system", Content: "Du erstellst ein knappes Persönlichkeitsprofil eines Discord-Users basierend auf seinen Nachrichten und Reaktionen. Deutsch."},
			{Role: "user", Content: prompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, p.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return p.generateProfileWithFallback(userID, name, history, given, received, model, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("AI API returned %d: %s", resp.StatusCode, string(respBody))
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return p.generateProfileWithFallback(userID, name, history, given, received, model, err)
		}
		return "", err
	}

	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", err
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("no choices in AI response")
	}

	return strings.TrimSpace(cr.Choices[0].Message.Content), nil
}

// generateProfileWithFallback retries once with the configured fallback model
// when the primary model failed transiently (timeout, rate limit, server
// error). The model guard prevents an endless fallback loop.
func (p *Profiler) generateProfileWithFallback(userID snowflake.ID, name, history string, given, received map[string]int, model string, err error) (string, error) {
	if p.cfg.FallbackModel == "" || model == p.cfg.FallbackModel {
		return "", err
	}
	slog.Warn("AI primary model failed, falling back", slog.String("from", model), slog.String("to", p.cfg.FallbackModel), slog.Any("err", err))
	return p.generateProfileWithModel(userID, name, history, given, received, p.cfg.FallbackModel)
}

func (s *Store) save() {
	tmpPath := s.filePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		slog.Warn("failed to create profiles save file", slog.Any("err", err))
		return
	}
	if err := json.NewEncoder(f).Encode(s); err != nil {
		f.Close()
		slog.Warn("failed to encode profiles", slog.Any("err", err))
		return
	}
	f.Close()
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		slog.Warn("failed to rename profiles save file", slog.Any("err", err))
	}
}

func (s *Store) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read profiles file", slog.Any("err", err))
		}
		return
	}
	if err := json.Unmarshal(data, s); err != nil {
		slog.Warn("failed to parse profiles file", slog.Any("err", err))
		s.Profiles = make(map[snowflake.ID]Profile)
		return
	}
	slog.Info("loaded profiles", slog.Int("users", len(s.Profiles)))
}
```

**Hinweis:** In `profile.go` fehlt der Import `"os"` in der obigen Liste — ergänze ihn. Außerdem: `p.store.save()` und `p.store.mu.Unlock()` gehören getauscht — speichern solange der Lock gehalten wird ist konsistent mit chatlog/`save()`-Muster. Die Reihenfolge im Code oben ist korrekt (save innerhalb des Locks).

- [ ] **Step 4: Tests laufen grün**

Run: `go test ./internal/profile/ -v`
Expected: PASS.

- [ ] **Step 5: gofmt + alle Tests**

Run: `gofmt -l internal/profile/` (leer) und `go build ./... && go test ./... && go vet ./...`
Expected: alles ok.

- [ ] **Step 6: Commit**

```bash
git add internal/profile/
git commit -m "feat: tägliche profil-pipeline aus chatlog und reaktionen"
```

---

## Task 4: Roast auf 90d + Reaktionen + Profil

**Files:**
- Modify: `roast.go:17-84` (handleRoast, buildRoastPrompt, Temp)
- Modify: `roast_test.go` (Tests für buildRoastPrompt)

- [ ] **Step 1: Failing Tests schreiben**

In `roast_test.go` anhängen:

```go
func TestBuildRoastPrompt(t *testing.T) {
	history := "kevin: heute wieder pizza"
	profileText := "Kevin ist ein Pizza-Enthusiast."
	given := map[string]int{"🍕": 3}
	received := map[string]int{"🔥": 5}

	p := buildRoastPrompt("Kevin", history, profileText, given, received)
	for _, want := range []string{"Kevin", "heute wieder pizza", "Pizza-Enthusiast", "🍕 (3)", "🔥 (5)"} {
		if !strings.Contains(p, want) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", want, p)
		}
	}
}

func TestBuildRoastPromptWithoutProfileAndReactions(t *testing.T) {
	p := buildRoastPrompt("Kevin", "", "", nil, nil)
	if !strings.Contains(p, "Kevin") {
		t.Fatalf("expected prompt to contain name, got:\n%s", p)
	}
	if strings.Contains(p, "Profil") || strings.Contains(p, "Reaktionen") {
		t.Fatalf("expected no profile/reaction sections, got:\n%s", p)
	}
}
```

- [ ] **Step 2: Tests laufen rot**

Run: `go test . -run TestBuildRoastPrompt -v`
Expected: FAIL — Signatur `buildRoastPrompt(name, history string)` existiert noch mit 2 Parametern.

- [ ] **Step 3: roast.go ändern**

In `handleRoast` (`roast.go`):

```go
	var history string
	if chatLog != nil {
		msgs := chatLog.GetMessages(target.ID, 90*24*time.Hour)
		if len(msgs) > 0 {
			var trimmed []string
			total := 0
			for i := len(msgs) - 1; i >= 0; i-- {
				if total+len(msgs[i]) > 1500 {
					break
				}
				trimmed = append([]string{msgs[i]}, trimmed...)
				total += len(msgs[i])
			}
			history = strings.Join(trimmed, "\n")
		}
	}

	var profileText string
	if profilePipeline != nil {
		if prof, ok := profilePipeline.Get(target.ID); ok {
			profileText = prof.Text
		}
	}

	var given, received map[string]int
	if reactionLog != nil {
		given, received = reactionLog.Stats(target.ID, 90*24*time.Hour)
	}

	roast, err := callAI(
		"Du schreibst witzige, bissige Einzeiler. Ein ganzer, grammatikalisch korrekter, verständlicher Satz. Kein zusammenhangloser Slang. Kurz und pointiert. Deutsch.",
		buildRoastPrompt(target.EffectiveName(), history, profileText, given, received),
	)
```

Und `buildRoastPrompt` ersetzen:

```go
func buildRoastPrompt(name, history, profileText string, given, received map[string]int) string {
	var b strings.Builder
	if profileText != "" {
		b.WriteString(fmt.Sprintf("Profil von @%s: %s\n\n", name, profileText))
	}
	if len(given) > 0 || len(received) > 0 {
		if g := formatEmojiTop(given, 5); g != "" {
			b.WriteString(fmt.Sprintf("Top-Reaktionen, die @%s vergibt: %s\n", name, g))
		}
		if r := formatEmojiTop(received, 5); r != "" {
			b.WriteString(fmt.Sprintf("Top-Reaktionen auf @%s's Nachrichten: %s\n", name, r))
		}
		b.WriteString("\n")
	}
	if history == "" {
		b.WriteString(fmt.Sprintf("Mach einen witzigen Einzeiler über jemanden namens @%s von dem wir absolut nichts wissen. Das ist der Witz — wir wissen nichts. Ein Satz. Deutsch.\n\nGutes Beispiel: \"@Kevin — selbst Siri sagt 'keine Ergebnisse' wenn sie nach deiner Persönlichkeit sucht.\"", name))
		return b.String()
	}
	b.WriteString(fmt.Sprintf("Chat von @%s:\n%s\n\nEin einziger kurzer, bissiger Satz der sich über DAS EINE lustigste Detail lustig macht. Keine Erklärung, kein Aufbau. Nur der Punch. Deutsch.\n\nGutes Beispiel: \"@Kevin — du hast 3x 'Pizza' geschrieben diese Woche. Dein Magen hat mehr Persönlichkeit als du.\"", name, history))
	return b.String()
}
```

**Hinweis:** `formatEmojiTop` existiert in `internal/profile` als unexportierte Funktion — package main kann sie nicht nutzen. Dupliziere eine kleine lokale Variante in `roast.go`:

```go
func formatEmojiTop(counts map[string]int, limit int) string {
	if len(counts) == 0 {
		return ""
	}
	type pair struct {
		emoji string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for e, c := range counts {
		pairs = append(pairs, pair{e, c})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].emoji < pairs[j].emoji
	})
	var parts []string
	for i, p := range pairs {
		if i >= limit {
			break
		}
		parts = append(parts, fmt.Sprintf("%s (%d)", p.emoji, p.count))
	}
	return strings.Join(parts, ", ")
}
```

Imports in `roast.go` ergänzen: `"sort"`.

Außerdem: Temperatur in `callAIWithModel` von `1.2` auf `1.0` ändern (`roast.go:93`):

```go
		Temperature: 1.0,
```

- [ ] **Step 4: Globals + Default-Werte**

In `commands.go` (Bereich der anderen Globals, ~Zeile 27) ergänzen:

```go
	chatLog            *chatlog.Logger
	reactionLog        *reactions.Logger
	profilePipeline    *profile.Profiler
```

Import `"github.com/levtoji/Puerierstab/internal/reactions"` und `"github.com/levtoji/Puerierstab/internal/profile"` ergänzen.

- [ ] **Step 5: Tests laufen grün**

Run: `go build ./... && go test ./... -run "TestBuildRoastPrompt|TestCallAI" -v && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add roast.go roast_test.go commands.go
git commit -m "feat: roast nutzt 90-tage-fenster, reaktionen und profil"
```

---

## Task 5: Wiring + Doku

**Files:**
- Modify: `main.go:48-50,83-97,129-141`
- Modify: `AGENTS.md`

- [ ] **Step 1: Reactions + Profiler in main.go verdrahten**

Nach dem chatlog-Block (`main.go` ~Zeile 50):

```go
	reactionLog = reactions.New(cfg.DataDir)
	stopReactionCleanup := reactionLog.StartCleanup()
	defer close(stopReactionCleanup)
```

Nach dem channelnamer-Block (nach `channelNamer = namer`):

```go
	profilePipeline = profile.New(profile.Config{
		APIKey:        cfg.AIAPIKey,
		Model:         cfg.AIModel,
		FallbackModel: cfg.AIFallbackModel,
		BaseURL:       cfg.AIBaseURL,
		Dir:           cfg.DataDir,
	}, chatLog, reactionLog)
```

Im `listeners`-Slice (`main.go` ~Zeile 84-93) ergänzen:

```go
		bot.WithEventListenerFunc(reactionLog.OnReactionAdd),
```

Nach dem Namer-Start-Block (nach `main.go` ~Zeile 135):

```go
	if profilePipeline != nil {
		slog.Info("profile pipeline active")
		stopProfile := profilePipeline.Start()
		defer close(stopProfile)
	} else {
		slog.Warn("profile pipeline disabled — AI_API_KEY missing")
	}
```

Imports ergänzen: `"github.com/levtoji/Puerierstab/internal/profile"` und `"github.com/levtoji/Puerierstab/internal/reactions"`.

- [ ] **Step 2: AGENTS.md aktualisieren**

- Architektur-Block: `internal/reactions/` und `internal/profile/` Zeilen ergänzen.
- Chatlog-Zeile: "30d retention" → "90d retention".
- Fallback-Beispiel in Env-Tabelle: `gpt-5.4-nano` → `gpt-5.4`.
- Gotcha "Polls": unverändert.

- [ ] **Step 3: Build + Tests + Vet**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: alles ok.

- [ ] **Step 4: gofmt-Kontrolle**

Run: `gofmt -l .`
Expected: keine meiner geänderten Dateien gelistet (`roast.go`, `roast_test.go`, `commands.go`, `main.go`, `internal/reactions/*`, `internal/profile/*`, `internal/chatlog/*`). Vorbestehende Unformatierungen (`main.go`, `commands.go` sind laut früherem Lauf bereits unformatiert) ignorieren — diese nicht anfassen.

- [ ] **Step 5: Railway-Env + Commit**

Railway-Env setzen:

```bash
railway variables --set "AI_FALLBACK_MODEL=gpt-5.4"
```

Commit:

```bash
git add main.go AGENTS.md
git commit -m "feat: reactions- und profil-pipeline verdrahtet"
```

---

## Nach der Implementierung

- Push nach `main`, Deploy abwarten (`railway status` → Online).
- `/backfill-chatlog` einmal ausführen (baut jetzt 90 Tage auf).
- Einen Test-Roast ausführen und Logs prüfen (`railway logs`): erwartet `[WARN] AI primary model failed, falling back` von big-pickle → gpt-5.4, dann ein Roast-Ergebnis.
- Profil-Pipeline: läuft automatisch um 3 Uhr; beim nächsten Roast danach sollte `Profil von @X:` im Prompt sein (Logs zeigen nur Fehler, keinen Prompt — Verifikation über Roast-Qualität).
