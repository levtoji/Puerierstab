package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"

	"github.com/levtoji/Puerierstab/internal/channelnamer"
	"github.com/levtoji/Puerierstab/internal/chatlog"
	"github.com/levtoji/Puerierstab/internal/icebreaker"
	"github.com/levtoji/Puerierstab/internal/memereact"
	"github.com/levtoji/Puerierstab/internal/poll"
)

var (
	registeredGuilds   = map[snowflake.ID]struct{}{}
	registeredGuildsMu sync.Mutex
	pollStore          *poll.Store
	icebreakerHandler  *icebreaker.Handler
	channelNamer       *channelnamer.Namer
	chatLog            *chatlog.Logger
	aiAPIKey           string
	aiModel            string
	aiBaseURL          string
	memeReactor        *memereact.Reactor
	startTime          = time.Now()
)

var knownCommands = []string{"clear-chat", "poll", "question", "rename-channels", "roast", "dashboard", "dump"}

func registerCommandsOnReady(event *events.GuildReady) {
	appID := event.Client().ApplicationID
	guildID := event.GuildID

	registeredGuildsMu.Lock()
	if _, ok := registeredGuilds[guildID]; ok {
		registeredGuildsMu.Unlock()
		return
	}
	registeredGuilds[guildID] = struct{}{}
	registeredGuildsMu.Unlock()

	if existing, err := event.Client().Rest.GetGlobalCommands(appID, false); err == nil {
		for _, cmd := range existing {
			for _, name := range knownCommands {
				if cmd.Name() == name {
					if err := event.Client().Rest.DeleteGlobalCommand(appID, cmd.ID()); err != nil {
						slog.Warn("failed to delete stale global command", slog.String("id", cmd.ID().String()), slog.Any("err", err))
					}
					break
				}
			}
		}
	}

	adminPerms := discord.Permissions(discord.PermissionAdministrator)
	cmds := []discord.ApplicationCommandCreate{
		discord.SlashCommandCreate{
				Name:                     "clear-chat",
				Description:              "Löscht alle Nachrichten in diesem Kanal",
				DefaultMemberPermissions: omit.NewPtr(adminPerms),
			},
			discord.SlashCommandCreate{
				Name:        "poll",
				Description: "Erstellt eine Umfrage mit Mehrfachauswahl",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionString{
						Name:        "question",
						Description: "Die Frage der Umfrage",
						Required:    true,
					},
					discord.ApplicationCommandOptionString{
						Name:        "options",
						Description: "Antwortmöglichkeiten, Komma-getrennt (2-5)",
						Required:    true,
					},
				},
			},
			discord.SlashCommandCreate{
				Name:        "question",
				Description: "Postet eine zufällige Diskussionsfrage",
			},
		discord.SlashCommandCreate{
			Name:                     "rename-channels",
			Description:              "Benennt alle Voice-Channel sofort um (Admin)",
			DefaultMemberPermissions: omit.NewPtr(adminPerms),
		},
		discord.SlashCommandCreate{
			Name:                     "roast",
			Description:              "Röstet einen User basierend auf seinem Chat-Verlauf (Admin)",
			DefaultMemberPermissions: omit.NewPtr(adminPerms),
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionUser{
					Name:        "user",
					Description: "Das Opfer",
					Required:    true,
				},
			},
		},
		discord.SlashCommandCreate{
			Name:                     "dashboard",
			Description:              "Zeigt Bot-Status und Statistiken (Admin)",
			DefaultMemberPermissions: omit.NewPtr(adminPerms),
		},
		discord.SlashCommandCreate{
			Name:                     "dump",
			Description:              "Zeigt gespeicherte Daten als Tabelle (Admin)",
			DefaultMemberPermissions: omit.NewPtr(adminPerms),
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionString{
					Name:        "target",
					Description: "Was anzeigen: chatlog, polls, memes",
					Required:    true,
					Choices: []discord.ApplicationCommandOptionChoiceString{
						{Name: "Chatlog", Value: "chatlog"},
						{Name: "Polls", Value: "polls"},
						{Name: "Meme-Log", Value: "memes"},
					},
				},
			},
		},
	}

	if _, err := event.Client().Rest.SetGuildCommands(appID, guildID, cmds); err != nil {
		slog.Error("failed to register guild commands", slog.Any("err", err))
	} else {
		slog.Info("registered guild commands", slog.Int("count", len(cmds)), slog.String("guild_id", guildID.String()))
	}
}

func handleSlashCommand(event *events.ApplicationCommandInteractionCreate) {
	data := event.SlashCommandInteractionData()
	switch data.CommandName() {
	case "clear-chat":
		handleClearChat(event)
	case "poll":
		pollStore.HandleCreate(event)
	case "question":
		icebreakerHandler.OnCommand(event)
	case "rename-channels":
		handleRenameChannels(event)
	case "roast":
		handleRoast(event)
	case "dashboard":
		handleDashboard(event)
	case "dump":
		handleDump(event)
	}
}

func handleClearChat(event *events.ApplicationCommandInteractionCreate) {
	if err := event.DeferCreateMessage(true); err != nil {
		slog.Warn("failed to defer interaction", slog.Any("err", err))
		return
	}

	channelID := event.Channel().ID()
	cutoff := time.Now().Add(-14 * 24 * time.Hour)
	var totalDeleted int
	var before snowflake.ID

	for totalDeleted < 1000 {
		messages, err := event.Client().Rest.GetMessages(channelID, 0, before, 0, 100)
		if err != nil {
			respond(event, fmt.Sprintf("Fehler: %v", err))
			return
		}
		if len(messages) == 0 {
			break
		}

		var ids []snowflake.ID
		for _, msg := range messages {
			if msg.ID.Time().After(cutoff) {
				ids = append(ids, msg.ID)
			}
		}

		if len(ids) == 0 {
			for _, msg := range messages {
				if err := event.Client().Rest.DeleteMessage(channelID, msg.ID); err != nil {
					respond(event, fmt.Sprintf("%d Nachrichten gelöscht (weitere zu alt)", totalDeleted))
					return
				}
				totalDeleted++
			}
			time.Sleep(200 * time.Millisecond)
		} else if len(ids) == 1 {
			if err := event.Client().Rest.DeleteMessage(channelID, ids[0]); err != nil {
				respond(event, fmt.Sprintf("%d Nachrichten gelöscht (weitere zu alt)", totalDeleted))
				return
			}
			totalDeleted++
			time.Sleep(200 * time.Millisecond)
		} else {
			if err := event.Client().Rest.BulkDeleteMessages(channelID, ids); err != nil {
				respond(event, fmt.Sprintf("%d Nachrichten gelöscht (Rest zu alt für Bulk-Delete)", totalDeleted))
				return
			}
			totalDeleted += len(ids)
			time.Sleep(200 * time.Millisecond)
		}

		before = messages[len(messages)-1].ID
	}

	if totalDeleted == 0 {
		respond(event, "Keine löschbaren Nachrichten gefunden")
	} else {
		respond(event, fmt.Sprintf("%d Nachrichten gelöscht", totalDeleted))
	}
}

func handleRenameChannels(event *events.ApplicationCommandInteractionCreate) {
	if channelNamer == nil {
		_ = event.CreateMessage(discord.NewMessageCreate().
			WithContent("Channel-Umbenennung ist nicht konfiguriert (RENAME_CHANNEL_IDS fehlt).").
			WithEphemeral(true))
		return
	}

	_ = event.CreateMessage(discord.NewMessageCreate().
		WithContent("Voice-Channel werden umbenannt...").
		WithEphemeral(true))

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("rename-channels panic", slog.Any("panic", r))
			}
		}()
		channelNamer.RenameAll(event.Client())
	}()
}

func respond(event *events.ApplicationCommandInteractionCreate, content string) {
	if _, err := event.Client().Rest.UpdateInteractionResponse(
		event.ApplicationID(),
		event.Token(),
		discord.MessageUpdate{Content: &content},
	); err != nil {
		slog.Warn("failed to edit interaction response", slog.Any("err", err))
	}
}

func handleRoast(event *events.ApplicationCommandInteractionCreate) {
	if err := event.DeferCreateMessage(false); err != nil {
		return
	}

	data := event.SlashCommandInteractionData()
	target := data.User("user")

	var history string
	if chatLog != nil {
		msgs := chatLog.GetMessages(target.ID, 30*24*time.Hour)
		if len(msgs) > 0 {
			var trimmed []string
			total := 0
			for i := len(msgs) - 1; i >= 0; i-- {
				if total+len(msgs[i]) > 500 {
					break
				}
				trimmed = append([]string{msgs[i]}, trimmed...)
				total += len(msgs[i])
			}
			history = strings.Join(trimmed, "\n")
		}
	}

	prompt := fmt.Sprintf("Roaste @%s auf lustige, freundschaftliche Art. Max 2 kurze Sätze, auf Deutsch. Sei kreativ aber nicht gemein.", target.EffectiveName())
	if history != "" {
		prompt += fmt.Sprintf("\n\nVerwende diese Infos über die Person:\n%s\n\nWICHTIG: Erwähne nicht alles davon, sondern nimm 1-2 der lustigsten Details.", history)
	}

	roast, err := callAI(prompt)
	if err != nil {
		slog.Warn("roast AI failed", slog.Any("err", err))
		roast = fmt.Sprintf("%s ist heute leider zu langweilig für einen guten Röst.", target.Mention())
	}

	if _, err := event.Client().Rest.UpdateInteractionResponse(
		event.ApplicationID(),
		event.Token(),
		discord.MessageUpdate{Content: &roast},
	); err != nil {
		slog.Warn("failed to send roast", slog.Any("err", err))
	}
}

type chatRequest struct {
	Model       string        `json:"model"`
	Temperature float64       `json:"temperature"`
	Messages    []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

func callAI(prompt string) (string, error) {
	reqBody := chatRequest{
		Model:       aiModel,
		Temperature: 0.9,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, aiBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+aiAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("AI API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", err
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("no choices in AI response")
	}

	return cr.Choices[0].Message.Content, nil
}

func handleDashboard(event *events.ApplicationCommandInteractionCreate) {
	if err := event.DeferCreateMessage(true); err != nil {
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	ramMB := m.HeapAlloc / 1024 / 1024

	var pollCount int
	pollLines := "Keine"
	if pollStore != nil {
		pollCount = pollStore.Count()
		pollLines = fmt.Sprintf("%d", pollCount)
	}

	chatUsers := 0
	if chatLog != nil {
		chatUsers = chatLog.UserCount()
	}

	memeYes, memeNo := 0, 0
	if memeReactor != nil && memeReactor.MemeLog() != nil {
		for _, entry := range memeReactor.MemeLog().Recent() {
			if entry.Decision == "YES" {
				memeYes++
			} else {
				memeNo++
			}
		}
	}

	embed := discord.Embed{
		Title: "📊 Puerierstab Status",
		Color: 0x5865F2,
		Fields: []discord.EmbedField{
			{Name: "Laufzeit", Value: fmt.Sprintf("%s", formatDuration(time.Since(startTime))), Inline: boolPtr(true)},
			{Name: "RAM", Value: fmt.Sprintf("%d MB", ramMB), Inline: boolPtr(true)},
			{Name: "Polls", Value: pollLines, Inline: boolPtr(true)},
			{Name: "Chatlog (User)", Value: fmt.Sprintf("%d", chatUsers), Inline: boolPtr(true)},
			{Name: "Meme-Gate (YES/NO)", Value: fmt.Sprintf("%d / %d", memeYes, memeNo), Inline: boolPtr(true)},
			{Name: "ASCII-Keywords", Value: "13", Inline: boolPtr(true)},
		},
	}

	_, _ = event.Client().Rest.UpdateInteractionResponse(
		event.ApplicationID(),
		event.Token(),
		discord.NewMessageUpdate().WithEmbeds(embed),
	)
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func handleDump(event *events.ApplicationCommandInteractionCreate) {
	if err := event.DeferCreateMessage(true); err != nil {
		return
	}

	data := event.SlashCommandInteractionData()
	target := data.String("target")

	switch target {
	case "chatlog":
		handleDumpChatlog(event)
	case "polls":
		handleDumpPolls(event)
	case "memes":
		handleDumpMemes(event)
	}
}

func handleDumpChatlog(event *events.ApplicationCommandInteractionCreate) {
	if chatLog == nil {
		_, _ = event.Client().Rest.UpdateInteractionResponse(
			event.ApplicationID(), event.Token(),
			discord.MessageUpdate{Content: strPtr("Chatlog nicht aktiv.")},
		)
		return
	}

	users := chatLog.AllUsers()
	if len(users) == 0 {
		_, _ = event.Client().Rest.UpdateInteractionResponse(
			event.ApplicationID(), event.Token(),
			discord.MessageUpdate{Content: strPtr("Keine User im Chatlog.")},
		)
		return
	}

	var fields []discord.EmbedField
	for _, userID := range users {
		msgs := chatLog.GetMessages(userID, 30*24*time.Hour)
		if len(msgs) > 0 && len(msgs) > 10 {
			msgs = msgs[len(msgs)-10:]
		}
		var lines []string
		for _, msg := range msgs {
			if len(msg) > 60 {
				msg = msg[:60] + "..."
			}
			lines = append(lines, msg)
		}
		if len(lines) > 0 {
			fields = append(fields, discord.EmbedField{
				Name:   userID.String(),
				Value:  strings.Join(lines, "\n"),
				Inline: boolPtr(false),
			})
		}
		if len(fields) >= 10 {
			break
		}
	}

	embed := discord.Embed{
		Title:  "📁 Chatlog",
		Fields: fields,
		Color:  0x5865F2,
	}
	_, _ = event.Client().Rest.UpdateInteractionResponse(
		event.ApplicationID(), event.Token(),
		discord.NewMessageUpdate().WithEmbeds(embed),
	)
}

func handleDumpPolls(event *events.ApplicationCommandInteractionCreate) {
	if pollStore == nil {
		respond(event, "Polls nicht aktiv.")
		return
	}

	allPolls := pollStore.All()
	if len(allPolls) == 0 {
		respond(event, "Keine aktiven Umfragen.")
		return
	}

	for _, p := range allPolls {
		embed := p.Embed()
		embed.Footer = &discord.EmbedFooter{Text: "ID: " + p.ID}
		_, _ = event.Client().Rest.CreateMessage(event.Channel().ID(), discord.NewMessageCreate().WithEmbeds(embed))
	}

	respond(event, fmt.Sprintf("%d Umfragen ausgegeben.", len(allPolls)))
}

func handleDumpMemes(event *events.ApplicationCommandInteractionCreate) {
	if memeReactor == nil || memeReactor.MemeLog() == nil {
		respond(event, "Meme-Reactor nicht aktiv.")
		return
	}

	entries := memeReactor.MemeLog().Recent()
	if len(entries) == 0 {
		respond(event, "Noch keine Meme-Entscheidungen.")
		return
	}

	var lines []string
	for i, entry := range entries {
		if i >= 20 {
			break
		}
		line := fmt.Sprintf("`%s` | %s", entry.Timestamp.Format("02.01. 15:04"), entry.Decision)
		if entry.Decision == "YES" && entry.Query != "" {
			line += fmt.Sprintf(" | %s", entry.Query)
		}
		lines = append(lines, line)
	}

	embed := discord.Embed{
		Title:       "🤖 Meme-Gate Log",
		Description: strings.Join(lines, "\n"),
		Color:       0x9B59B6,
	}
	_, _ = event.Client().Rest.UpdateInteractionResponse(
		event.ApplicationID(), event.Token(),
		discord.NewMessageUpdate().WithEmbeds(embed),
	)
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool   { return &b }
