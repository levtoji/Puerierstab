package main

import (
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestLoadConfigFromEnvWithJSONConfig(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "token")
	t.Setenv(roleChannelEnv, "123456789012345678")
	t.Setenv(roleConfigEnv, `[{"name":"Filme","description":"Filmrollen","emoji":"🎬","roles":[{"role_id":"987654321098765432","label":"Film","description":"Ping für Filme","custom_id":"film_role_toggle","style":"success"}]}]`)

	cfg, err := loadConfigFromEnv()
	if err != nil {
		t.Fatalf("loadConfigFromEnv() error = %v", err)
	}

	if cfg.Token != "token" {
		t.Fatalf("expected token to be loaded from DISCORD_BOT_TOKEN, got %q", cfg.Token)
	}
	if cfg.RoleChannelID != snowflake.MustParse("123456789012345678") {
		t.Fatalf("unexpected role channel id: %s", cfg.RoleChannelID)
	}
	if len(cfg.Categories) != 1 || len(cfg.Categories[0].Roles) != 1 {
		t.Fatalf("unexpected category layout: %+v", cfg.Categories)
	}

	category := cfg.Categories[0]
	if category.Emoji != "🎬" {
		t.Fatalf("expected emoji 🎬, got %q", category.Emoji)
	}

	role := category.Roles[0]
	if role.CustomID != "film_role_toggle" {
		t.Fatalf("unexpected custom id: %q", role.CustomID)
	}
	if role.Style != "success" {
		t.Fatalf("unexpected style: %q", role.Style)
	}
}

func TestLoadConfigFromEnvMissingRoleCategoriesJSON(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "token")
	t.Setenv(roleChannelEnv, "123456789012345678")
	t.Setenv(roleConfigEnv, "")

	_, err := loadConfigFromEnv()
	if err == nil {
		t.Fatalf("expected error when ROLE_CATEGORIES_JSON is missing, got nil")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected error message to contain 'missing', got %q", err.Error())
	}
}

func TestLoadConfigFromEnvMissingToken(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "")
	t.Setenv(roleChannelEnv, "123456789012345678")
	t.Setenv(roleConfigEnv, `[{"name":"Test","emoji":"🎮","roles":[]}]`)

	_, err := loadConfigFromEnv()
	if err == nil {
		t.Fatalf("expected error when DISCORD_BOT_TOKEN is missing, got nil")
	}
	if !strings.Contains(err.Error(), "DISCORD_BOT_TOKEN") {
		t.Fatalf("expected error message to mention DISCORD_BOT_TOKEN, got %q", err.Error())
	}
}

func TestBuildCategoryMessagesChunksButtonsIntoMultipleMessages(t *testing.T) {
	roles := make([]roleButton, 0, 26)
	for i := 0; i < 26; i++ {
		roles = append(roles, roleButton{
			RoleID:      snowflake.ID(1000 + i),
			Label:       "Rolle " + string(rune('A'+(i%26))),
			Description: "Desc " + string(rune('a'+(i%26))),
			CustomID:    "custom-" + string(rune('a'+(i%26))),
		})
	}

	messages := buildCategoryMessages(roleCategory{
		Name:        "Games",
		Description: "Selbstbedienungsrollen",
		Emoji:       "🎮",
		Roles:       roles,
	})

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages for 26 buttons, got %d", len(messages))
	}

	// Check first message
	if len(messages[0].Components) != maxRowsPerMessage {
		t.Fatalf("expected %d action rows in first message, got %d", maxRowsPerMessage, len(messages[0].Components))
	}
	if len(messages[0].Embeds) == 0 {
		t.Fatalf("expected embed in first message, got none")
	}
	embed := messages[0].Embeds[0]
	if embed.Title != "🎮 Games" {
		t.Fatalf("expected title '🎮 Games', got %q", embed.Title)
	}

	// Check second message
	if len(messages[1].Components) != 1 {
		t.Fatalf("expected 1 action row in second message, got %d", len(messages[1].Components))
	}
}

func TestRoleButtonComponentUsesRequestedStyle(t *testing.T) {
	tests := []struct {
		style        string
		expectedEnum discord.ButtonStyle
	}{
		{"primary", discord.ButtonStylePrimary},
		{"danger", discord.ButtonStyleDanger},
		{"success", discord.ButtonStyleSuccess},
		{"secondary", discord.ButtonStyleSecondary},
		{"unknown", discord.ButtonStylePrimary}, // defaults to primary
	}

	for _, tt := range tests {
		t.Run(tt.style, func(t *testing.T) {
			button := roleButton{
				Label:    "Test",
				CustomID: "test",
				Style:    tt.style,
			}.component()
			if button.Style != tt.expectedEnum {
				t.Fatalf("expected style %v, got %v", tt.expectedEnum, button.Style)
			}
		})
	}
}
