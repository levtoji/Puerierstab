# Poll Persistence & Activity Log Timestamps

## 1. Activity Log: Date & Time Footer

Add a timestamp footer to every activity log embed.

**Change in `internal/activitylog/activitylog.go`**:
- New helper `timestampFooter() string` → `time.Now().Format("02.01.2006 15:04:05")`
- Every embed builder (`joinEmbed`, `leaveEmbed`, `nickChangeEmbed`, `roleAddedEmbed`, `roleRemovedEmbed`, `voiceJoinEmbed`, `voiceLeaveEmbed`, `voiceMoveEmbed`) sets `.Footer` to `&discord.EmbedFooter{Text: timestampFooter()}`.

## 2. Poll Persistence

### File

`.polls.json` in the working directory. Already covered by `.gitignore` (line `Puerierstab` is just the binary, but `.role_messages.json` is already there — add `.polls.json`).

### Store changes (`internal/poll/poll.go`)

**New fields on `Store`**:
```go
type Store struct {
    polls    map[string]*Poll
    filePath string
    mu       sync.RWMutex
}
```

**New field on `Poll`**:
```go
type Poll struct {
    ID        string
    Question  string
    Options   []string
    Votes     map[int]map[snowflake.ID]struct{}
    CreatorID snowflake.ID
    CreatedAt time.Time    // NEW
    mu        sync.RWMutex
}
```

**Save/Load**:
- `Store.save()` — writes entire map as JSON to `.polls.json` (temp file + rename for atomicity). Called after `Create` and `ToggleVote`.
- `Store.load()` — reads `.polls.json`, unmarshals, filters out polls where `time.Since(p.CreatedAt) > 24h`.
- `NewStore()` — returns `*Store` from `load()` or fresh `*Store` on error. `filePath` defaults to `.polls.json` if empty.

**Expiry check**:
- `Poll.IsExpired() bool` — `time.Since(p.CreatedAt) > 24h`.
- `Store.HandleComponent` — if poll is expired, respond with ephemeral "Diese Umfrage ist abgelaufen." instead of toggling.
- `Poll.Embed()` — if expired, embeds show "(abgelaufen)" in title.

### Key behaviors

- Save errors are `slog.Warn`, bot keeps running.
- Load errors are `slog.Warn`, bot starts with empty store.
- Expired polls are still in the store until login/restart cleanup. Expiry is checked on every interaction.

### Existing `.role_messages.json`

Unrelated; this file is written by `rolepanel` to track which messages it posted. No changes needed there.

## `.gitignore`

Add `.polls.json` (alongside existing `.role_messages.json`).

## Testing

- `internal/poll/poll_test.go` — add tests for `load`, `save` (with temp file), `IsExpired`, expired interaction rejection, `CreatedAt` included on create.
- `internal/activitylog/activitylog_test.go` — test that embeds contain footer with timestamp format.
