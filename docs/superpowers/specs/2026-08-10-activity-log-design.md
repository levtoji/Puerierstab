# Activity-Log Design

## Ziel

Mitglieder-Aktivitäten auf dem Discord-Server protokollieren und in einen privaten Log-Channel posten. Gepostet werden: Join, Leave, Nickname-Änderungen, Rollen-Änderungen und Voice-Channel-Wechsel.

## Konfiguration

Neue optionale Env-Variable `ACTIVITY_LOG_CHANNEL_ID`:

- Gesetzt → Feature aktiv, Channel-ID ist der Zielkanal für alle Log-Einträge.
- Leer/ungesetzt → Feature deaktiviert. Keine Voice-Intents, keine Listener, kein Fehler beim Start.

Die bestehenden Pflicht-Variablen (`DISCORD_BOT_TOKEN`, `ROLE_CHANNEL_ID`, `ROLE_CATEGORIES_JSON`) bleiben unverändert.

## Projektstruktur

Feature-Packages, `main.go` als dünner Einstiegspunkt:

```
main.go                    // Config laden, Client bauen, Listener registrieren
config.go                  // bleibt; + optionale ACTIVITY_LOG_CHANNEL_ID
internal/activitylog/      // NEU
  activitylog.go           // Handler: event → Embed → Post in Log-Channel
  activitylog_test.go
internal/rolepanel/        // VERSCHOBEN aus main.go
  rolepanel.go             // publishRolePanels, getRoleChannelMessages, Index-Helfer
  interaction.go           // onComponentInteraction + Toggle-Logik
  rolepanel_test.go
```

Der Rollen-Umzug ist ein reines Verschieben ohne Verhaltensänderung.

## ActivityLog-Handler

Reine Struktur (kein Interface): kapselt REST-Client + Channel-ID.

Methoden pro Event-Typ:
- `OnGuildMemberJoin(event *events.GuildMemberJoin)`
- `OnGuildMemberLeave(event *events.GuildMemberLeave)`
- `OnGuildMemberUpdate(event *events.GuildMemberUpdate)` → Nick- und Rollen-Diff über `OldMember` vs. `Member`
- `OnGuildVoiceJoin(event *events.GuildVoiceJoin)`
- `OnGuildVoiceMove(event *events.GuildVoiceMove)`
- `OnGuildVoiceLeave(event *events.GuildVoiceLeave)`

### Diff-Logik

- **Nickname:** `OldMember.Nick` vs. `Member.Nick`; loggen nur wenn unterschiedlich.
- **Rollen:** `OldMember.RoleIDs` vs. `Member.RoleIDs`; pro Änderung ein Eintrag (Rolle hinzugefügt / entfernt). Erfasst automatisch auch Self-Service-Button-Toggles, da deren REST-Änderung als `GUILD_MEMBER_UPDATE` eintrifft.
- **Voice:** die disgo-Events `GuildVoiceJoin`/`GuildVoiceMove`/`GuildVoiceLeave` liefern bereits die Channel-Wechsel — keine eigene State-Diff nötig. Andere Voice-States (mute/deaf/streaming) werden ignoriert.

### Fehlerbehandlung

Post-Fehler → `slog.Warn`, weiterlaufen, kein Crash, kein Retry-Loop.

## Embeds

Minimales Embed je Ereignis, Farbcodierung:

| Ereignis | Farbe | Titel |
|---|---|---|
| Join | grün `0x57F287` | `**@Nick** ist beigetreten` |
| Leave | rot `0xED4245` | `**@Nick** hat den Server verlassen` |
| Nick | grau `0x95A5A6` | `**@Nick**: @Old → @New` |
| Rolle + | blau `0x5865F2` | `**@Nick** + @Rolle` |
| Rolle − | orange `0xE67E22` | `**@Nick** − @Rolle` |
| Voice Join | hellblau `0x3498DB` | `**@Nick** → #Channel` |
| Voice Leave | hellblau `0x3498DB` | `**@Nick** ← #Channel` |
| Voice Move | hellblau `0x3498DB` | `**@Nick**: #A → #B` |

Kein Avatar, keine Thumbnails, keine User-ID-Footer — bewusst minimal. Nickname = `Member.User.Username` bzw. `Member.Nick` (fallback auf Username).

## Intents

- `gateway.IntentGuildMembers` — bereits gesetzt (für Rollen-Toggles nötig)
- `gateway.IntentGuildVoiceStates` — neu, nur wenn `ACTIVITY_LOG_CHANNEL_ID` gesetzt

## Datenfluss

1. `main.go` lädt Config.
2. Ist Log-Channel gesetzt: `activitylog.New(client, channelID)` erzeugen und als Listener registrieren; Voice-Intent aktivieren.
3. disgo dispatcht Events an die Handler.
4. Handler baut Embed, `client.Rest.CreateMessage(logChannelID, embed)`.
5. Fehler → `slog.Warn`, weiter.

## Teststrategie

- Embed-Builder je Ereignistyp (Titel + Farbe)
- Nick-Diff (gleich/unterschiedlich/leer)
- Rollen-Diff (add/remove/mehrere)
- Voice: Join/Move/Leave-Erkennung über die disgo-Events
- Aktivierung nur bei gesetzter Env-Variable (Config-Test)
