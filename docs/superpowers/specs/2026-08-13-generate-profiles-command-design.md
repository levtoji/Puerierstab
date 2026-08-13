# Manuelle Profil-Generierung (Admin) — Design

**Datum:** 2026-08-13

## Ziel

Die tägliche Profil-Pipeline (`internal/profile`, läuft automatisch um 3 Uhr) soll zusätzlich manuell per Admin-Slash-Command anstoßbar sein — analog zu `/backfill-chatlog`. Damit entfällt das Warten auf 3 Uhr zum Testen/Verifizieren.

## Änderungen

1. **Neuer Slash-Command `/generate-profiles`** (Admin-only, `DefaultMemberPermissions: Administrator`):
   - Registrierung im `registerCommandsOnReady`-Muster in `commands.go`
   - Eintrag in `knownCommands`
   - Neuer Handler `handleGenerateProfiles(event)`: `DeferCreateMessage` → `profilePipeline.RunOnce()` in Goroutine (mit Panic-Recovery wie `backfill-chatlog`) → Antwort `"Profil-Pipeline fertig: X Profile generiert/aktualisiert."`
   - Wenn `profilePipeline == nil` (kein `AI_API_KEY`): Antwort `"Profil-Pipeline ist deaktiviert (AI_API_KEY fehlt)."`

2. **`profile.RunOnce()` gibt `int` zurück** — Anzahl der erfolgreich aktualisierten Profile. Rückgabewert dient der Nutzerantwort; `TestRunOncePersists` wird erweitert.

3. **Mini-Fix:** `backfill-chatlog`-Beschreibung korrigieren — "letzte 30 Tage" → "letzte 90 Tage" (überholt durch Task 1 der Roast-Pipeline).

4. **`AGENTS.md`:** `generate-profiles` in die Command-Zeile der Architektur aufnehmen.

## Nicht im Scope

- Kein Profil-Caching/Invalidation, keine Optionen (z.B. einzelner User), keine Änderung des 3-Uhr-Plans.
- `/dump` bleibt Anzeige-only; Profil-Anzeige ist separat denkbar, aber nicht Teil dieses Features.

## Tests

- `internal/profile/profile_test.go`: `TestRunOncePersists` prüft zusätzlich den Rückgabewert von `RunOnce()`.
- Keine neuen Event-Tests (Codebase-Muster konstruiert keine Interaction-Events; die testbare Logik liegt in `RunOnce`).
