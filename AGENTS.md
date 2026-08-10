# Puerierstab — Discord Bot

Go 1.24.13, `github.com/disgoorg/disgo` v0.19.6. Railway auto-deploy from `main`.

## Commands

```bash
go build ./...          # build
go test ./...           # all tests
go test ./internal/X/ -run TestFoo -v  # single package/test
go vet ./...            # static analysis
```

No CI, no linter config. Always run `go build ./... && go test ./... && go vet ./...` before committing.

## Architecture

```
main.go              — thin entrypoint (env → config → client → listener wiring)
commands.go          — slash command registration + dispatch (all 3 commands)
internal/config/     — Config types, env loading, RoleCategory/RoleButton, BuildCategoryMessages
internal/rolepanel/  — RoleBot, role button interaction, channel-scan panel publishing
internal/activitylog/— join/leave/nick/role/voice event logging with embeddings
internal/poll/       — multi-choice polls (Store, slash+component handlers)
internal/icebreaker/ — random question from env JSON (or built-in defaults)
```

Key pattern: feature packages under `internal/`, `main.go` only wires listeners.

## Env vars

| Variable | Required | Notes |
|---|---|---|
| `DISCORD_BOT_TOKEN` | yes | |
| `ROLE_CHANNEL_ID` | yes | snowflake |
| `ROLE_CATEGORIES_JSON` | yes | JSON array of category objects |
| `ACTIVITY_LOG_CHANNEL_ID` | no | snowflake, feature disabled when empty |
| `ICE_BREAKER_QUESTIONS_JSON` | no | JSON string array, falls back to built-in 10 defaults |

## disgo patterns & gotchas

- **Listeners**: `bot.WithEventListenerFunc(func(e *events.SomeEvent))` — generic, type-safe
- **REST/Caches**: `client.Rest.Xxx(...)` and `client.Caches.Xxx(...)` — **fields**, not methods
- **Intents**: `gateway.WithIntents` uses `.Add()` — appends, safe to call multiple times
- **Commands**: register as **Guild commands** via `SetGuildCommands` (bulk overwrite) in `GuildReady` handler. Guild commands need no `applications.commands` scope. Use `sync.Once` to prevent re-registration on reconnect.
- **`discord.Message.ID.Time()`** — snowflake ID contains the timestamp, no separate `CreatedAt` method
- **Bulk delete**: `BulkDeleteMessages` requires ≥2 message IDs, all <14 days old. Fall back to single `DeleteMessage` for 1 message or older messages.
- **Event dedup**: use `event.SequenceNumber()` — duplicate seq = replay, skip it
- **Role names**: try `client.Caches.Role(guildID, roleID)` first, then REST `GetRoles(guildID)` as fallback. Cache can miss.
- **Channel names**: try `client.Caches.GuildVoiceChannel(channelID)` → `Name()`, then REST `GetChannel(channelID)` type-asserted to `discord.GuildVoiceChannel`.
- **Member cache unreliable**: do NOT use `event.OldMember` from `GuildMemberUpdate` for role/name diffs — the disgo member cache often returns zero values. Maintain a local `map[snowflake.ID][]snowflake.ID` role tracker.

## Testing

Tests are internal (`package foo`, not `package foo_test`), giving access to unexported symbols. Standard Go testing — no fixtures, no mocks needed.
