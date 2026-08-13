# Design: Röst-Verbesserung + Hintergrund-Profil-Pipeline

Datum: 2026-08-13
Status: Genehmigt

## Ziel

Zwei zusammenhängende, aber unabhängige Verbesserungen rund um `/roast`:

1. **Röst-Qualität sofort verbessern**: Fallback-Modell auf `gpt-5.4` anheben (bisher `gpt-5.4-nano`, liefert lame/unverständliche Rösts), Prompt härten, Temperatur senken.
2. **3-Monats-Fenster**: Chatlog-Retention und alle Fenster von 30 auf 90 Tage.
3. **Reaktions-Erfassung**: Neue Hintergrund-Erfassung von Emoji-Reaktionen (gegeben + bekommen) pro User, Live aus Gateway-Events.
4. **Profil-Pipeline**: Täglicher Hintergrund-Job (3 Uhr), der pro User ein Persönlichkeits-Profil aus 90d Nachrichten + Reaktions-Stats per AI erstellt und in `.profiles.json` ablegt.
5. **Roast nutzt alles**: `/roast` arbeitet auf 90d Nachrichten + Reaktions-Stats + gespeichertem Profil (falls vorhanden). Unabhängig von der Profil-Pipeline: ohne Profil röstet er weiter auf Basis von Nachrichten + Reaktionen.

## Architektur

```
Gateway (MessageCreate, ReactionAdd)
   │
   ├─► internal/chatlog   (bestehend, erweitert)   → .chatlog.json   (90d)
   ├─► internal/reactions (neu)                    → .reactions.json (90d)
   │
   ├─► internal/profile   (neu, täglich 3 Uhr)     → .profiles.json
   │        liest: chatlog + reactions + AI
   │
   └─► /roast (roast.go, bestehend, erweitert)
            liest: chatlog (90d) + reactions (90d) + profile (falls da) + AI
```

## Komponenten

### 1. Fallback-Modell & Prompt-Härtung (roast.go)

- Railway-Env: `AI_FALLBACK_MODEL=gpt-5.4`
- System-Prompt in `handleRoast` härten:
  `"Du schreibst witzige, bissige Einzeiler. Ein ganzer, grammatikalisch korrekter, verständlicher Satz. Kein zusammenhangloser Slang. Kurz und pointiert. Deutsch."`
- `Temperature: 1.2` → `1.0`
- Betrifft nur den Roast; memereact/channelnamer bleiben unverändert.

### 2. 3-Monats-Fenster (internal/chatlog)

- `cleanup()`: Retention `30 * 24 * time.Hour` → `90 * 24 * time.Hour`
- `commands.go` `/backfill-chatlog`: `cutoff` 30d → 90d
- `roast.go`: `GetMessages(target.ID, 90*24*time.Hour)`
- Bestehende `GetMessages(userID, maxAge)`-Signatur braucht keine Änderung (Parameter wird vom Aufrufer bestimmt).
- Bestehender `.chatlog.json`-Datenbestand: Altlasten älter als 90d werden beim nächsten `cleanup()` entfernt; ältere gültige Daten bleiben.

### 3. Reaktions-Erfassung (neues Paket `internal/reactions`)

Datenmodell (Event-Liste pro Actor, Muster wie chatlog, Store-Typ `Logger`):

```go
type Reaction struct {
    ActorID      snowflake.ID `json:"actor_id"`       // wer reagiert
    TargetUserID snowflake.ID `json:"target_user_id"` // Author der Nachricht (0 = unbekannt)
    Emoji        string       `json:"emoji"`          // "🍕" oder Custom-Name
    Timestamp    time.Time    `json:"timestamp"`
}
```

- Store: `map[snowflake.ID][]Reaction`, Datei `.reactions.json`, Speicher-/Ladepattern und 90d-`cleanup()` wie `chatlog.Logger` (tmp-File + Rename, mutex).
- **Message→Author-Index** (in-memory, bounded): Listener auf `GuildMessageCreate` (alle Nachrichten inkl. Bots), `map[messageID]authorID`, Cap ~10.000 Einträge (älteste werden verworfen). Dient der "bekommen"-Zuordnung ohne REST-Lookup.
- **`OnReactionAdd`** (`GuildMessageReactionAdd`):
  - immer: `given` für den Actor loggen (`ActorID`, Emoji)
  - wenn MessageID im Author-Index: zusätzlich `received` für den Author loggen (`TargetUserID`, Emoji)
- **Statistik-Funktion** für Konsumenten:
  `Stats(userID, maxAge) (given map[string]int, received map[string]int)` — aggregiert über 90d.
- Emoji-String: `event.Emoji.Name` (Custom-Emoji: Name; Unicode: Glyphe).
- Nur Live-Daten ab Deploy. Historisches Nachladen von Reaktionen ist per Discord-API (per-Emoji-User-Liste je Nachricht) unverhältnismäßig teuer → **out of scope**.
- `IntentGuildMessageReactions` ist in `main.go` bereits gesetzt.

### 4. Profil-Pipeline (neues Paket `internal/profile`)

- Struktur wie `channelnamer.Namer`: `New(cfg)` mit `cfg{APIKey, BaseURL, Model, FallbackModel}`, eigener HTTP-Client (60s Timeout), Fallback bei Timeout/429/5xx (gleiche Regel wie `aiGateWithFallback`), `nextSchedule()` täglich 3 Uhr.
- `Start(stop <-chan struct{}, logger *chatlog.Logger, reactions *reactions.Logger)` — Timer-Schleife nach dem Muster von `channelnamer.Namer.Start`.
- Pro Lauf, pro User (aus `chatlog.AllUsers()` mit Nachrichten in 90d):
  1. 90d Nachrichten sammeln (Budget ~1500 Zeichen, gleiche Trimm-Logik wie Roast)
  2. Reaktions-Stats holen (`reactions.Stats(user, 90d)`)
  3. Ein AI-Call: 2-3 Sätze Persönlichkeitsprofil, Deutsch, ggf. mit Tags
  4. In `map[userID]{Text, UpdatedAt}` speichern → `.profiles.json` (tmp+Rename, mutex)
- Fehlerbehandlung: AI-Fehler → `slog.Warn`, Profil des Users nicht überschreiben, nächster User. Kein Abbruch des gesamten Laufs.
- `.profiles.json`-Format:
  ```go
  type Profile struct { Text string; UpdatedAt time.Time }
  type Store struct { Profiles map[snowflake.ID]Profile }
  ```
- Roast liest `profile.Get(userID)` nur lesend — keine Abhängigkeit Roast→Pipeline.

### 5. Roast-Prompt-Erweiterung (roast.go)

- Fenster: `90 * 24 * time.Hour`
- Trimm-Budget Nachrichten: 500 → ~1500 Zeichen
- Neue Prompt-Teile (nur wenn vorhanden):
  - Profil: `Profil von @X: <text>`
  - Reaktionen gegeben: `Top-Reaktionen, die @X vergibt: 🍕 (12), 😂 (8), …`
  - Reaktionen bekommen: `Top-Reaktionen auf @X's Nachrichten: 🔥 (15), …`
- Ohne Profil/Reaktionen: Prompt wie bisher (funktional identisch).

## Datenfluss

1. Live: `GuildMessageCreate` → chatlog (nicht-Bot) + reactions-Author-Index (alle)
2. Live: `GuildMessageReactionAdd` → reactions (given + received via Index)
3. 3 Uhr täglich: profile.Run → je User: chatlog + reactions + AI → `.profiles.json`
4. `/roast` → chatlog(90d) + reactions.Stats(90d) + profile.Get → AI → Antwort

## Fehlerbehandlung

- Chatlog/reactions: Speicherfehler nur Warn + Skip (bestehendes Muster).
- Profil-Pipeline: AI-Fehler je User → altes Profil behalten, weiterlaufen.
- Roast: AI-Fehler → bestehender Fallback-Text; kein Profil/keine Reaktionen → Prompt funktioniert ohne.

## Konfiguration

Keine neuen Env-Vars. Bestehende gelten auch für reactions/profile:
`DATA_DIR` (`.reactions.json`, `.profiles.json`), `AI_API_KEY`, `AI_BASE_URL`, `AI_MODEL`, `AI_FALLBACK_MODEL` (= `gpt-5.4`).

## Tests

- `internal/chatlog`: bestehende Tests anpassen (90d-Cleanup), neuer Test für 90d-Fenster.
- `internal/reactions`: Store save/load/cleanup, `OnReactionAdd` given/received-Zuordnung (mit/ohne Author-Index), `Stats`-Aggregation.
- `internal/profile`: `nextSchedule()`, Prompt-Building, AI-Call mit httptest-Stub (Muster `roast_test.go`/memereact), Fallback-Verhalten.
- `roast_test.go`: neue Prompt-Pfade (mit/ohne Profil, mit/ohne Reaktionen), 90d-Fenster.

## Out of scope

- Historisches Nachladen von Reaktionen (Backfill).
- Profil in memereact/channelnamer einbinden.
- Eigenes Röst-Modell (bleibt: big-pickle primär, gpt-5.4 Fallback).
- Anzeige des Profils im Chat.
