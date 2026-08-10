package rolepanel

import (
	"sort"
	"strings"
	"sync"

	"log/slog"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/levtoji/Puerierstab/internal/config"
)

type RoleBot struct {
	cfg            config.Config
	roleByCustomID map[string]config.RoleButton
	syncOnce       sync.Once
}

func NewRoleBot(cfg config.Config) *RoleBot {
	roleByCustomID := make(map[string]config.RoleButton, totalRoleCount(cfg.Categories))
	for _, category := range cfg.Categories {
		for _, role := range category.Roles {
			roleByCustomID[role.CustomID] = role
		}
	}

	return &RoleBot{
		cfg:            cfg,
		roleByCustomID: roleByCustomID,
	}
}

func (b *RoleBot) publishRolePanels(client *bot.Client) error {
	existingMessages, err := b.getRoleChannelMessages(client)
	if err != nil {
		return err
	}

	existingByCustomIDs := indexMessagesByCustomIDs(existingMessages)

	for _, category := range b.cfg.Categories {
		for messageIdx, messageCreate := range config.BuildCategoryMessages(category) {
			wantCustomIDs := messageCreateCustomIDs(messageCreate)

			messageID, ok := existingByCustomIDs[wantCustomIDs.key()]
			if ok {
				if _, err := client.Rest.UpdateMessage(b.cfg.RoleChannelID, messageID, messageUpdateFromCreate(messageCreate)); err != nil {
					slog.Warn("failed to update role panel, creating new", slog.String("category", category.Name), slog.Int("message_index", messageIdx), slog.Any("err", err))
				} else {
					delete(existingByCustomIDs, wantCustomIDs.key())
					continue
				}
			}

			created, err := client.Rest.CreateMessage(b.cfg.RoleChannelID, messageCreate)
			if err != nil {
				return err
			}
			slog.Info("created new role panel", slog.String("category", category.Name), slog.Int("message_index", messageIdx), slog.String("message_id", created.ID.String()))
		}
	}

	for _, messageID := range existingByCustomIDs {
		if err := client.Rest.DeleteMessage(b.cfg.RoleChannelID, messageID); err != nil {
			slog.Warn("failed to delete stale role panel", slog.String("message_id", messageID.String()), slog.Any("err", err))
		}
	}
	return nil
}

func (b *RoleBot) getRoleChannelMessages(client *bot.Client) ([]discord.Message, error) {
	var (
		messages []discord.Message
		before   snowflake.ID
	)
	for {
		page, err := client.Rest.GetMessages(b.cfg.RoleChannelID, 0, before, 0, 100)
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

func (b *RoleBot) isManagedRolePanelMessage(message discord.Message) bool {
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

func messageUpdateFromCreate(messageCreate discord.MessageCreate) discord.MessageUpdate {
	return discord.NewMessageUpdate().
		WithEmbeds(messageCreate.Embeds...).
		WithComponents(messageCreate.Components...)
}

func totalRoleCount(categories []config.RoleCategory) int {
	count := 0
	for _, category := range categories {
		count += len(category.Roles)
	}
	return count
}
