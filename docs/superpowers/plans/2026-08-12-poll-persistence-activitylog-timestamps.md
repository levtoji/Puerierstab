# Poll Persistence & Activity Log Timestamps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add JSON file persistence to polls (survive restarts, expire after 24h) and timestamps to activity log embeds.

**Architecture:** `poll.Store` writes `.polls.json` on every mutation via atomic temp-file+rename. `Poll` gains `CreatedAt` and `IsExpired()`. Activity log embeds get a `Footer` with `time.Now().Format("02.01.2006 15:04:05")`.

**Tech Stack:** Go 1.24, stdlib `encoding/json`, `os`, `time`. No new dependencies.

---

### Task 1: Activity log — add timestamp footer to all embeds

**Files:**
- Modify: `internal/activitylog/activitylog.go:32-86`
- Modify: `internal/activitylog/activitylog_test.go`

- [ ] **Step 1: Add `timestampFooter` helper**

In `internal/activitylog/activitylog.go`, after the `memberName` helper (line 92), add:

```go
func timestampFooter() *discord.EmbedFooter {
	return &discord.EmbedFooter{Text: time.Now().Format("02.01.2006 15:04:05")}
}
```

Add `"time"` to imports.

- [ ] **Step 2: Add Footer to all 8 embed builders**

Update each embed builder to include `.Footer: timestampFooter()`:

- `joinEmbed` (line 32): add `Footer: timestampFooter()` 
- `leaveEmbed` (line 39): add `Footer: timestampFooter()`
- `nickChangeEmbed` (line 46): add `Footer: timestampFooter()`
- `roleAddedEmbed` (line 53): add `Footer: timestampFooter()`
- `roleRemovedEmbed` (line 60): add `Footer: timestampFooter()`
- `voiceJoinEmbed` (line 67): add `Footer: timestampFooter()`
- `voiceLeaveEmbed` (line 74): add `Footer: timestampFooter()`
- `voiceMoveEmbed` (line 81): add `Footer: timestampFooter()`

Example for `joinEmbed`:
```go
func joinEmbed(member discord.Member) discord.Embed {
	return discord.Embed{
		Title:  fmt.Sprintf("**%s** ist dem Server beigetreten", memberName(member)),
		Color:  0x57F287,
		Footer: timestampFooter(),
	}
}
```

Apply the same pattern to all 8.

- [ ] **Step 3: Write test for timestamp footer presence**

In `internal/activitylog/activitylog_test.go`, add:

```go
func TestEmbedHasTimestampFooter(t *testing.T) {
	member := discord.Member{
		User: discord.User{
			ID:            snowflake.MustParse("111111111111111111"),
			Username:      "TestUser",
			GlobalName:    &[]string{"TestUser"}[0],
		},
	}

	embeds := []discord.Embed{
		joinEmbed(member),
		leaveEmbed(member),
		nickChangeEmbed(member, "old", "new"),
		roleAddedEmbed(member, "Admin"),
		roleRemovedEmbed(member, "Admin"),
		voiceJoinEmbed(member, "General"),
		voiceLeaveEmbed(member, "General"),
		voiceMoveEmbed(member, "A", "B"),
	}

	for i, embed := range embeds {
		if embed.Footer == nil {
			t.Fatalf("embed %d: expected Footer, got nil", i)
		}
		if embed.Footer.Text == "" {
			t.Fatalf("embed %d: expected non-empty Footer text", i)
		}
		_, err := time.Parse("02.01.2006 15:04:05", embed.Footer.Text)
		if err != nil {
			t.Fatalf("embed %d: Footer text %q is not valid timestamp: %v", i, embed.Footer.Text, err)
		}
	}
}
```

Add `"time"` and `"github.com/disgoorg/snowflake/v2"` to test imports.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/activitylog/ -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/activitylog/activitylog.go internal/activitylog/activitylog_test.go
git commit -m "feat: add timestamp footer to activity log embeds"
```

---

### Task 2: Poll — add CreatedAt field and IsExpired check

**Files:**
- Modify: `internal/poll/poll.go:50-114`
- Modify: `internal/poll/poll.go:185-208` (HandleComponent)

- [ ] **Step 1: Add `CreatedAt` to `Poll` struct and set it in `Create`**

In `internal/poll/poll.go`, add `"time"` to imports (after `"sync"`).

Add `CreatedAt time.Time` to `Poll` struct:
```go
type Poll struct {
	ID        string
	Question  string
	Options   []string
	Votes     map[int]map[snowflake.ID]struct{}
	CreatorID snowflake.ID
	CreatedAt time.Time
	mu        sync.RWMutex
}
```

In `Store.Create`, add after `CreatorID: creatorID,`:
```go
CreatedAt: time.Now(),
```

- [ ] **Step 2: Add `IsExpired` method and update `Embed`**

Add `IsExpired` to `Poll`:
```go
func (p *Poll) IsExpired() bool {
	return time.Since(p.CreatedAt) > 24*time.Hour
}
```

Update `Poll.Embed` to show "(abgelaufen)" in title when expired:
```go
func (p *Poll) Embed() discord.Embed {
	p.mu.RLock()
	defer p.mu.RUnlock()
	title := p.Question
	if p.IsExpired() {
		title += " (abgelaufen)"
	}
	fields := make([]discord.EmbedField, len(p.Options))
	for i, opt := range p.Options {
		count := len(p.Votes[i])
		fields[i] = discord.EmbedField{
			Name:  opt,
			Value: fmt.Sprintf("%d Stimmen", count),
		}
	}
	return discord.Embed{
		Title:  title,
		Color:  0x5865F2,
		Fields: fields,
	}
}
```

- [ ] **Step 3: Add expiry rejection to `HandleComponent`**

In `HandleComponent`, after `poll, found := s.Get(pollID)` (line 196), before the `userID` line, add:

```go
if poll.IsExpired() {
	_ = event.CreateMessage(discord.NewMessageCreate().
		WithContent("Diese Umfrage ist abgelaufen.").
		WithEphemeral(true))
	return
}
```

- [ ] **Step 4: Write tests for CreatedAt, IsExpired, and Embed title**

In `internal/poll/poll_test.go`, add:

```go
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
```

Add `"time"` to test imports and `"strings"` if not already present.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/poll/ -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/poll/poll.go internal/poll/poll_test.go
git commit -m "feat: add CreatedAt and expiry to polls"
```

---

### Task 3: Poll — JSON file persistence (save/load/.polls.json)

**Files:**
- Modify: `internal/poll/poll.go:16-44` (Store struct, NewStore, Create)
- Modify: `internal/poll/poll.go:185-208` (HandleComponent)
- Modify: `internal/poll/poll_test.go`
- Modify: `.gitignore`

- [ ] **Step 1: Add `filePath` field to `Store`, update `NewStore`**

Update `Store` struct:
```go
type Store struct {
	polls    map[string]*Poll
	filePath string
	mu       sync.RWMutex
}
```

Update `NewStore`:
```go
func NewStore() *Store {
	s := &Store{
		polls:    make(map[string]*Poll),
		filePath: ".polls.json",
	}
	s.load()
	return s
}
```

Add imports: `"encoding/json"`, `"os"`.

- [ ] **Step 2: Implement `Store.save()` and `Store.load()`**

After `NewStore`, add:

```go
func (s *Store) save() {
	tmpPath := s.filePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		slog.Warn("failed to create poll save file", slog.Any("err", err))
		return
	}
	if err := json.NewEncoder(f).Encode(s.polls); err != nil {
		f.Close()
		slog.Warn("failed to encode polls", slog.Any("err", err))
		return
	}
	f.Close()
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		slog.Warn("failed to rename poll save file", slog.Any("err", err))
	}
}

func (s *Store) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("failed to read poll save file", slog.Any("err", err))
		}
		return
	}
	var polls map[string]*Poll
	if err := json.Unmarshal(data, &polls); err != nil {
		slog.Warn("failed to parse poll save file", slog.Any("err", err))
		return
	}
	now := time.Now()
	for id, p := range polls {
		if now.Sub(p.CreatedAt) > 24*time.Hour {
			continue
		}
		if p.Votes == nil {
			p.Votes = make(map[int]map[snowflake.ID]struct{})
		}
		s.polls[id] = p
	}
	slog.Info("loaded polls", slog.Int("count", len(s.polls)))
}
```

- [ ] **Step 3: Call `save()` after `Create`**

In `Store.Create`, add `defer s.save()` after acquiring the lock. Update the method:

```go
func (s *Store) Create(question string, options []string, creatorID snowflake.ID) *Poll {
	p := &Poll{
		ID:        randomID(),
		Question:  question,
		Options:   options,
		Votes:     make(map[int]map[snowflake.ID]struct{}),
		CreatorID: creatorID,
		CreatedAt: time.Now(),
	}
	s.mu.Lock()
	s.polls[p.ID] = p
	s.save()
	s.mu.Unlock()
	return p
}
```

- [ ] **Step 4: Add `Store.ToggleVote` method that saves**

Add new method to `Store`:

```go
func (s *Store) ToggleVote(pollID string, optionIdx int, userID snowflake.ID) bool {
	s.mu.RLock()
	p, ok := s.polls[pollID]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	p.ToggleVote(optionIdx, userID)
	s.mu.Lock()
	s.save()
	s.mu.Unlock()
	return true
}
```

- [ ] **Step 5: Update `HandleComponent` to use `Store.ToggleVote`**

In `HandleComponent`, replace `poll.ToggleVote(optionIdx, userID)` with:

```go
s.ToggleVote(pollID, optionIdx, userID)
```

- [ ] **Step 6: Add `.polls.json` to `.gitignore`**

In `.gitignore`, after the existing `.role_messages.json` line (line 38), add:

```
.polls.json
```

- [ ] **Step 7: Write persistence tests**

In `internal/poll/poll_test.go`, add:

```go
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
	filepath := filepath.Join(dir, ".polls.json")

	store := &Store{
		polls:    make(map[string]*Poll),
		filePath: filepath,
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
		filePath: filepath,
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
	filepath := filepath.Join(dir, ".polls.json")

	store := &Store{
		polls:    make(map[string]*Poll),
		filePath: filepath,
	}
	p := store.Create("Test?", []string{"A", "B"}, snowflake.ID(1))
	store.ToggleVote(p.ID, 0, snowflake.ID(99))

	store2 := &Store{
		polls:    make(map[string]*Poll),
		filePath: filepath,
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
```

Add `"path/filepath"` to test imports.

- [ ] **Step 8: Run tests**

```bash
go test ./internal/poll/ -v
```

Expected: all tests PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/poll/poll.go internal/poll/poll_test.go .gitignore
git commit -m "feat: add JSON file persistence to poll store"
```

---

### Task 4: Verify build, vet, and full test suite

- [ ] **Step 1: Run full verification**

```bash
go build ./... && go test ./... && go vet ./...
```

Expected: all PASS, no errors.

- [ ] **Step 2: Commit any remaining changes**

Only if there are leftover modifications.

---

### Task 5: Update AGENTS.md

**Files:**
- Modify: `AGENTS.md`

- [ ] **Step 1: Update poll persistence note**

In `AGENTS.md`, change line 60 from:
```
- **Polls are in-memory**: `poll.Store` is a Go map, lost on restart. No persistence.
```
to:
```
- **Polls**: persisted to `.polls.json` (autosaved on create/vote, expires after 24h).
```

- [ ] **Step 2: Commit**

```bash
git add AGENTS.md
git commit -m "docs: update AGENTS.md for poll persistence"
```
