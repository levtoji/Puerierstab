package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/snowflake/v2"
)

type roleBot struct {
	config         config
	roleByCustomID map[string]roleButton
	syncOnce       sync.Once
}

func newRoleBot(cfg config) *roleBot {
	roleByCustomID := make(map[string]roleButton, totalRoleCount(cfg.Categories))
	for _, category := range cfg.Categories {
		for _, role := range category.Roles {
			roleByCustomID[role.CustomID] = role
		}
	}

	return &roleBot{
		config:         cfg,
		roleByCustomID: roleByCustomID,
	}
}

func main() {
	if err := run(); err != nil {
		slog.Error("bot stopped", slog.Any("err", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfigFromEnv()
	if err != nil {
		return err
	}

	app := newRoleBot(cfg)

	client, err := disgo.New(cfg.Token,
		bot.WithGatewayConfigOpts(
			gateway.WithIntents(
				gateway.IntentGuilds,
				gateway.IntentGuildMembers,
			),
		),
		bot.WithEventListenerFunc(app.onReady),
		bot.WithEventListenerFunc(app.onComponentInteraction),
	)
	if err != nil {
		return err
	}
	defer client.Close(context.Background())

	if err = client.OpenGateway(context.Background()); err != nil {
		return err
	}

	slog.Info("role bot is running", slog.String("version", disgo.Version), slog.String("role_channel_id", cfg.RoleChannelID.String()))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	return nil
}

func (b *roleBot) onReady(event *events.Ready) {
	b.syncOnce.Do(func() {
		go func() {
			if err := b.publishRolePanels(event.Client()); err != nil {
				slog.Error("publishing role panels failed", slog.Any("err", err))
			}
		}()
	})
}

func (b *roleBot) publishRolePanels(client *bot.Client) error {
	existingMessages, err := b.getRoleChannelMessages(client)
	if err != nil {
		return err
	}

	existingByCustomIDs := indexMessagesByCustomIDs(existingMessages)

	for _, category := range b.config.Categories {
		for messageIdx, messageCreate := range buildCategoryMessages(category) {
			wantCustomIDs := messageCreateCustomIDs(messageCreate)

			// Reuse an existing panel whose buttons carry the exact same custom IDs.
			messageID, ok := existingByCustomIDs[wantCustomIDs.key()]
			if ok {
				if _, err := client.Rest.UpdateMessage(b.config.RoleChannelID, messageID, messageUpdateFromCreate(messageCreate)); err != nil {
					slog.Warn("failed to update role panel, creating new", slog.String("category", category.Name), slog.Int("message_index", messageIdx), slog.Any("err", err))
				} else {
					delete(existingByCustomIDs, wantCustomIDs.key())
					continue
				}
			}

			// No reusable panel found (or update failed): create a fresh one.
			created, err := client.Rest.CreateMessage(b.config.RoleChannelID, messageCreate)
			if err != nil {
				return err
			}
			slog.Info("created new role panel", slog.String("category", category.Name), slog.Int("message_index", messageIdx), slog.String("message_id", created.ID.String()))
		}
	}

	// Delete leftover managed panels that are no longer configured (e.g. removed categories).
	for _, messageID := range existingByCustomIDs {
		if err := client.Rest.DeleteMessage(b.config.RoleChannelID, messageID); err != nil {
			slog.Warn("failed to delete stale role panel", slog.String("message_id", messageID.String()), slog.Any("err", err))
		}
	}
	return nil
}

func messageUpdateFromCreate(messageCreate discord.MessageCreate) discord.MessageUpdate {
	return discord.NewMessageUpdate().
		WithEmbeds(messageCreate.Embeds...).
		WithComponents(messageCreate.Components...)
}

func (b *roleBot) getRoleChannelMessages(client *bot.Client) ([]discord.Message, error) {
	var (
		messages []discord.Message
		before   snowflake.ID
	)
	for {
		page, err := client.Rest.GetMessages(b.config.RoleChannelID, 0, before, 0, 100)
		if err != nil {
			return nil, err
		}
		for _, message := range page {
			if message.Author.Bot && b.isManagedRolePanelMessage(message) {
				messages = append(messages, message)
			}
		}
		if len(page) < 100 {
			return messages, nil
		}
		before = page[len(page)-1].ID
	}
}

func (b *roleBot) isManagedRolePanelMessage(message discord.Message) bool {
	for customID := range messageCustomIDs(message) {
		if _, managed := b.roleByCustomID[customID]; managed {
			return true
		}
	}
	return false
}

type customIDSet map[string]struct{}

func (s customIDSet) key() string {
	keys := make([]string, 0, len(s))
	for customID := range s {
		keys = append(keys, customID)
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x00")
}

func messageCustomIDs(message discord.Message) customIDSet {
	return collectButtonCustomIDs(message.Components)
}

func messageCreateCustomIDs(message discord.MessageCreate) customIDSet {
	return collectButtonCustomIDs(message.Components)
}

func collectButtonCustomIDs(components []discord.LayoutComponent) customIDSet {
	customIDs := make(customIDSet)
	for _, layout := range components {
		row, ok := layout.(discord.ActionRowComponent)
		if !ok {
			continue
		}
		for _, component := range row.Components {
			button, ok := component.(discord.ButtonComponent)
			if !ok {
				continue
			}
			customIDs[button.CustomID] = struct{}{}
		}
	}
	return customIDs
}

func indexMessagesByCustomIDs(messages []discord.Message) map[string]snowflake.ID {
	index := make(map[string]snowflake.ID, len(messages))
	for _, message := range messages {
		index[messageCustomIDs(message).key()] = message.ID
	}
	return index
}

func (b *roleBot) onComponentInteraction(event *events.ComponentInteractionCreate) {
	if event.Data.Type() != discord.ComponentTypeButton {
		return
	}

	button := event.ButtonInteractionData()
	role, ok := b.roleByCustomID[button.CustomID()]
	if !ok {
		return
	}

	guildID := event.GuildID()
	if guildID == nil {
		_ = event.CreateMessage(ephemeralMessage("Diese Rollenverwaltung funktioniert nur auf einem Server."))
		return
	}

	member := event.Member()
	if member == nil {
		_ = event.CreateMessage(ephemeralMessage("Deine Server-Mitgliedschaft konnte nicht geladen werden."))
		return
	}

	userID := userIDFromInteraction(event)
	if userID == 0 {
		_ = event.CreateMessage(ephemeralMessage("Dein Benutzer konnte nicht geladen werden."))
		return
	}

	var (
		err     error
		message string
	)
	if memberHasRole(member.RoleIDs, role.RoleID) {
		err = event.Client().Rest.RemoveMemberRole(*guildID, userID, role.RoleID)
		message = fmtRoleRemoved(role)
	} else {
		err = event.Client().Rest.AddMemberRole(*guildID, userID, role.RoleID)
		message = fmtRoleAdded(role)
	}

	if err != nil {
		slog.Error("role toggle failed", slog.Any("err", err), slog.String("custom_id", role.CustomID), slog.String("role_id", role.RoleID.String()))
		_ = event.CreateMessage(ephemeralMessage("Die Rolle konnte gerade nicht aktualisiert werden. Bitte versuche es erneut."))
		return
	}

	_ = event.CreateMessage(ephemeralMessage(message))
}

func totalRoleCount(categories []roleCategory) int {
	count := 0
	for _, category := range categories {
		count += len(category.Roles)
	}
	return count
}

func userIDFromInteraction(event *events.ComponentInteractionCreate) snowflake.ID {
	if user := event.User(); user.ID != 0 {
		return user.ID
	}
	if member := event.Member(); member != nil {
		return member.User.ID
	}
	return 0
}

func fmtRoleAdded(role roleButton) string {
	return "Die Rolle „" + role.Label + "“ wurde dir gegeben."
}

func fmtRoleRemoved(role roleButton) string {
	return "Die Rolle „" + role.Label + "“ wurde dir entfernt."
}

func ephemeralMessage(content string) discord.MessageCreate {
	return discord.NewMessageCreate().
		WithContent(content).
		WithEphemeral(true)
}
