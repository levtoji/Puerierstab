package rolepanel

import (
	"log/slog"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	"github.com/levtoji/Puerierstab/internal/config"
)

func (b *RoleBot) OnReady(event *events.Ready) {
	b.syncOnce.Do(func() {
		go func() {
			if err := b.publishRolePanels(event.Client()); err != nil {
				slog.Error("publishing role panels failed", slog.Any("err", err))
			}
		}()
	})
}

func (b *RoleBot) OnComponentInteraction(event *events.ComponentInteractionCreate) {
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
	if config.MemberHasRole(member.RoleIDs, role.RoleID) {
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

func userIDFromInteraction(event *events.ComponentInteractionCreate) snowflake.ID {
	if user := event.User(); user.ID != 0 {
		return user.ID
	}
	if member := event.Member(); member != nil {
		return member.User.ID
	}
	return 0
}

func fmtRoleAdded(role config.RoleButton) string {
	return "Die Rolle „" + role.Label + "“ wurde dir gegeben."
}

func fmtRoleRemoved(role config.RoleButton) string {
	return "Die Rolle „" + role.Label + "“ wurde dir entfernt."
}

func ephemeralMessage(content string) discord.MessageCreate {
	return discord.NewMessageCreate().
		WithContent(content).
		WithEphemeral(true)
}
