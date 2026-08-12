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
	aiAPIKey         string
	aiModel          string
	aiFallbackModel  string
	aiBaseURL        string
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
