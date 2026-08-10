package main

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestNewRoleBotInitializesRoleMap(t *testing.T) {
	cfg := config{
		Token:         "test_token",
		RoleChannelID: snowflake.MustParse("123456789012345678"),
		Categories: []roleCategory{
			{
				Name:  "Gaming",
				Emoji: "🎮",
				Roles: []roleButton{
					{
						RoleID:   snowflake.MustParse("111111111111111111"),
						Label:    "Minecraft",
						CustomID: "minecraft_toggle",
					},
					{
						RoleID:   snowflake.MustParse("222222222222222222"),
						Label:    "Valorant",
						CustomID: "valorant_toggle",
					},
				},
			},
			{
				Name:  "Films",
				Emoji: "🎬",
				Roles: []roleButton{
					{
						RoleID:   snowflake.MustParse("333333333333333333"),
						Label:    "Filmschauer",
						CustomID: "film_toggle",
					},
				},
			},
		},
	}

	bot := newRoleBot(cfg)

	if len(bot.roleByCustomID) != 3 {
		t.Fatalf("expected 3 roles, got %d", len(bot.roleByCustomID))
	}

	// Verify each role is accessible
	tests := map[string]string{
		"minecraft_toggle": "Minecraft",
		"valorant_toggle":  "Valorant",
		"film_toggle":      "Filmschauer",
	}

	for customID, expectedLabel := range tests {
		role, ok := bot.roleByCustomID[customID]
		if !ok {
			t.Fatalf("expected role with customID %q to exist", customID)
		}
		if role.Label != expectedLabel {
			t.Fatalf("expected label %q, got %q", expectedLabel, role.Label)
		}
	}
}

func TestMemberHasRole(t *testing.T) {
	roleID1 := snowflake.MustParse("111111111111111111")
	roleID2 := snowflake.MustParse("222222222222222222")
	roleID3 := snowflake.MustParse("333333333333333333")

	memberRoles := []snowflake.ID{roleID1, roleID3}

	tests := []struct {
		name     string
		roleID   snowflake.ID
		expected bool
	}{
		{"has role 1", roleID1, true},
		{"has role 3", roleID3, true},
		{"missing role 2", roleID2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := memberHasRole(memberRoles, tt.roleID)
			if result != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestTotalRoleCount(t *testing.T) {
	tests := []struct {
		name     string
		category []roleCategory
		expected int
	}{
		{
			"single category single role",
			[]roleCategory{
				{Name: "Cat1", Roles: []roleButton{{Label: "R1"}}},
			},
			1,
		},
		{
			"single category multiple roles",
			[]roleCategory{
				{Name: "Cat1", Roles: []roleButton{
					{Label: "R1"},
					{Label: "R2"},
					{Label: "R3"},
				}},
			},
			3,
		},
		{
			"multiple categories",
			[]roleCategory{
				{Name: "Cat1", Roles: []roleButton{{Label: "R1"}, {Label: "R2"}}},
				{Name: "Cat2", Roles: []roleButton{{Label: "R3"}}},
				{Name: "Cat3", Roles: []roleButton{{Label: "R4"}, {Label: "R5"}, {Label: "R6"}}},
			},
			6,
		},
		{
			"empty categories",
			[]roleCategory{},
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := totalRoleCount(tt.category)
			if result != tt.expected {
				t.Fatalf("expected %d roles, got %d", tt.expected, result)
			}
		})
	}
}

func TestFormatRoleMessages(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		expected string
	}{
		{"simple label", "Filmschauer", "Die Rolle „Filmschauer“ wurde dir gegeben."},
		{"label with spaces", "The Trucker Bros", "Die Rolle „The Trucker Bros“ wurde dir gegeben."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role := roleButton{Label: tt.label}
			result := fmtRoleAdded(role)
			if result != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, result)
			}
		})
	}

	// Test removal message
	role := roleButton{Label: "TestRole"}
	removeMsg := fmtRoleRemoved(role)
	if removeMsg != "Die Rolle „TestRole“ wurde dir entfernt." {
		t.Fatalf("unexpected removal message: %q", removeMsg)
	}
}

func TestCollectButtonCustomIDs(t *testing.T) {
	tests := []struct {
		name       string
		components []discord.LayoutComponent
		expected   customIDSet
	}{
		{
			"single row single button",
			[]discord.LayoutComponent{
				discord.ActionRowComponent{
					Components: []discord.InteractiveComponent{
						discord.NewPrimaryButton("Film", "film_role_toggle"),
					},
				},
			},
			customIDSet{"film_role_toggle": {}},
		},
		{
			"multiple rows multiple buttons",
			[]discord.LayoutComponent{
				discord.ActionRowComponent{
					Components: []discord.InteractiveComponent{
						discord.NewPrimaryButton("A", "custom_a"),
						discord.NewSuccessButton("B", "custom_b"),
					},
				},
				discord.ActionRowComponent{
					Components: []discord.InteractiveComponent{
						discord.NewDangerButton("C", "custom_c"),
					},
				},
			},
			customIDSet{"custom_a": {}, "custom_b": {}, "custom_c": {}},
		},
		{
			"non-button components ignored",
			[]discord.LayoutComponent{
				discord.ActionRowComponent{
					Components: []discord.InteractiveComponent{
						discord.NewPrimaryButton("Film", "film_role_toggle"),
					},
				},
			},
			customIDSet{"film_role_toggle": {}},
		},
		{
			"empty components",
			[]discord.LayoutComponent{},
			customIDSet{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collectButtonCustomIDs(tt.components)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d custom IDs, got %d: %v", len(tt.expected), len(result), result)
			}
			for customID := range tt.expected {
				if _, ok := result[customID]; !ok {
					t.Fatalf("expected custom ID %q in result, got %v", customID, result)
				}
			}
		})
	}
}

func TestCustomIDSetKeyIsDeterministic(t *testing.T) {
	first := customIDSet{"b": {}, "a": {}, "c": {}}
	second := customIDSet{"c": {}, "a": {}, "b": {}}

	if first.key() != second.key() {
		t.Fatalf("expected identical keys for same set, got %q and %q", first.key(), second.key())
	}

	// Different sets must produce different keys
	third := customIDSet{"a": {}, "b": {}}
	if first.key() == third.key() {
		t.Fatalf("expected different keys for different sets")
	}
}

func TestIndexMessagesByCustomIDs(t *testing.T) {
	messageA := discord.Message{
		ID:     snowflake.MustParse("111111111111111111"),
		Author: discord.User{ID: snowflake.MustParse("999999999999999999"), Bot: true},
		Components: []discord.LayoutComponent{
			discord.ActionRowComponent{
				Components: []discord.InteractiveComponent{
					discord.NewPrimaryButton("A", "custom_a"),
				},
			},
		},
	}
	messageB := discord.Message{
		ID:     snowflake.MustParse("222222222222222222"),
		Author: discord.User{ID: snowflake.MustParse("999999999999999999"), Bot: true},
		Components: []discord.LayoutComponent{
			discord.ActionRowComponent{
				Components: []discord.InteractiveComponent{
					discord.NewPrimaryButton("B", "custom_b"),
				},
			},
		},
	}

	index := indexMessagesByCustomIDs([]discord.Message{messageA, messageB})

	if len(index) != 2 {
		t.Fatalf("expected 2 entries in index, got %d", len(index))
	}

	keyA := customIDSet{"custom_a": {}}.key()
	if index[keyA] != messageA.ID {
		t.Fatalf("expected message A ID for key %q, got %s", keyA, index[keyA])
	}

	keyB := customIDSet{"custom_b": {}}.key()
	if index[keyB] != messageB.ID {
		t.Fatalf("expected message B ID for key %q, got %s", keyB, index[keyB])
	}
}

func TestIsManagedRolePanelMessage(t *testing.T) {
	bot := newRoleBot(config{
		Categories: []roleCategory{
			{
				Name: "Filme",
				Roles: []roleButton{
					{RoleID: snowflake.ID(1), Label: "Film", CustomID: "film_role_toggle"},
				},
			},
		},
	})

	managed := discord.Message{
		Author: discord.User{Bot: true},
		Components: []discord.LayoutComponent{
			discord.ActionRowComponent{
				Components: []discord.InteractiveComponent{
					discord.NewPrimaryButton("Film", "film_role_toggle"),
				},
			},
		},
	}
	if !bot.isManagedRolePanelMessage(managed) {
		t.Fatalf("expected message with managed custom ID to be recognized")
	}

	// Message with a foreign custom ID should not be managed
	foreign := discord.Message{
		Author: discord.User{Bot: true},
		Components: []discord.LayoutComponent{
			discord.ActionRowComponent{
				Components: []discord.InteractiveComponent{
					discord.NewPrimaryButton("Other", "unrelated_custom_id"),
				},
			},
		},
	}
	if bot.isManagedRolePanelMessage(foreign) {
		t.Fatalf("expected foreign message to not be managed")
	}
}

func TestMessageUpdateFromCreate(t *testing.T) {
	messageCreate := discord.NewMessageCreate().
		WithEmbeds(discord.Embed{Title: "🎮 Gaming", Description: "Test"}).
		AddActionRow(discord.NewPrimaryButton("Film", "film_role_toggle"))

	messageUpdate := messageUpdateFromCreate(messageCreate)

	if messageUpdate.Embeds == nil || len(*messageUpdate.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %v", messageUpdate.Embeds)
	}
	if (*messageUpdate.Embeds)[0].Title != "🎮 Gaming" {
		t.Fatalf("expected embed title, got %q", (*messageUpdate.Embeds)[0].Title)
	}
	if messageUpdate.Components == nil || len(*messageUpdate.Components) != 1 {
		t.Fatalf("expected 1 component, got %v", messageUpdate.Components)
	}
}
