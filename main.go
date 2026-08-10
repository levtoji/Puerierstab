package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
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
	for _, category := range b.config.Categories {
		for _, message := range buildCategoryMessages(category) {
			if _, err := client.Rest.CreateMessage(b.config.RoleChannelID, message); err != nil {
				return err
			}
		}
	}
	return nil
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
