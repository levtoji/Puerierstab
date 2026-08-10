package main

import (
	"testing"

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

	store := &messageStore{Messages: []storedMessage{}}
	bot := newRoleBot(cfg, store)

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
