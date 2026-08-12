# Röst-Meister & ASCII-Art Reactions

## 1. Röst-Meister `/roast @user`

### Command

`/roast @user` — nutzt den OpenCode-Zen-API-Endpoint wie der Channel-Namer.

### Chat-Logger (`internal/chatlog/`)

Horcht auf `GuildMessageCreate`, speichert pro User:
- User-ID
- Timestamp
- Nachrichtentext

Lädt/speichert via `.chatlog.json` (JSON, atomic temp+rename, gleiches Pattern wie `.polls.json`).

Cleanup beim Laden und stündlich: Nachrichten >30 Tage werden entfernt. Keine persistente Speicherung von privaten Daten nötig.

### Roast-Prompt

```
Roaste [@Username] auf lustige, freundschaftliche Art. Max 2 Sätze, Deutsch.
Verwende diese Infos über die Person (falls vorhanden):
[letzten 30 Tage Chat-History, Zeilenweise, max ~500 Zeichen]
```

### Flow

1. `/roast @user` → `chatlog.GetMessages(userID, 30 days)`
2. Prompt bauen → AI API aufrufen
3. Antwort als öffentliche Message in den Channel posten
4. Errors: ephemeral "Rösten fehlgeschlagen, versuch's später."

---

## 2. ASCII-Art Reactions (`internal/asciireact/`)

### Listener

Horcht auf `GuildMessageCreate`. Scannt Nachrichtentext (lowercase) auf Keywords. Bei Treffer postet Bot ASCII-Art als Antwort.

### Keywords (ca. 10-15)

Jedes Keyword triggert eine feste ASCII-Art-Antwort. Verspielt, deutsche Internetkultur:

| Keyword | ASCII |
|---------|-------|
| `hunde`, `hund`, `dog`, `doggo` | `ʕ•ᴥ•ʔ` |
| `katze`, `cat`, `miau` | `ᓚᘏᗢ` |
| `party`, `feier`, `feiern` | ASCII-Disco-Kugel |
| `montag` | trauriger ASCII-Montag |
| `pizza`, `margherita` | ASCII-Pizzastück |
| `kaffee` | ASCII-Kaffeetasse |
| `schlafen`, `müde`, `bett` | ASCII-Schlafender |
| `code`, `programmier`, `bug` | ASCII-Code-Monitor |
| `regen`, `wetter` | ASCII-Regenschirm |
| `discord`, `nitro` | ASCII-Discord-Logo (simpel) |
| `gaming`, `zock`, `spiel` | ASCII-Controller |
| `musik` | ASCII-Kopfhörer |
| `banane`, `banan` | ASCII-Banane |

Jedes Keyword hat eine Antwort. Bot ignoriert eigene Nachrichten und Bots. Rate-Limit: max 1 Reaktion alle 10 Sekunden (global, cooldown).

---

## Testing

- `internal/chatlog/chatlog_test.go` — save, load, cleanup (>30d), GetMessages
- `internal/asciireact/asciireact_test.go` — keyword matching, cooldown
- `commands.go` — roast command registration + handler

## `.gitignore`

`.chatlog.json` hinzufügen.
