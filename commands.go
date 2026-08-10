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
)

var registerOnce sync.Once

func registerCommandsOnReady(event *events.GuildReady) {
	registerOnce.Do(func() {
		appID := event.Client().ApplicationID
		guildID := event.GuildID

		if existing, err := event.Client().Rest.GetGuildCommands(appID, guildID, false); err == nil {
			for _, cmd := range existing {
				if cmd.Name() == "clear-chat" {
					if err := event.Client().Rest.DeleteGuildCommand(appID, guildID, cmd.ID()); err != nil {
						slog.Warn("failed to delete stale slash command", slog.String("id", cmd.ID().String()), slog.Any("err", err))
					}
				}
			}
		}

		perms := discord.Permissions(discord.PermissionAdministrator)
		cmd, err := event.Client().Rest.CreateGuildCommand(appID, guildID,
			discord.SlashCommandCreate{
				Name:                     "clear-chat",
				Description:              "Löscht alle Nachrichten in diesem Kanal",
				DefaultMemberPermissions: omit.NewPtr(perms),
			},
		)
		if err != nil {
			slog.Error("failed to register clear-chat command", slog.Any("err", err))
			return
		}
		slog.Info("registered slash command", slog.String("name", cmd.Name()), slog.String("id", cmd.ID().String()), slog.String("guild_id", guildID.String()))
	})
}

func handleSlashCommand(event *events.ApplicationCommandInteractionCreate) {
	data := event.SlashCommandInteractionData()
	if data.CommandName() != "clear-chat" {
		return
	}

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
