# Puerierstab

Discord-Bot in Go mit `disgo`, der Rollen per Button als Selbstbedienung vergibt oder entzieht.

## Konfiguration

Der Bot liest seinen Token aus `DISGOCORD_BOT_TOKEN` oder `DISCORD_BOT_TOKEN`.

Zusätzlich wird ein Ziel-Channel für die Rollenübersicht benötigt:

```bash
export ROLE_CHANNEL_ID=123456789012345678
```

### Rollen per JSON konfigurieren

Die bevorzugte Konfiguration läuft über `ROLE_CATEGORIES_JSON`. Jede Kategorie wird beim Start in den konfigurierten Channel geschrieben und enthält die zugehörigen Buttons.

```bash
export ROLE_CATEGORIES_JSON='[
  {
    "name": "Filme",
    "description": "Rollen für Filmabende",
    "roles": [
      {
        "role_id": "987654321098765432",
        "label": "Filmrolle",
        "description": "Benachrichtigungen zu Filmen",
        "custom_id": "film_role_toggle",
        "style": "primary"
      }
    ]
  },
  {
    "name": "Games",
    "description": "Optionale Spiele-Rollen",
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

### Kompatibilitätsmodus für die Filmrolle

Wenn nur eine einzelne Filmrolle gebraucht wird, reicht alternativ:

```bash
export FILM_ROLE_ID=987654321098765432
```

Dann erstellt der Bot automatisch eine Kategorie `Filme` mit einem Button auf der Custom-ID `film_role_toggle`.

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
