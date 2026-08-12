# Röst-Meister & ASCII-Art Reactions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `/roast @user` command (AI-generated roast from chat history) + ASCII-art keyword reactions in chat.

**Architecture:** New `internal/chatlog/` stores recent messages per user to `.chatlog.json` (30d TTL, hourly cleanup). New `internal/asciireact/` listens for keywords and posts ASCII art in code blocks. Both wire as `GuildMessageCreate` listeners in `main.go`. Roast command uses same AI API pattern as channelnamer (OpenCode Zen).

**Tech Stack:** Go 1.24, stdlib `encoding/json`, `net/http`, disgo events. No new dependencies.

---

### Task 1: Chat Logger (`internal/chatlog/`)

**Files:**
- Create: `internal/chatlog/chatlog.go`
- Create: `internal/chatlog/chatlog_test.go`
- Modify: `main.go` (wire chatlog + content intent)
- Modify: `.gitignore` (add `.chatlog.json`)
- Modify: `AGENTS.md` (add chatlog package)

Implement `chatlog.Logger` with:
- `Log(userID, content)` — appends entry, saves
- `GetMessages(userID, maxAge)` — returns messages within time window
- `cleanup()` — removes entries older than 30 days, calls save
- `save()` / `load()` — JSON file persistence (atomic tmp+rename)
- `StartCleanup() chan struct{}` — hourly cleanup goroutine with stop channel

In `main.go`:
- Create `chatLog = chatlog.New()` 
- Add `GatewayIntentMessageContent` to intents
- Register `chatLog.OnMessageCreate` as listener
- Start cleanup: `stopChatCleanup := chatLog.StartCleanup(); defer close(stopChatCleanup)`

In `commands.go`:
- Add `chatLog *chatlog.Logger` to package-level vars

Tests (4): `TestLogAndGet`, `TestGetMessagesMaxAge`, `TestSaveAndLoad`, `TestCleanup`

---

### Task 2: ASCII-Art Reactions (`internal/asciireact/`)

**Files:**
- Create: `internal/asciireact/asciireact.go`
- Create: `internal/asciireact/asciireact_test.go`
- Modify: `main.go` (wire reactor)
- Modify: `AGENTS.md` (add asciireact)

Implement `asciireact.Reactor` with:
- `reactions` map: 13 keyword groups → ASCII art strings
- `matchKeyword(content) (string, bool)` — lowercase contains match
- `OnMessageCreate(event)` — matches keyword, honors 10s global cooldown, posts art in code block

Keywords: hunde/hund/doggo/dog → ʕ•ᴥ•ʔ, katze/cat/miau → ᓚᘏᗢ, party/feier/feiern → party bunny, montag → coffee mug, pizza/pizz → pizza slice, kaffee → coffee cup, schlafen/müde/bett → sleeping cat, code/programmier/bug → monitor, regen/wetter → umbrella, discord/nitro → wumpus, gaming/zock/spiel → controller, musik → headphones, banane/banan → banana

In `main.go`: register `reactor.OnMessageCreate` as listener

Tests (2): `TestMatchKeyword`, `TestNoMatch`

---

### Task 3: Röst-Meister `/roast` command

**Files:**
- Modify: `commands.go` (add command registration + handler)

Add `/roast @user` slash command (admin-only like clear-chat):
- Register in `registerCommandsOnReady`
- Add to `knownCommands`
- Add `handleRoast` handler:
  1. Defer ephemeral
  2. Get messages from `chatLog.GetMessages(targetUserID, 30*24*time.Hour)`
  3. Build prompt with chat history (last 500 chars)
  4. Call AI API (same endpoint/config as channelnamer)
  5. Edit deferred response with `roast` as public content (or fallback text on error)
- Use `cfg.AIAPIKey`, `cfg.AIModel`, `cfg.AIBaseURL` — pass from `main.go` via new package-level var `aiConfig`

In `commands.go`: add `"bytes"`, `"net/http"`, `"io"`, `chatlog` import

In `main.go`: set `aiConfig = channelnamer.Config{...}` package-level var

---

### Task 4: Verify, build, vet, deploy

```bash
go build ./... && go test ./... && go vet ./...
```

All 22+ tests must pass. Then `git push` for Railway deploy.
