# Puerierstab

Discord-Bot in Go mit `disgo`, der Rollen per Button als Selbstbedienung vergibt oder entzieht.

## Konfiguration

Der Bot benötigt folgende Umgebungsvariablen:

### Erforderlich

- **`DISCORD_BOT_TOKEN`** — Der Bot-Token von Discord
- **`ROLE_CHANNEL_ID`** — Die Discord-Channel-ID, in der die Rollenbuttons gepostet werden
- **`ROLE_CATEGORIES_JSON`** — JSON-Konfiguration für die Rollenkategorien und Buttons

### Konfigurationsbeispiel

```bash
export DISCORD_BOT_TOKEN="your_bot_token"
export ROLE_CHANNEL_ID=123456789012345678
export ROLE_CATEGORIES_JSON='[
  {
    "name": "Filme",
    "description": "Rollen für Filmabende",
    "emoji": "🎬",
    "roles": [
      {
        "role_id": "987654321098765432",
        "label": "Filmschauer",
        "description": "Benachrichtigungen zu Filmen und Serien",
        "custom_id": "film_role_toggle",
        "style": "primary"
      }
    ]
  },
  {
    "name": "Games",
    "description": "Optionale Spiele-Rollen",
    "emoji": "🎮",
    "roles": [
      {
        "role_id": "111111111111111111",
        "label": "Minecraft",
        "description": "Pings für Minecraft",
        "custom_id": "games_minecraft_toggle",
        "style": "success"
      },
      {
        "role_id": "222222222222222222",
        "label": "Valorant",
        "description": "Pings für Valorant",
        "custom_id": "games_valorant_toggle",
        "style": "secondary"
      }
    ]
  }
]'
```

## Starten

```bash
go run .
```

## Docker

Es ist ein Alpine-basiertes Multi-Stage-Dockerfile enthalten:

```bash
docker build -t puerierstab .
docker run --rm \
  -e DISCORD_BOT_TOKEN=... \
  -e ROLE_CHANNEL_ID=123456789012345678 \
  -e ROLE_CATEGORIES_JSON='[...]' \
  puerierstab
```
