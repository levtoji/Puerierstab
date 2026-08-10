# Poll & Icebreaker Design

## Ziel

Zwei Features, die Interaktion im Server fördern:
- `/poll` — Mehrfachauswahl-Umfragen mit Buttons
- `/question` — Zufällige Fragen zur Diskussion

## Struktur

```
internal/poll/
  poll.go           — Store, Handler (Slash-Command + Component-Interaction)
  poll_test.go
internal/icebreaker/
  icebreaker.go     — Fragen laden, Random Pick, Slash-Command-Handler
  icebreaker_test.go
commands.go         — registerCommandsOnReady (alle drei Commands), handleSlashCommand (Dispatch)
```

## /poll

- **Parameter:** `question` (String, required), `options` (String, required)
- **Options-Format:** Komma-getrennt, z.B. `"Dune, Barbie, John Wick"` → max 5 nach Split
- **Voting:** Mehrfachauswahl per Button-Toggle. CustomID: `poll:{id}:{idx}`
- **State:** `Store` (map[string]*Poll + RWMutex), in-memory, verloren bei Restart
- **Embed:** Zeigt Frage + Optionen mit Vote-Counts, wird bei jedem Klick via `UpdateMessage` aktualisiert
- **Kein Close:** Polls bleiben offen, bis der Bot neustartet oder die Nachricht gelöscht wird
- **Handler:** eigener Component-Interaction-Listener in main.go registriert

## /question

- **Parameter:** keine
- **Konfiguration:** `ICE_BREAKER_QUESTIONS_JSON='["Frage 1", "Frage 2", ...]'`
- **Default:** Built-in-Liste (deutsch), verwendet wenn Env-Variable fehlt/leer/ungültig
- **Ausgabe:** Embed (blau 0x5865F2), nur die Frage
- **Package:** `internal/icebreaker/`

## Konfiguration

| Variable | Pflicht | Beschreibung |
|---|---|---|
| `ICE_BREAKER_QUESTIONS_JSON` | Nein | JSON-Array von Fragen-Strings. Falls leer: Built-in-Defaults |

## Teststrategie

- **Poll:** Store CRUD, Options-Parsing, Vote-Toggle, Embed-Building
- **Icebreaker:** Fragen laden (Env+Disk), Random Pick, Embed-Building
