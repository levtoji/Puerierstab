package main

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

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
