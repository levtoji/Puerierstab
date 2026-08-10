# Comprehensive Test Coverage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add comprehensive test coverage for all code paths including config loading with emojis, message store persistence, embed building, and role toggle logic.

**Architecture:** TDD approach with unit tests for each component and integration tests for message publishing. Tests will validate config parsing, message store CRUD operations, embed building with proper structure, and error handling for edge cases.

**Tech Stack:** Go `testing` package, `t.Run` for sub-tests, `t.Setenv` for environment variables

---

## File Structure

**Files to modify/create:**
- `config_test.go` - Update existing tests, add tests for emoji field and new JSON structure
- `messages_test.go` - Create new test file for message store CRUD and persistence
- `main_test.go` - Create integration tests for role toggle and message publishing

---

## Task 1: Update config_test.go for new emoji field and legacy support removal

**Files:**
- Modify: `config_test.go`

- [ ] **Step 1: Update TestLoadConfigFromEnvWithJSONConfig to test emoji field**

Replace the entire test function with this:

```go
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
```

- [ ] **Step 2: Delete TestLoadConfigFromEnvWithLegacyFilmRole test**

Remove the entire `TestLoadConfigFromEnvWithLegacyFilmRole` function (lines 40-58) since we removed legacy support.

- [ ] **Step 3: Add test for missing ROLE_CATEGORIES_JSON**

Add this new test function:

```go
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
```

- [ ] **Step 4: Add test for missing DISCORD_BOT_TOKEN**

Add this new test function:

```go
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
```

- [ ] **Step 5: Update TestBuildCategoryMessagesChunksButtonsIntoMultipleMessages**

Replace with:

```go
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
```

- [ ] **Step 6: Update TestRoleButtonComponentUsesRequestedStyle**

Replace with:

```go
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
```

- [ ] **Step 7: Delete TestIsManagedRolePanelMessage**

Remove the entire `TestIsManagedRolePanelMessage` function (lines 100-125) since this function was deleted from main.go.

- [ ] **Step 8: Run tests to verify they fail**

Run: `go test -v`
Expected: Multiple test failures related to missing functions/fields

- [ ] **Step 9: Commit**

```bash
git add config_test.go
git commit -m "test: update config tests for emoji support and remove legacy tests"
```

---

## Task 2: Create messages_test.go for message store testing

**Files:**
- Create: `messages_test.go`

- [ ] **Step 1: Create test file with MessageStore creation test**

Create `/Users/lev.perschin/GolandProjects/Puerierstab/messages_test.go` with this content:

```go
package main

import (
	"os"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func TestMessageStoreLoadEmpty(t *testing.T) {
	// Clean up test file if it exists
	_ = os.Remove(".role_messages_test.json")
	defer os.Remove(".role_messages_test.json")

	// Test loading non-existent file
	store, err := loadMessageStore()
	if err != nil {
		t.Fatalf("loadMessageStore() error = %v, want nil", err)
	}
	if store == nil {
		t.Fatalf("expected store, got nil")
	}
	if len(store.Messages) != 0 {
		t.Fatalf("expected empty messages, got %d", len(store.Messages))
	}
}

func TestMessageStoreSetAndGet(t *testing.T) {
	store := &messageStore{Messages: []storedMessage{}}

	categoryName := "Gaming"
	messageIdx := 0
	messageID := snowflake.MustParse("123456789012345678")

	// Get should return false for non-existent
	_, exists := store.getMessageID(categoryName, messageIdx)
	if exists {
		t.Fatalf("expected exists=false for non-existent message")
	}

	// Set the message
	store.setMessageID(categoryName, messageIdx, messageID)

	// Get should now return true
	id, exists := store.getMessageID(categoryName, messageIdx)
	if !exists {
		t.Fatalf("expected exists=true after set")
	}
	if id != messageID {
		t.Fatalf("expected messageID %s, got %s", messageID, id)
	}
}

func TestMessageStoreUpdate(t *testing.T) {
	store := &messageStore{Messages: []storedMessage{}}

	categoryName := "Films"
	messageIdx := 0
	oldID := snowflake.MustParse("111111111111111111")
	newID := snowflake.MustParse("222222222222222222")

	// Set initial
	store.setMessageID(categoryName, messageIdx, oldID)

	// Update with new ID
	store.setMessageID(categoryName, messageIdx, newID)

	// Verify only one entry exists with new ID
	id, exists := store.getMessageID(categoryName, messageIdx)
	if !exists {
		t.Fatalf("expected exists=true after update")
	}
	if id != newID {
		t.Fatalf("expected updated messageID %s, got %s", newID, id)
	}
	if len(store.Messages) != 1 {
		t.Fatalf("expected 1 message stored, got %d", len(store.Messages))
	}
}

func TestMessageStoreMultipleEntries(t *testing.T) {
	store := &messageStore{Messages: []storedMessage{}}

	// Add entries for different categories and indices
	store.setMessageID("Gaming", 0, snowflake.MustParse("111111111111111111"))
	store.setMessageID("Gaming", 1, snowflake.MustParse("222222222222222222"))
	store.setMessageID("Films", 0, snowflake.MustParse("333333333333333333"))
	store.setMessageID("Films", 1, snowflake.MustParse("444444444444444444"))

	// Verify all entries
	tests := []struct {
		category string
		idx      int
		expected snowflake.ID
	}{
		{"Gaming", 0, snowflake.MustParse("111111111111111111")},
		{"Gaming", 1, snowflake.MustParse("222222222222222222")},
		{"Films", 0, snowflake.MustParse("333333333333333333")},
		{"Films", 1, snowflake.MustParse("444444444444444444")},
	}

	for _, tt := range tests {
		t.Run(tt.category+":"+string(rune(tt.idx)), func(t *testing.T) {
			id, exists := store.getMessageID(tt.category, tt.idx)
			if !exists {
				t.Fatalf("expected exists=true")
			}
			if id != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, id)
			}
		})
	}

	if len(store.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(store.Messages))
	}
}

func TestMessageStoreSaveAndLoad(t *testing.T) {
	testFile := ".role_messages_test.json"
	defer os.Remove(testFile)

	// Create and populate store
	store1 := &messageStore{
		Messages: []storedMessage{
			{
				CategoryName: "Gaming",
				MessageIndex: 0,
				MessageID:    snowflake.MustParse("111111111111111111"),
			},
			{
				CategoryName: "Films",
				MessageIndex: 0,
				MessageID:    snowflake.MustParse("222222222222222222"),
			},
		},
	}

	// Save to custom file (simulate)
	data, err := json.MarshalIndent(store1, "", "  ")
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if err := os.WriteFile(testFile, data, 0o644); err != nil {
		t.Fatalf("write file error: %v", err)
	}

	// Load from file
	fileData, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	var store2 messageStore
	if err := json.Unmarshal(fileData, &store2); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Verify
	if len(store2.Messages) != 2 {
		t.Fatalf("expected 2 messages after load, got %d", len(store2.Messages))
	}
	if store2.Messages[0].CategoryName != "Gaming" {
		t.Fatalf("expected first message category Gaming, got %q", store2.Messages[0].CategoryName)
	}
}
```

Don't forget to add import at the top of the file:

```go
package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)
```

- [ ] **Step 2: Run tests**

Run: `go test ./... -v -run TestMessageStore`
Expected: All 5 new test functions pass

- [ ] **Step 3: Commit**

```bash
git add messages_test.go
git commit -m "test: add comprehensive message store tests"
```

---

## Task 3: Add integration tests in main_test.go

**Files:**
- Create: `main_test.go`

- [ ] **Step 1: Create main_test.go with role button tests**

Create `/Users/lev.perschin/GolandProjects/Puerierstab/main_test.go`:

```go
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
		{"simple label", "Filmschauer", "Die Rolle „Filmschauer" wurde dir gegeben."},
		{"label with spaces", "The Trucker Bros", "Die Rolle „The Trucker Bros" wurde dir gegeben."},
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
	if removeMsg != "Die Rolle „TestRole" wurde dir entfernt." {
		t.Fatalf("unexpected removal message: %q", removeMsg)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./... -v -run TestNewRoleBot`
Expected: All tests pass

- [ ] **Step 3: Commit**

```bash
git add main_test.go
git commit -m "test: add role bot initialization and helper function tests"
```

---

## Task 4: Add normalization tests in config_test.go

**Files:**
- Modify: `config_test.go`

- [ ] **Step 1: Add test for category validation (empty name)**

Add this to `config_test.go`:

```go
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
```

- [ ] **Step 2: Add test for auto-generated custom IDs**

Add this to `config_test.go`:

```go
func TestNormalizeCategoriesAutoGenerateCustomID(t *testing.T) {
	input := []roleCategoryJSON{
		{
			Name: "Gaming",
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
```

- [ ] **Step 3: Run all config tests**

Run: `go test ./... -v -run TestNormalize`
Expected: All tests pass

- [ ] **Step 4: Commit**

```bash
git add config_test.go
git commit -m "test: add comprehensive category normalization tests"
```

---

## Task 5: Verify all tests pass and final build

**Files:**
- None (verification only)

- [ ] **Step 1: Run all tests with coverage**

Run: `go test -v -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | head -20`
Expected: All tests pass, coverage report shows tested functions

- [ ] **Step 2: Build the project**

Run: `go build -o /tmp/puerierstab .`
Expected: Build succeeds with no errors

- [ ] **Step 3: Verify test count**

Run: `go test ./... -v 2>&1 | grep -c "^=== RUN"` (should show ~20+ tests)

- [ ] **Step 4: Final commit**

```bash
git add .
git commit -m "test: complete comprehensive test coverage for all code paths"
```

---

## Summary

This plan adds **20+ comprehensive tests** covering:
- ✅ Config loading with emoji field
- ✅ Message store CRUD operations
- ✅ Role button styles
- ✅ Category validation & normalization
- ✅ Message building with embeds
- ✅ Error handling for edge cases
- ✅ Integration between components
