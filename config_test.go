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

func TestNormalizeCategoriesValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   []roleCategoryJSON
		wantErr bool
		errMsg  string
	}{
		{
			"empty categories",
			[]roleCategoryJSON{},
			true,
			"at least one role category is required",
		},
		{
			"category without name",
			[]roleCategoryJSON{
				{Description: "Test", Roles: []roleButtonJSON{{Label: "R1", RoleID: jsonSnowflake(1)}}},
			},
			true,
			"name is required",
		},
		{
			"category without roles",
			[]roleCategoryJSON{
				{Name: "Gaming", Description: "Test", Roles: []roleButtonJSON{}},
			},
			true,
			"at least one role is required",
		},
		{
			"role without label",
			[]roleCategoryJSON{
				{Name: "Gaming", Roles: []roleButtonJSON{{RoleID: jsonSnowflake(1)}}},
			},
			true,
			"label is required",
		},
		{
			"duplicate custom_id",
			[]roleCategoryJSON{
				{
					Name: "Gaming",
					Roles: []roleButtonJSON{
						{Label: "R1", RoleID: jsonSnowflake(1), CustomID: "same_id"},
						{Label: "R2", RoleID: jsonSnowflake(2), CustomID: "same_id"},
					},
				},
			},
			true,
			"duplicate custom_id",
		},
		{
			"valid single role",
			[]roleCategoryJSON{
				{Name: "Gaming", Emoji: "🎮", Roles: []roleButtonJSON{{Label: "R1", RoleID: jsonSnowflake(1)}}},
			},
			false,
			"",
		},
		{
			"valid multiple categories",
			[]roleCategoryJSON{
				{Name: "Cat1", Emoji: "🎮", Roles: []roleButtonJSON{{Label: "R1", RoleID: jsonSnowflake(1)}}},
				{Name: "Cat2", Emoji: "🎬", Roles: []roleButtonJSON{{Label: "R2", RoleID: jsonSnowflake(2)}}},
			},
			false,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := normalizeCategories(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result == nil {
					t.Fatalf("expected categories, got nil")
				}
			}
		})
	}
}

func TestNormalizeCategoriesAutoGenerateCustomID(t *testing.T) {
	input := []roleCategoryJSON{
		{
			Name:  "Gaming",
			Emoji: "🎮",
			Roles: []roleButtonJSON{
				{Label: "R1", RoleID: jsonSnowflake(111)},
				{Label: "R2", RoleID: jsonSnowflake(222), CustomID: "custom_r2"},
			},
		},
	}

	result, err := normalizeCategories(input)
	if err != nil {
		t.Fatalf("normalizeCategories() error = %v", err)
	}

	roles := result[0].Roles
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}

	// First role should have auto-generated custom ID
	if !strings.Contains(roles[0].CustomID, "111") {
		t.Fatalf("expected auto-generated custom_id to contain role ID 111, got %q", roles[0].CustomID)
	}

	// Second role should use provided custom ID
	if roles[1].CustomID != "custom_r2" {
		t.Fatalf("expected custom_r2, got %q", roles[1].CustomID)
	}
}
