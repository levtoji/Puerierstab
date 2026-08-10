package main

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"

	"github.com/levtoji/Puerierstab/internal/icebreaker"
	"github.com/levtoji/Puerierstab/internal/poll"
)

var (
	registerOnce       sync.Once
	pollStore          *poll.Store
	icebreakerHandler  *icebreaker.Handler
)

var knownCommands = []string{"clear-chat", "poll", "question"}

func registerCommandsOnReady(event *events.GuildReady) {
	registerOnce.Do(func() {
		appID := event.Client().ApplicationID
		guildID := event.GuildID

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
		}

		if _, err := event.Client().Rest.SetGuildCommands(appID, guildID, cmds); err != nil {
			slog.Error("failed to register guild commands", slog.Any("err", err))
		} else {
			slog.Info("registered guild commands", slog.Int("count", len(cmds)), slog.String("guild_id", guildID.String()))
		}
	})
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
	}
}

func handleClearChat(event *events.ApplicationCommandInteractionCreate) {
	if err := event.DeferCreateMessage(true); err != nil {
		slog.Warn("failed to defer interaction", slog.Any("err", err))
		return
	}

	channelID := event.Channel().ID()
	var totalDeleted int

	for totalDeleted < 1000 {
		messages, err := event.Client().Rest.GetMessages(channelID, 0, 0, 0, 100)
		if err != nil {
			respond(event, fmt.Sprintf("Fehler beim Laden der Nachrichten: %v", err))
			return
		}
		if len(messages) == 0 {
			break
		}

		var ids []snowflake.ID
		for _, msg := range messages {
			ids = append(ids, msg.ID)
		}

		if err := event.Client().Rest.BulkDeleteMessages(channelID, ids); err != nil {
			respond(event, fmt.Sprintf("%d Nachrichten gelöscht (Limit erreicht)", totalDeleted))
			return
		}
		totalDeleted += len(ids)
		time.Sleep(200 * time.Millisecond)
	}

	if totalDeleted == 0 {
		respond(event, "Keine Nachrichten zum Löschen gefunden")
	} else {
		respond(event, fmt.Sprintf("%d Nachrichten gelöscht", totalDeleted))
	}
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
