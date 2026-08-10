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
	t.Setenv(roleConfigEnv, `[{"name":"Filme","description":"Filmrollen","roles":[{"role_id":"987654321098765432","label":"Film","description":"Ping für Filme","custom_id":"film_role_toggle","style":"success"}]}]`)
	t.Setenv(legacyFilmRoleEnv, "")

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
	role := cfg.Categories[0].Roles[0]
	if role.CustomID != legacyFilmCustomID {
		t.Fatalf("unexpected custom id: %q", role.CustomID)
	}
	if role.Style != "success" {
		t.Fatalf("unexpected style: %q", role.Style)
	}
}

func TestLoadConfigFromEnvWithLegacyFilmRole(t *testing.T) {
	t.Setenv("DISGOCORD_BOT_TOKEN", "token")
	t.Setenv(roleChannelEnv, "123456789012345678")
	t.Setenv(legacyFilmRoleEnv, "987654321098765432")
	t.Setenv(roleConfigEnv, "")

	cfg, err := loadConfigFromEnv()
	if err != nil {
		t.Fatalf("loadConfigFromEnv() error = %v", err)
	}

	role := cfg.Categories[0].Roles[0]
	if role.CustomID != legacyFilmCustomID {
		t.Fatalf("expected legacy custom id %q, got %q", legacyFilmCustomID, role.CustomID)
	}
	if role.RoleID != snowflake.MustParse("987654321098765432") {
		t.Fatalf("unexpected role id: %s", role.RoleID)
	}
}

func TestBuildCategoryMessagesChunksButtonsIntoMultipleMessages(t *testing.T) {
	roles := make([]roleButton, 0, 26)
	for i := 0; i < 26; i++ {
		roles = append(roles, roleButton{
			RoleID:   snowflake.ID(1000 + i),
			Label:    "Rolle " + string(rune('A'+(i%26))),
			CustomID: "custom-" + string(rune('a'+(i%26))),
		})
	}

	messages := buildCategoryMessages(roleCategory{
		Name:        "Games",
		Description: "Selbstbedienungsrollen",
		Roles:       roles,
	})

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages for 26 buttons, got %d", len(messages))
	}
	if len(messages[0].Components) != maxRowsPerMessage {
		t.Fatalf("expected %d action rows in first message, got %d", maxRowsPerMessage, len(messages[0].Components))
	}
	if len(messages[1].Components) != 1 {
		t.Fatalf("expected 1 action row in second message, got %d", len(messages[1].Components))
	}
	if !strings.Contains(messages[1].Content, "Fortsetzung") {
		t.Fatalf("expected continuation marker in second message content, got %q", messages[1].Content)
	}
}

func TestRoleButtonComponentUsesRequestedStyle(t *testing.T) {
	button := roleButton{Label: "Film", CustomID: legacyFilmCustomID, Style: "danger"}.component()
	if button.Style != discord.ButtonStyleDanger {
		t.Fatalf("expected danger style, got %v", button.Style)
	}
}
